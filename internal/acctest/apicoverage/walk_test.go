package main

import (
	"go/ast"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/pkg/models"
	"golang.org/x/tools/go/packages"
)

// TestGatherFacts walks the provider in this module and checks what it finds for a
// call of each kind: an SDK Api method, an operation of the SDK's generated
// client, and a request the provider builds itself.
func TestGatherFacts(t *testing.T) {
	f, err := gatherFacts("../../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.problems) > 0 {
		t.Errorf("problems: %q", f.problems)
	}
	if failed := factSentinels(f, defaultMinimums); len(failed) > 0 {
		t.Errorf("sentinels: %q", failed)
	}

	sites := map[string]callSite{}
	for _, site := range f.sites {
		sites[site.call] = site
	}
	tests := []struct {
		call   string
		pkg    string
		sdk    bool
		routes []route
	}{
		{
			call: "Tenants.Update", pkg: "tenants", sdk: true,
			routes: []route{{"PATCH", "/v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}"}},
		},
		{
			call: "ProxyConfigs.Create", pkg: "proxy_configs", sdk: true,
			routes: []route{{"POST", "/v2/facts/{proj_id}/{env_id}/proxy_configs"}},
		},
		{
			call: "APIKeysApiService.GetApiKeyScope", pkg: ".", sdk: true,
			routes: []route{{"GET", "/v2/api-key/scope"}},
		},
		{
			call: "proxy_configs.update (HTTP)", pkg: "proxy_configs",
			routes: []route{
				{"PATCH", "/v2/facts/{proj_id}/{env_id}/proxy_configs/{proxy_config_id}"},
			},
		},
	}
	for _, tt := range tests {
		site, ok := sites[tt.call]
		if !ok {
			t.Errorf("found no call site of %s", tt.call)
			continue
		}
		if site.pkg != tt.pkg || site.sdk != tt.sdk || !slices.Equal(site.routes, tt.routes) {
			t.Errorf("%s: package %s, SDK %v, routes %v; want %s, %v, %v", tt.call, site.pkg,
				site.sdk, site.routes, tt.pkg, tt.sdk, tt.routes)
		}
		if len(site.wire) == 0 {
			t.Errorf("%s: no offline wire route", tt.call)
		}
	}

	tenant := route{"GET", "/v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}"}.key()
	if got, want := f.models[tenant], reflect.TypeFor[*models.TenantRead](); got != want {
		t.Errorf("the tenant read decodes into %v, want %v", got, want)
	}
	if want := []string{"permitio_tenant"}; !slices.Equal(f.surfaces["tenants"], want) {
		t.Errorf("the tenants package registers %q, want %q", f.surfaces["tenants"], want)
	}
	if want := []string{"permitio_resource", "data.permitio_resource"}; !slices.Equal(
		f.surfaces["resources"], want) {
		t.Errorf("the resources package registers %q, want %q", f.surfaces["resources"], want)
	}
}

// TestWalkPackage walks testdata/walk with a resolver that knows no SDK source, so
// no Api method call can be followed to the requests it sends.
func TestWalkPackage(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: "testdata/walk",
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || len(pkgs[0].Errors) > 0 {
		t.Fatalf("loaded %d packages, errors %v", len(pkgs), pkgs[0].Errors)
	}
	resolver := &sdkResolver{decls: map[string]*ast.FuncDecl{}, memo: map[string][]string{}}

	sites, problems := walkPackage(pkgs[0], ".", resolver, map[string]route{})

	want := []string{
		"testdata/walk/walk.go:13:12: found no request that Tenants.Get sends in the SDK's source",
		"testdata/walk/walk.go:18:20: Delete is used but not called directly, so its requests " +
			"cannot be told",
	}
	if !slices.Equal(problems, want) {
		t.Errorf("problems = %q, want %q", problems, want)
	}
	if len(sites) != 1 || !strings.HasSuffix(sites[0].call, "/walk.send (HTTP)") || sites[0].sdk {
		t.Errorf("sites = %+v, want the net/http request in send only", sites)
	}
}

func TestAttachMockRoutes(t *testing.T) {
	scope := route{"GET", "/v2/api-key/scope"}
	thing := route{"GET", "/v2/things/{thing_id}"}
	sites := []callSite{
		{position: "a.go:1:1", call: "Things.Get", sdk: true,
			routes: []route{{"GET", "/v2/things/{id}"}}},
		{position: "a.go:2:1", call: "Things.List", sdk: true,
			routes: []route{{"GET", "/v2/things"}}},
		{position: "a.go:3:1", call: "things.send (HTTP)"},
		{position: "a.go:4:1", call: "things.other (HTTP)"},
		{position: "a.go:5:1", call: "APIKeysApiService.GetApiKeyScope", sdk: true,
			routes: []route{scope}},
		{position: "a.go:6:1", call: "ThingsApiService.DeleteThing", sdk: true,
			routes: []route{thing}},
	}
	mockRoutes := map[string][]route{
		"Things.Get":         {thing},
		"Things.List":        {{"GET", "/v2/things/all"}},
		"things.send (HTTP)": {thing},
	}

	problems := attachMockRoutes(sites, mockRoutes, scope)

	want := []string{
		"a.go:2:1: mockpermit serves Things.List on GET /v2/things/all, but the SDK's source " +
			"sends GET /v2/things",
		"a.go:4:1: mockpermit has no route for things.other (HTTP), so the request it sends " +
			"is unknown",
		"a.go:6:1: mockpermit has no route for ThingsApiService.DeleteThing, so the offline " +
			"tests cannot serve it; add one to its route table",
	}
	if !slices.Equal(problems, want) {
		t.Errorf("problems = %q, want %q", problems, want)
	}
	if !slices.Equal(sites[0].wire, []route{thing}) {
		t.Errorf("the SDK call's wire routes = %v, want %v", sites[0].wire, thing)
	}
	if !slices.Equal(sites[2].routes, []route{thing}) {
		t.Errorf("the HTTP call's routes = %v, want mockpermit's %v", sites[2].routes, thing)
	}
	if !slices.Equal(sites[4].wire, []route{scope}) {
		t.Errorf("the scope call's wire routes = %v, want %v", sites[4].wire, scope)
	}
}
