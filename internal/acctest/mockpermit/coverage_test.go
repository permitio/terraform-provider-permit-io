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
	"ImplicitGrants":     ImplicitGrants,
	"ResourceAttributes": ResourceAttributes,
	"ResourceRelations":  ResourceRelations,
	"ResourceRoles":      ResourceRoles,
	"Resources":          Resources,
	"Roles":              Roles,
	"Tenants":            Tenants,
}

// providerDir is the provider's source, relative to this package.
const providerDir = "../../provider"

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
// request only has to reach the route, not succeed.
func TestRoutesMatchTheSDK(t *testing.T) {
	for _, setName := range slices.Sorted(maps.Keys(routeSets)) {
		for _, rt := range routeSets[setName] {
			t.Run(rt.operation, func(t *testing.T) {
				rec := &errorRecorder{TB: t}
				m := New(rec, Routes{rt})
				call, err := sdkCall(t.Context(), m.URL, rt.operation)
				if err != nil {
					t.Fatalf("route %q: %v", rt.pattern, err)
				}

				call()

				m.mu.Lock()
				var sent []string
				for _, request := range m.requests {
					if request.Path != scopePath {
						sent = append(sent, request.Method+" "+request.Path)
					}
				}
				hits := m.hits[rt.pattern]
				m.mu.Unlock()
				if len(sent) != 1 || hits != 1 {
					t.Errorf("%s sent %q; want one request, to %q", rt.operation, sent, rt.pattern)
				}
				if errs := rec.take(); len(errs) > 0 {
					t.Errorf("%s: the mock failed the request: %q", rt.operation, errs)
				}
			})
		}
	}
}

// sdkCall returns a call of an SDK operation, written Group.Method, on a client of
// the mock at url. Its arguments are placeholders: ctx for the context, "x" for
// strings, and zero values for the rest.
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
// route table against them. A package whose tests use this mock must have a route
// for every call it makes, and every route must be for a call the provider makes.
// The test logs how many call sites the routes cover.
func TestProviderCallSitesHaveRoutes(t *testing.T) {
	sites, tested := walkProvider(t)
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
	coveredSites := 0
	uncovered := map[string]bool{}
	for _, site := range sites {
		called[site.operation] = true
		if served[site.operation] {
			coveredSites++
			continue
		}
		uncovered[site.pkg+": "+site.operation] = true
		if tested[site.pkg] {
			t.Errorf("%s: %s has no route in mockpermit, but the tests of %s use the mock",
				site.position, site.operation, site.pkg)
		}
	}
	for _, setName := range slices.Sorted(maps.Keys(routeSets)) {
		for _, rt := range routeSets[setName] {
			if !called[rt.operation] {
				t.Errorf("%s route %q is for %q, which the provider never calls",
					setName, rt.pattern, rt.operation)
			}
		}
	}

	coveredOperations := 0
	for operation := range called {
		if served[operation] {
			coveredOperations++
		}
	}
	t.Logf("mockpermit has routes for %d of %d provider call sites (%d%%) and %d of %d "+
		"operations", coveredSites, len(sites), coveredSites*100/len(sites), coveredOperations,
		len(called))
	t.Logf("operations without a route:\n%s",
		strings.Join(slices.Sorted(maps.Keys(uncovered)), "\n"))
}

// walkProvider parses the provider's source and returns its call sites and the
// packages whose tests start this mock. It fails the test on any use of an Api
// field it cannot attribute to an operation.
func walkProvider(t *testing.T) ([]callSite, map[string]bool) {
	t.Helper()
	fset := token.NewFileSet()
	var sites []callSite
	tested := map[string]bool{}
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
			if startsMock(file) {
				tested[pkg] = true
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
	return sites, tested
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

// startsMock reports whether file calls mockpermit.New.
func startsMock(file *ast.File) bool {
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
		if ok && pkg.Name == "mockpermit" && function.Sel.Name == "New" {
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

func TestStartsMock(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "starts the mock", body: `m := mockpermit.New(t, mockpermit.Tenants)`, want: true},
		{name: "uses the mock package only", body: `id := mockpermit.ObjectID(1)`},
		{name: "starts another package's New", body: `m := other.New(t)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "package sample_test\n\nfunc TestSample(t *testing.T) {\n\t" +
				tt.body + "\n}\n"
			file, err := parser.ParseFile(token.NewFileSet(), "sample_test.go", source, 0)
			if err != nil {
				t.Fatalf("parsing the sample: %v", err)
			}
			if got := startsMock(file); got != tt.want {
				t.Errorf("startsMock() = %v, want %v", got, tt.want)
			}
		})
	}
}
