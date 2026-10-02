package main

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	tfprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/permitio/permit-golang/pkg/openapi"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

const (
	modulePath    = "github.com/permitio/terraform-provider-permit-io"
	providerPath  = modulePath + "/internal/provider"
	mockPath      = modulePath + "/internal/acctest/mockpermit"
	sdkAPIPath    = "github.com/permitio/permit-golang/pkg/api"
	sdkClientPath = "github.com/permitio/permit-golang/pkg/openapi"
	sdkModelsPath = "github.com/permitio/permit-golang/pkg/models"
)

// httpSuffix ends the name of a call that sends a request the provider builds
// itself with net/http, as mockpermit names it.
const httpSuffix = " (HTTP)"

// providerSurface is the surface name of the provider block, whose configuration
// reads the API key's scope.
const providerSurface = "provider"

// callSite is a call in the provider's source that sends requests to the API.
type callSite struct {
	position string
	// pkg is the provider package's directory under internal/provider, "." for
	// internal/provider itself.
	pkg string
	// call names the call as mockpermit's routes do: Group.Method for the SDK's
	// Api, Service.Operation for the SDK's generated client, and
	// "<pkg>.<function> (HTTP)" for a request the provider builds itself.
	call string
	// sdk is true for a call through the SDK.
	sdk bool
	// routes are the requests the call sends: from the SDK's source for an SDK
	// call, from mockpermit's route table for a request the provider builds.
	routes []route
	// wire are mockpermit's routes for the call, which the offline tests serve.
	wire []route
}

// facts is what apicoverage learns from the provider and the SDK.
type facts struct {
	sites []callSite
	// clientOperations is the number of operations indexed from the SDK's
	// generated client.
	clientOperations int
	// models maps a route key to the Go type the SDK's generated client decodes
	// the route's response into, for responses decoded into a model.
	models map[string]reflect.Type
	// surfaces maps a provider package's directory to the Terraform resources and
	// data sources it registers ("data." before a data source), and "." to the
	// provider block.
	surfaces    map[string][]string
	resources   int
	dataSources int
	// problems are the findings of the walk itself.
	problems []string
}

// gatherFacts loads the provider, mockpermit and the SDK's API and generated
// client from source in the module at root, and walks them.
func gatherFacts(root string) (facts, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: root,
	}
	pkgs, err := packages.Load(cfg, "./internal/provider/...", "./internal/acctest/mockpermit",
		sdkAPIPath, sdkClientPath)
	if err != nil {
		return facts{}, fmt.Errorf("loading the packages: %w", err)
	}
	var loadErrors []string
	byPath := map[string]*packages.Package{}
	var providerPkgs []*packages.Package
	for _, pkg := range pkgs {
		for _, loadErr := range pkg.Errors {
			loadErrors = append(loadErrors, loadErr.Error())
		}
		byPath[pkg.PkgPath] = pkg
		if pkg.PkgPath == providerPath || strings.HasPrefix(pkg.PkgPath, providerPath+"/") {
			providerPkgs = append(providerPkgs, pkg)
		}
	}
	if len(loadErrors) > 0 {
		return facts{}, fmt.Errorf("loading the packages: %s", strings.Join(loadErrors, "; "))
	}
	for _, path := range []string{mockPath, sdkAPIPath, sdkClientPath} {
		if byPath[path] == nil {
			return facts{}, fmt.Errorf("package %s was not loaded", path)
		}
	}

	clientRoutes, err := indexClient(byPath[sdkClientPath])
	if err != nil {
		return facts{}, err
	}
	mockRoutes, scope, err := readMockRoutes(byPath[mockPath])
	if err != nil {
		return facts{}, err
	}
	f := facts{clientOperations: len(clientRoutes), models: clientModels(clientRoutes)}
	resolver := newSDKResolver(byPath[sdkAPIPath])
	for _, pkg := range providerPkgs {
		sites, problems := walkPackage(pkg, root, resolver, clientRoutes)
		f.sites = append(f.sites, sites...)
		f.problems = append(f.problems, problems...)
	}
	f.problems = append(f.problems, attachMockRoutes(f.sites, mockRoutes, scope)...)
	slices.SortFunc(f.sites, func(a, b callSite) int {
		return strings.Compare(a.position, b.position)
	})

	f.surfaces, f.resources, f.dataSources = providerSurfaces()
	return f, nil
}

// indexClient returns the route of every operation of the SDK's generated client,
// by Service.Operation, read from the operation's Execute method: its
// localVarHTTPMethod and the path literal of its localVarPath.
func indexClient(pkg *packages.Package) (map[string]route, error) {
	routes := map[string]route{}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			operation, isExecute := strings.CutSuffix(fn.Name.Name, "Execute")
			service := receiverName(fn)
			if !isExecute || !isClientService(service) {
				continue
			}
			r, err := executeRoute(pkg.TypesInfo, fn)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", service, fn.Name.Name, err)
			}
			routes[service+"."+operation] = r
		}
	}
	return routes, nil
}

// isClientService reports whether a type name is a service of the SDK's generated
// client, such as TenantsApiService or ProxyConfigAPIService.
func isClientService(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), "apiservice")
}

// receiverName returns the name of a method's receiver type, without a pointer.
func receiverName(fn *ast.FuncDecl) string {
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// executeRoute reads the HTTP method and path a generated Execute method sends.
func executeRoute(info *types.Info, fn *ast.FuncDecl) (route, error) {
	var r route
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.ValueSpec:
			for i, name := range n.Names {
				if name.Name == "localVarHTTPMethod" && i < len(n.Values) {
					if value := info.Types[n.Values[i]].Value; value != nil &&
						value.Kind() == constant.String {
						r.method = constant.StringVal(value)
					}
				}
			}
		case *ast.AssignStmt:
			ident, ok := n.Lhs[0].(*ast.Ident)
			if !ok || ident.Name != "localVarPath" || n.Tok != token.DEFINE || len(n.Rhs) != 1 {
				return true
			}
			if sum, ok := n.Rhs[0].(*ast.BinaryExpr); ok {
				if literal, ok := sum.Y.(*ast.BasicLit); ok && literal.Kind == token.STRING {
					r.path, _ = strconv.Unquote(literal.Value)
				}
			}
		}
		return true
	})
	if r.method == "" || !strings.HasPrefix(r.path, "/") {
		return r, errors.New("found no localVarHTTPMethod constant and localVarPath literal")
	}
	return r, nil
}

// clientModels returns the Go type each generated client operation decodes its
// response into, by the key of the operation's route, for types of the SDK's
// models package.
func clientModels(clientRoutes map[string]route) map[string]reflect.Type {
	models := map[string]reflect.Type{}
	client := reflect.TypeFor[openapi.APIClient]()
	for i := range client.NumField() {
		service := client.Field(i).Type
		if service.Kind() != reflect.Pointer {
			continue
		}
		for j := range service.NumMethod() {
			method := service.Method(j)
			operation, ok := strings.CutSuffix(method.Name, "Execute")
			r, known := clientRoutes[service.Elem().Name()+"."+operation]
			if !ok || !known || method.Type.NumOut() < 3 {
				continue
			}
			result := method.Type.Out(0)
			element := result
			for element.Kind() == reflect.Pointer || element.Kind() == reflect.Slice {
				element = element.Elem()
			}
			if element.PkgPath() == sdkModelsPath {
				models[r.key()] = result
			}
		}
	}
	return models
}

// sdkResolver finds the generated client operations an SDK Api method sends,
// following the calls it makes to other functions of the SDK's API package.
type sdkResolver struct {
	info  *types.Info
	decls map[string]*ast.FuncDecl
	memo  map[string][]string
}

func newSDKResolver(pkg *packages.Package) *sdkResolver {
	r := &sdkResolver{info: pkg.TypesInfo, decls: map[string]*ast.FuncDecl{},
		memo: map[string][]string{}}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				if obj, ok := pkg.TypesInfo.Defs[fn.Name].(*types.Func); ok {
					r.decls[obj.FullName()] = fn
				}
			}
		}
	}
	return r
}

// resolve returns the generated client operations fn calls, directly or through
// other functions of the API package, as sorted Service.Operation names.
func (r *sdkResolver) resolve(fn *types.Func) []string {
	name := fn.Origin().FullName()
	if operations, done := r.memo[name]; done {
		return operations
	}
	r.memo[name] = nil
	decl := r.decls[name]
	if decl == nil {
		return nil
	}
	found := map[string]bool{}
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee := typeutil.StaticCallee(r.info, call)
		if operation, ok := clientOperation(callee); ok {
			found[operation] = true
		} else if callee != nil && callee.Pkg() != nil && callee.Pkg().Path() == sdkAPIPath {
			for _, operation := range r.resolve(callee) {
				found[operation] = true
			}
		}
		return true
	})
	r.memo[name] = slices.Sorted(maps.Keys(found))
	return r.memo[name]
}

// clientOperation returns Service.Operation when fn is an operation, or its
// Execute method, of the SDK's generated client.
func clientOperation(fn *types.Func) (string, bool) {
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != sdkClientPath {
		return "", false
	}
	service := methodReceiver(fn)
	if !isClientService(service) {
		return "", false
	}
	return service + "." + strings.TrimSuffix(fn.Name(), "Execute"), true
}

// methodReceiver returns the name of fn's receiver type, or "" for a function.
func methodReceiver(fn *types.Func) string {
	recv := fn.Signature().Recv()
	if recv == nil {
		return ""
	}
	t := recv.Type()
	if pointer, ok := t.(*types.Pointer); ok {
		t = pointer.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}

// httpRequestFuncs are the net/http functions, and methods of http.Client, that
// build or send a request.
var httpRequestFuncs = []string{"NewRequest", "NewRequestWithContext", "Get", "Head", "Post",
	"PostForm"}

// isAPIMethod reports whether fn is a method of the SDK's Api, such as
// Tenants.Create.
func isAPIMethod(fn *types.Func) bool {
	return fn.Pkg().Path() == sdkAPIPath && methodReceiver(fn) != ""
}

// isHTTPRequest reports whether fn is a net/http function, or a method of
// http.Client, that builds or sends a request.
func isHTTPRequest(fn *types.Func) bool {
	if fn.Pkg().Path() != "net/http" || !slices.Contains(httpRequestFuncs, fn.Name()) {
		return false
	}
	receiver := methodReceiver(fn)
	return receiver == "" || receiver == "Client"
}

// walkPackage returns the call sites in a provider package, and a problem for
// each use of an SDK operation that is not a direct call, and for each SDK Api
// method call whose requests the resolver cannot find.
func walkPackage(pkg *packages.Package, root string, resolver *sdkResolver,
	clientRoutes map[string]route,
) ([]callSite, []string) {
	dir := strings.TrimPrefix(strings.TrimPrefix(pkg.PkgPath, providerPath), "/")
	if dir == "" {
		dir = "."
	}
	var sites []callSite
	var problems []string
	for _, file := range pkg.Syntax {
		called := map[*ast.SelectorExpr]bool{}
		for _, decl := range file.Decls {
			function := "init"
			if fn, ok := decl.(*ast.FuncDecl); ok {
				function = fn.Name.Name
			}
			ast.Inspect(decl, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
					called[selector] = true
				}
				callee := typeutil.StaticCallee(pkg.TypesInfo, call)
				if callee == nil || callee.Pkg() == nil {
					return true
				}
				site := callSite{position: relativePosition(pkg.Fset, call.Pos(), root), pkg: dir}
				var operations []string
				clientCall, isClient := clientOperation(callee)
				switch {
				case isAPIMethod(callee):
					site.call = methodReceiver(callee) + "." + callee.Name()
					site.sdk = true
					operations = resolver.resolve(callee)
				case isClient:
					site.call = clientCall
					site.sdk = true
					operations = []string{clientCall}
				case isHTTPRequest(callee):
					site.call = dir + "." + function + httpSuffix
				default:
					return true
				}
				for _, operation := range operations {
					r, known := clientRoutes[operation]
					if !known {
						problems = append(problems, fmt.Sprintf("%s: %s calls %s, which has no "+
							"route in the SDK's generated client", site.position, site.call,
							operation))
						continue
					}
					site.routes = append(site.routes, r)
				}
				if site.sdk && len(site.routes) == 0 {
					problems = append(problems, fmt.Sprintf("%s: found no request that %s sends "+
						"in the SDK's source", site.position, site.call))
					return true
				}
				sites = append(sites, site)
				return true
			})
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || called[selector] {
				return true
			}
			method, ok := pkg.TypesInfo.Uses[selector.Sel].(*types.Func)
			if !ok || method.Pkg() == nil {
				return true
			}
			if _, isClient := clientOperation(method); isClient || isAPIMethod(method) {
				position := relativePosition(pkg.Fset, selector.Pos(), root)
				problems = append(problems, fmt.Sprintf("%s: %s is used but not called directly, "+
					"so its requests cannot be told", position, selector.Sel.Name))
			}
			return true
		})
	}
	return sites, problems
}

// relativePosition returns file:line:column with the file relative to root.
func relativePosition(fset *token.FileSet, pos token.Pos, root string) string {
	position := fset.Position(pos)
	absRoot, err := filepath.Abs(root)
	if err == nil {
		if rel, err := filepath.Rel(absRoot, position.Filename); err == nil {
			position.Filename = filepath.ToSlash(rel)
		}
	}
	return position.String()
}

// readMockRoutes reads mockpermit's route table, by operation, from the constant
// patterns and operations of the Routes literals the package declares, and the
// API key scope route it serves besides them. The route table is checked against
// the SDK and the provider by mockpermit's own tests.
func readMockRoutes(pkg *packages.Package) (map[string][]route, route, error) {
	routesType, ok := pkg.Types.Scope().Lookup("Routes").(*types.TypeName)
	if !ok {
		return nil, route{}, errors.New("mockpermit declares no Routes type")
	}
	scopePath, ok := pkg.Types.Scope().Lookup("scopePath").(*types.Const)
	if !ok || scopePath.Val().Kind() != constant.String {
		return nil, route{}, errors.New("mockpermit declares no scopePath string constant")
	}
	scope := route{method: "GET", path: constant.StringVal(scopePath.Val())}
	routes := map[string][]route{}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range value.Names {
					obj := pkg.TypesInfo.Defs[name]
					if obj == nil || !types.Identical(obj.Type(), routesType.Type()) ||
						i >= len(value.Values) {
						continue
					}
					literal, ok := value.Values[i].(*ast.CompositeLit)
					if !ok {
						return nil, route{}, fmt.Errorf("mockpermit.%s is not a Routes literal",
							name.Name)
					}
					if err := readRouteLiteral(pkg.TypesInfo, literal, routes); err != nil {
						return nil, route{}, fmt.Errorf("mockpermit.%s: %w", name.Name, err)
					}
				}
			}
		}
	}
	if len(routes) == 0 {
		return nil, route{}, errors.New("read no routes from mockpermit")
	}
	return routes, scope, nil
}

func readRouteLiteral(info *types.Info, literal *ast.CompositeLit,
	routes map[string][]route,
) error {
	for _, element := range literal.Elts {
		fields, ok := element.(*ast.CompositeLit)
		if !ok || len(fields.Elts) < 2 {
			return errors.New("a route is not a {pattern, operation, handler} literal")
		}
		values := map[string]ast.Expr{"pattern": fields.Elts[0], "operation": fields.Elts[1]}
		for _, field := range fields.Elts {
			if keyed, ok := field.(*ast.KeyValueExpr); ok {
				if key, ok := keyed.Key.(*ast.Ident); ok {
					values[key.Name] = keyed.Value
				}
			}
		}
		pattern, err := constantString(info, values["pattern"])
		if err != nil {
			return err
		}
		operation, err := constantString(info, values["operation"])
		if err != nil {
			return err
		}
		r, err := parseRoute(pattern)
		if err != nil {
			return err
		}
		routes[operation] = append(routes[operation], r)
	}
	return nil
}

func constantString(info *types.Info, expr ast.Expr) (string, error) {
	value := info.Types[expr].Value
	if value == nil || value.Kind() != constant.String {
		return "", fmt.Errorf("%s is not a string constant", types.ExprString(expr))
	}
	return constant.StringVal(value), nil
}

// attachMockRoutes sets each site's wire routes from mockpermit, which serves the
// API key scope to every test, and the routes of a request the provider builds
// itself, which only mockpermit's route table knows. It returns a problem for a
// call mockpermit has no route for, and for an SDK call that mockpermit serves on
// a route the SDK's source does not send, so this walk and mockpermit's own must
// find the same calls.
func attachMockRoutes(sites []callSite, mockRoutes map[string][]route, scope route) []string {
	var problems []string
	for i := range sites {
		site := &sites[i]
		site.wire = mockRoutes[site.call]
		if len(site.wire) == 0 && slices.Contains(site.routes, scope) {
			site.wire = []route{scope}
		}
		if !site.sdk {
			site.routes = site.wire
			if len(site.routes) == 0 {
				problems = append(problems, fmt.Sprintf("%s: mockpermit has no route for %s, "+
					"so the request it sends is unknown", site.position, site.call))
			}
			continue
		}
		if len(site.wire) == 0 {
			problems = append(problems, fmt.Sprintf("%s: mockpermit has no route for %s, so the "+
				"offline tests cannot serve it; add one to its route table", site.position,
				site.call))
			continue
		}
		sent := map[string]bool{}
		for _, r := range site.routes {
			sent[r.key()] = true
		}
		for _, r := range site.wire {
			if !sent[r.key()] {
				problems = append(problems, fmt.Sprintf("%s: mockpermit serves %s on %s, but the "+
					"SDK's source sends %s", site.position, site.call, r, joinRoutes(site.routes)))
			}
		}
	}
	return problems
}

func joinRoutes(routes []route) string {
	names := make([]string, len(routes))
	for i, r := range routes {
		names[i] = r.String()
	}
	return strings.Join(names, ", ")
}

// providerSurfaces returns the Terraform resources and data sources the provider
// registers, by the directory of the package that implements each, and how many
// of each there are. The provider block is the surface of internal/provider.
func providerSurfaces() (map[string][]string, int, int) {
	ctx := context.Background()
	p := provider.New("apicoverage")()
	var meta tfprovider.MetadataResponse
	p.Metadata(ctx, tfprovider.MetadataRequest{}, &meta)
	surfaces := map[string][]string{".": {providerSurface}}
	resources := p.Resources(ctx)
	for _, newResource := range resources {
		r := newResource()
		var response resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: meta.TypeName}, &response)
		dir := packageDir(r)
		surfaces[dir] = append(surfaces[dir], response.TypeName)
	}
	dataSources := p.DataSources(ctx)
	for _, newDataSource := range dataSources {
		d := newDataSource()
		var response datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: meta.TypeName}, &response)
		dir := packageDir(d)
		surfaces[dir] = append(surfaces[dir], "data."+response.TypeName)
	}
	return surfaces, len(resources), len(dataSources)
}

// packageDir returns the directory under internal/provider of the package that
// declares v's type.
func packageDir(v any) string {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return strings.TrimPrefix(strings.TrimPrefix(t.PkgPath(), providerPath), "/")
}
