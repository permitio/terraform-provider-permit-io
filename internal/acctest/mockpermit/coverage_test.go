package mockpermit

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/permit"
)

// routeSets are the route sets the package exports, by name.
// TestRouteSetsAreListed checks this list against the package source, so the
// coverage tests below see every set.
var routeSets = map[string]Routes{
	"ConditionSetRules":  ConditionSetRules,
	"ConditionSets":      ConditionSets,
	"GroupRoles":         GroupRoles,
	"ImplicitGrants":     ImplicitGrants,
	"ProxyConfigs":       ProxyConfigs,
	"ResourceAttributes": ResourceAttributes,
	"ResourceInstances":  ResourceInstances,
	"ResourceRelations":  ResourceRelations,
	"ResourceRoles":      ResourceRoles,
	"Resources":          Resources,
	"RoleAssignments":    RoleAssignments,
	"Roles":              Roles,
	"TenantList":         TenantList,
	"Tenants":            Tenants,
	"Users":              Users,
}

// providerDir is the provider's source, relative to this package.
const providerDir = "../../provider"

// releasedOnly are the operations that the provider no longer calls but the
// release the provider's upgrade test starts from does, so that test serves their
// routes. That release's permitio_group_resource_instance_role_assignment reads the
// project and environment IDs from the tenant list.
var releasedOnly = map[string]bool{"Tenants.List": true}

func TestRouteSetsAreListed(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the package source: %v", err)
	}
	fset := token.NewFileSet()
	var exported []string
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, source, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		exported = append(exported, exportedRouteSets(file)...)
	}
	if len(exported) == 0 {
		t.Fatalf("found no exported route sets in %q", sources)
	}
	slices.Sort(exported)
	if listed := slices.Sorted(maps.Keys(routeSets)); !slices.Equal(listed, exported) {
		t.Errorf("routeSets lists %q, but the package exports the route sets %q", listed, exported)
	}
}

// exportedRouteSets returns the names of the exported package variables in file
// that are declared as Routes or set to a Routes literal.
func exportedRouteSets(file *ast.File) []string {
	isRoutes := func(expr ast.Expr) bool {
		ident, ok := expr.(*ast.Ident)
		return ok && ident.Name == "Routes"
	}
	var names []string
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
				literal := false
				if i < len(value.Values) {
					composite, ok := value.Values[i].(*ast.CompositeLit)
					literal = ok && isRoutes(composite.Type)
				}
				if name.IsExported() && (isRoutes(value.Type) || literal) {
					names = append(names, name.Name)
				}
			}
		}
	}
	return names
}

// TestRoutesMatchTheSDK calls each route's operation through the SDK, against a
// mock that serves only that route, and checks that the SDK sent exactly one
// request and that the route received it. The arguments are placeholders: the
// request only has to reach the route, not succeed. A route for a request the
// provider builds itself is checked by CheckHTTPRoutes in the tests of the provider
// package that builds it instead, which TestProviderCallSitesHaveRoutes requires.
func TestRoutesMatchTheSDK(t *testing.T) {
	sdkRoutes := 0
	for _, setName := range slices.Sorted(maps.Keys(routeSets)) {
		for _, rt := range routeSets[setName] {
			if strings.HasSuffix(rt.operation, httpSuffix) {
				continue
			}
			sdkRoutes++
			t.Run(rt.operation, func(t *testing.T) {
				checkRouteCall(t, rt, func(url string) {
					call, err := sdkCall(t.Context(), url, rt.operation)
					if err != nil {
						t.Fatalf("route %q: %v", rt.pattern, err)
					}
					call()
				})
			})
		}
	}
	if sdkRoutes == 0 {
		t.Errorf("found no SDK routes in the route sets %q", slices.Sorted(maps.Keys(routeSets)))
	}
}

// TestRoutesWithOnePatternShareAHandler checks that routes the fake serves once,
// because they have the same pattern, are also served the same way.
func TestRoutesWithOnePatternShareAHandler(t *testing.T) {
	handlers := map[string]uintptr{}
	shared := 0
	for _, setName := range slices.Sorted(maps.Keys(routeSets)) {
		for _, rt := range routeSets[setName] {
			handler := reflect.ValueOf(rt.handle).Pointer()
			first, seen := handlers[rt.pattern]
			switch {
			case !seen:
				handlers[rt.pattern] = handler
			case first != handler:
				t.Errorf("%s route %q for %s has another handler than an earlier route with "+
					"the same pattern", setName, rt.pattern, rt.operation)
			default:
				shared++
			}
		}
	}
	if shared == 0 {
		t.Errorf("found no routes that share a pattern; the check has nothing to check")
	}
}

// sdkCall returns a call of an SDK operation, written Group.Method, on a client of
// the mock at url. Its arguments are placeholders: ctx for the context, "x" for
// strings, 1 for integers, which the SDK's list methods take as page numbers and
// sizes, and zero values for the rest.
func sdkCall(ctx context.Context, url, operation string) (func(), error) {
	group, method, ok := strings.Cut(operation, ".")
	if !ok {
		return nil, fmt.Errorf("operation %q is not written Group.Method", operation)
	}
	client := permit.NewPermit(config.NewConfigBuilder(APIKey).WithApiUrl(url).Build())
	field, ok := reflect.TypeFor[*permit.Client]().Elem().FieldByName("Api")
	if !ok {
		return nil, fmt.Errorf("the SDK client has no Api field")
	}
	groupField, ok := field.Type.Elem().FieldByName(group)
	if !ok || !groupField.IsExported() {
		return nil, fmt.Errorf("the SDK client's Api has no group %q", group)
	}
	call := reflect.ValueOf(client.Api).Elem().FieldByIndex(groupField.Index).MethodByName(method)
	if !call.IsValid() {
		return nil, fmt.Errorf("the SDK's %s has no method %q", group, method)
	}
	callType := call.Type()
	if callType.IsVariadic() {
		return nil, fmt.Errorf("%s is variadic, which sdkCall does not support", operation)
	}
	args := make([]reflect.Value, callType.NumIn())
	for i := range args {
		argType := callType.In(i)
		switch {
		case argType == reflect.TypeFor[context.Context]():
			args[i] = reflect.ValueOf(ctx)
		case argType.Kind() == reflect.String:
			args[i] = reflect.ValueOf("x").Convert(argType)
		case argType.Kind() == reflect.Int:
			args[i] = reflect.ValueOf(1).Convert(argType)
		case argType.Kind() == reflect.Pointer:
			args[i] = reflect.New(argType.Elem())
		default:
			args[i] = reflect.Zero(argType)
		}
	}
	return func() { call.Call(args) }, nil
}

// callSite is a place in the provider's source that sends a request to the API.
type callSite struct {
	position string
	// pkg is the directory of the file, relative to the provider's source.
	pkg string
	// operation is Group.Method for a call through the SDK's Api, or
	// "<pkg>.<function> (HTTP)" for a request the provider builds itself.
	operation string
}

// TestProviderCallSitesHaveRoutes finds every call site in the provider's source
// that sends a request to the API, through the SDK or directly, and checks the
// route table against them. Every call site must have a route, and the tests of
// every package with a call site must start this mock. Every route must be for a
// call the provider makes, or else for an operation in releasedOnly, and the tests
// of the package that builds a request itself must check its routes with
// CheckHTTPRoutes. The test logs how many call sites and operations the routes
// cover.
func TestProviderCallSitesHaveRoutes(t *testing.T) {
	sites, tests := walkProvider(t)
	if len(sites) == 0 {
		t.Fatalf("found no call sites under %s", providerDir)
	}
	served := map[string]bool{}
	for _, set := range routeSets {
		for _, rt := range set {
			served[rt.operation] = true
		}
	}

	called := map[string]bool{}
	untested := map[string]bool{}
	coveredSites := 0
	for _, site := range sites {
		called[site.operation] = true
		if served[site.operation] {
			coveredSites++
		} else {
			t.Errorf("%s: %s has no route in mockpermit", site.position, site.operation)
		}
		if !tests.startMock[site.pkg] {
			untested[site.pkg] = true
		}
	}
	for _, pkg := range slices.Sorted(maps.Keys(untested)) {
		t.Errorf("%s calls the API, but none of its tests start mockpermit", pkg)
	}
	for _, operation := range slices.Sorted(maps.Keys(releasedOnly)) {
		if !served[operation] {
			t.Errorf("releasedOnly lists %q, which no route serves", operation)
		}
	}
	for _, setName := range slices.Sorted(maps.Keys(routeSets)) {
		for _, rt := range routeSets[setName] {
			switch {
			case !called[rt.operation] && !releasedOnly[rt.operation]:
				t.Errorf("%s route %q is for %q, which the provider never calls",
					setName, rt.pattern, rt.operation)
			case called[rt.operation] && releasedOnly[rt.operation]:
				t.Errorf("%s route %q is for %q, which the provider calls, so releasedOnly "+
					"must not list it", setName, rt.pattern, rt.operation)
			}
			pkg, _, _ := strings.Cut(rt.operation, ".")
			if strings.HasSuffix(rt.operation, httpSuffix) && !tests.checkHTTPRoutes[pkg] {
				t.Errorf("%s route %q is for %q, but none of the tests of %s check it with "+
					"mockpermit.CheckHTTPRoutes", setName, rt.pattern, rt.operation, pkg)
			}
		}
	}

	t.Logf("mockpermit has routes for %d of %d provider call sites (%d%%), of %d operations",
		coveredSites, len(sites), coveredSites*100/len(sites), len(called))
}

// providerTests are the provider packages whose tests call mockpermit functions.
type providerTests struct {
	// startMock has the packages whose tests call New.
	startMock map[string]bool
	// checkHTTPRoutes has the packages whose tests call CheckHTTPRoutes.
	checkHTTPRoutes map[string]bool
}

// walkProvider parses the provider's source and returns its call sites and the
// packages whose tests start this mock or check HTTP routes with it. It fails the
// test on any use of an Api field it cannot attribute to an operation.
func walkProvider(t *testing.T) ([]callSite, providerTests) {
	t.Helper()
	fset := token.NewFileSet()
	var sites []callSite
	tests := providerTests{startMock: map[string]bool{}, checkHTTPRoutes: map[string]bool{}}
	err := filepath.WalkDir(providerDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == "testdata" {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		pkg, err := filepath.Rel(providerDir, filepath.Dir(path))
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, "_test.go") {
			if callsMock(file, "New") {
				tests.startMock[pkg] = true
			}
			if callsMock(file, "CheckHTTPRoutes") {
				tests.checkHTTPRoutes[pkg] = true
			}
			return nil
		}
		fileSites, unattributed := callSites(fset, file, pkg)
		sites = append(sites, fileSites...)
		for _, position := range unattributed {
			t.Errorf("%s: a use of the SDK's Api that is not a call written "+
				"x.Api.Group.Method(...); teach callSites to read it", position)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", providerDir, err)
	}
	return sites, tests
}

// callSites returns the call sites in file: calls written x.Api.Group.Method(...),
// and calls of the net/http functions that build or send a request, named after
// the function they are in ("init" outside a function). It also returns the
// positions of the Api fields file uses in any other way.
func callSites(fset *token.FileSet, file *ast.File, pkg string) ([]callSite, []string) {
	var sites []callSite
	attributed := map[*ast.SelectorExpr]bool{}
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
			site := callSite{position: fset.Position(call.Pos()).String(), pkg: pkg}
			if api, operation, ok := sdkOperation(call); ok {
				attributed[api] = true
				site.operation = operation
				sites = append(sites, site)
			} else if isHTTPRequest(call) {
				site.operation = pkg + "." + function + " (HTTP)"
				sites = append(sites, site)
			}
			return true
		})
	}
	var unattributed []string
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "Api" && !attributed[selector] {
			unattributed = append(unattributed, fset.Position(selector.Pos()).String())
		}
		return true
	})
	return sites, unattributed
}

// sdkOperation returns the Group.Method of a call written x.Api.Group.Method(...)
// and its x.Api selector.
func sdkOperation(call *ast.CallExpr) (*ast.SelectorExpr, string, bool) {
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, "", false
	}
	group, ok := method.X.(*ast.SelectorExpr)
	if !ok {
		return nil, "", false
	}
	api, ok := group.X.(*ast.SelectorExpr)
	if !ok || api.Sel.Name != "Api" {
		return nil, "", false
	}
	return api, group.Sel.Name + "." + method.Sel.Name, true
}

// isHTTPRequest reports whether call is a net/http function that builds or sends
// a request.
func isHTTPRequest(call *ast.CallExpr) bool {
	function, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := function.X.(*ast.Ident)
	return ok && pkg.Name == "http" && slices.Contains(
		[]string{"NewRequest", "NewRequestWithContext", "Get", "Head", "Post", "PostForm"},
		function.Sel.Name)
}

// callsMock reports whether file calls the mockpermit function with this name.
func callsMock(file *ast.File, name string) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return !found
		}
		function, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return !found
		}
		pkg, ok := function.X.(*ast.Ident)
		if ok && pkg.Name == "mockpermit" && function.Sel.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// TestCallSites checks how the walk reads a source file: which calls it counts,
// what it names them, and which uses of an Api field it reports as unattributed.
func TestCallSites(t *testing.T) {
	const source = `package sample

func (c *client) read(ctx context.Context, key string) {
	c.client.Api.Resources.Get(ctx, key)
	api := c.client.Api
	api.Roles.Get(ctx, key)
	c.client.Check(user, action, resource)
}

func send(ctx context.Context) {
	http.NewRequestWithContext(ctx, "POST", "u", nil)
	http.Get("u")
	other.NewRequest("GET", "u", nil)
}

var request, _ = http.NewRequest("GET", "u", nil)
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", source, 0)
	if err != nil {
		t.Fatalf("parsing the sample: %v", err)
	}

	sites, unattributed := callSites(fset, file, "sample")

	var got []string
	for _, site := range sites {
		got = append(got, site.position+" "+site.pkg+" "+site.operation)
	}
	want := []string{
		"sample.go:4:2 sample Resources.Get",
		"sample.go:11:2 sample sample.send (HTTP)",
		"sample.go:12:2 sample sample.send (HTTP)",
		"sample.go:16:18 sample sample.init (HTTP)",
	}
	if !slices.Equal(got, want) {
		t.Errorf("call sites = %q, want %q", got, want)
	}
	if want := []string{"sample.go:5:9"}; !slices.Equal(unattributed, want) {
		t.Errorf("unattributed Api uses = %q, want %q", unattributed, want)
	}
}

func TestCallsMock(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		function string
		want     bool
	}{
		{
			name: "starts the mock", body: `m := mockpermit.New(t, mockpermit.Tenants)`,
			function: "New", want: true,
		},
		{name: "uses the mock package only", body: `id := mockpermit.ObjectID(1)`, function: "New"},
		{name: "starts another package's New", body: `m := other.New(t)`, function: "New"},
		{
			name: "checks HTTP routes", body: `mockpermit.CheckHTTPRoutes(t, set, calls)`,
			function: "CheckHTTPRoutes", want: true,
		},
		{
			name: "starts the mock without checking HTTP routes", body: `mockpermit.New(t)`,
			function: "CheckHTTPRoutes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "package sample_test\n\nfunc TestSample(t *testing.T) {\n\t" +
				tt.body + "\n}\n"
			file, err := parser.ParseFile(token.NewFileSet(), "sample_test.go", source, 0)
			if err != nil {
				t.Fatalf("parsing the sample: %v", err)
			}
			if got := callsMock(file, tt.function); got != tt.want {
				t.Errorf("callsMock(%q) = %v, want %v", tt.function, got, tt.want)
			}
		})
	}
}
