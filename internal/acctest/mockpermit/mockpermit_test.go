package mockpermit

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const tenantsPath = "/v2/facts/" + ProjectID + "/" + EnvironmentID + "/tenants"

// call sends a request to the mock and returns the status code and body.
func call(t *testing.T, m *Server, method, path, authorization, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, m.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building %s %s: %v", method, path, err)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: reading the response: %v", method, path, err)
	}
	return resp.StatusCode, string(respBody)
}

const bearer = "Bearer " + APIKey

func TestNewPointsTheProviderAtTheMock(t *testing.T) {
	m := New(t)
	if got := os.Getenv("PERMITIO_API_URL"); got != m.URL || got == "" {
		t.Errorf("PERMITIO_API_URL = %q, want the mock URL %q", got, m.URL)
	}
	if got := os.Getenv("PERMITIO_API_KEY"); got != APIKey {
		t.Errorf("PERMITIO_API_KEY = %q, want %q", got, APIKey)
	}
}

func TestAPIKeyScope(t *testing.T) {
	m := New(t)
	status, body := call(t, m, http.MethodGet, "/v2/api-key/scope", bearer, "")
	if status != http.StatusOK {
		t.Fatalf("GET /v2/api-key/scope: status %d, want 200; body %s", status, body)
	}
	var scope map[string]string
	if err := json.Unmarshal([]byte(body), &scope); err != nil {
		t.Fatalf("scope body %q is not a JSON object of strings: %v", body, err)
	}
	want := map[string]string{
		"organization_id": OrganizationID, "project_id": ProjectID, "environment_id": EnvironmentID,
	}
	if !maps.Equal(scope, want) {
		t.Errorf("scope = %v, want %v", scope, want)
	}
}

func TestUnservedRequestsFailTheTest(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantError  string
	}{
		{
			name: "unknown path", method: http.MethodGet, path: tenantsPath + "/acme/users",
			wantStatus: http.StatusNotImplemented,
			wantError:  "no route for GET " + tenantsPath + "/acme/users",
		},
		{
			name: "known path, unserved method", method: http.MethodPut, path: tenantsPath + "/acme",
			wantStatus: http.StatusNotImplemented,
			wantError:  "no route for PUT " + tenantsPath + "/acme",
		},
		{
			name: "project outside the API key scope", method: http.MethodGet,
			path:       "/v2/facts/other-project/" + EnvironmentID + "/tenants/acme",
			wantStatus: http.StatusNotFound,
			wantError:  `proj_id = "other-project"`,
		},
		{
			name: "environment outside the API key scope", method: http.MethodDelete,
			path:       "/v2/facts/" + ProjectID + "/other-env/tenants/acme",
			wantStatus: http.StatusNotFound,
			wantError:  `env_id = "other-env"`,
		},
		{
			name: "empty path segment", method: http.MethodGet, path: tenantsPath + "//users",
			wantStatus: http.StatusBadRequest,
			wantError:  "GET " + tenantsPath + "//users: the path has an empty or dot segment",
		},
		{
			name: "dot path segment", method: http.MethodGet, path: tenantsPath + "/./acme",
			wantStatus: http.StatusBadRequest,
			wantError:  "GET " + tenantsPath + "/./acme: the path has an empty or dot segment",
		},
		{
			name: "empty segment in a served route", method: http.MethodDelete,
			path:       "/v2/facts/" + ProjectID + "//" + EnvironmentID + "/tenants/acme",
			wantStatus: http.StatusBadRequest,
			wantError:  "the path has an empty or dot segment",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := New(rec, Tenants)

			status, _ := call(t, m, tt.method, tt.path, bearer, "")

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			got := rec.take()
			if len(got) != 1 || !strings.Contains(got[0], tt.wantError) {
				t.Errorf("test errors = %q, want one containing %q", got, tt.wantError)
			}
		})
	}
}

// TestCleanPathMatchesServeMux checks that cleanPath flags exactly the paths that
// http.ServeMux would redirect instead of routing, so none of them reach a route.
func TestCleanPathMatchesServeMux(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})
	paths := []string{
		"/", "/a", "/a/", "/a/b", "//a", "/a//b", "/a/b//", "/a/./b", "/a/../b", "/a/.",
		"/a/..", "/a/./", "/a/%2F/b", "/a/%2e/b",
	}
	for _, p := range paths {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, p, nil))
		redirected := w.Code >= 300 && w.Code < 400
		if flagged := cleanPath(p) != p; flagged != redirected {
			t.Errorf("%s: cleanPath flags it = %v, ServeMux redirects it = %v (status %d)",
				p, flagged, redirected, w.Code)
		}
	}
}

func TestRoutesAreServedOnlyWhenRequested(t *testing.T) {
	rec := &errorRecorder{TB: t}
	m := New(rec)

	status, _ := call(t, m, http.MethodGet, tenantsPath+"/acme", bearer, "")

	if status != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501 from a mock started without Tenants", status)
	}
	if got := rec.take(); len(got) != 1 {
		t.Errorf("test errors = %q, want one for the unserved tenant route", got)
	}
}

// TestAllRouteSetsTogether checks that every route set can be served by one mock,
// which a test with every resource type needs: none of their patterns conflict.
func TestAllRouteSetsTogether(t *testing.T) {
	var sets []Routes
	for _, name := range slices.Sorted(maps.Keys(routeSets)) {
		sets = append(sets, routeSets[name])
	}
	m := New(t, sets...)

	send(t, m, http.MethodPost, resourcesPath, `{"key": "file", "name": "File", "actions": {}}`,
		http.StatusOK)
	send(t, m, http.MethodPost, rolesPath, `{"key": "admin", "name": "Admin"}`, http.StatusOK)

	wantStoredKeys(t, m, "resources/file", "roles/admin")
}

func TestWrongAuthorizationFailsTheTest(t *testing.T) {
	for _, authorization := range []string{"", "bearer " + APIKey, "Bearer other-key", APIKey} {
		t.Run(fmt.Sprintf("%q", authorization), func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := New(rec)

			status, _ := call(t, m, http.MethodGet, "/v2/api-key/scope", authorization, "")

			if status != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", status)
			}
			got := rec.take()
			want := fmt.Sprintf("Authorization header = %q, want %q", authorization, bearer)
			if len(got) != 1 || !strings.Contains(got[0], want) {
				t.Errorf("test errors = %q, want one containing %q", got, want)
			}
		})
	}
}

func TestRequestsAreRecorded(t *testing.T) {
	m := New(t, Tenants)
	body := `{"key":"acme","name":"Acme"}`

	call(t, m, http.MethodPost, tenantsPath+"?page=2", bearer, body)
	call(t, m, http.MethodGet, tenantsPath+"/acme", bearer, "")
	call(t, m, http.MethodGet, tenantsPath+"/other", bearer, "")

	posts := m.Requests(http.MethodPost, tenantsPath)
	if len(posts) != 1 {
		t.Fatalf("POST %s: recorded %d requests, want 1", tenantsPath, len(posts))
	}
	got := posts[0]
	if got.RawQuery != "page=2" {
		t.Errorf("RawQuery = %q, want %q", got.RawQuery, "page=2")
	}
	if got.Header.Get("Authorization") != bearer {
		t.Errorf("Authorization header = %q, want %q", got.Header.Get("Authorization"), bearer)
	}
	if string(got.Body) != body {
		t.Errorf("Body = %q, want %q", got.Body, body)
	}
	for _, key := range []string{"acme", "other"} {
		if gets := m.Requests(http.MethodGet, tenantsPath+"/"+key); len(gets) != 1 {
			t.Errorf("GET %s/%s: recorded %d requests, want 1", tenantsPath, key, len(gets))
		}
	}
	if other := m.Requests(http.MethodDelete, tenantsPath+"/acme"); len(other) != 0 {
		t.Errorf("DELETE %s/acme: recorded %d requests, want 0", tenantsPath, len(other))
	}
}

func TestAssertAllRoutesHit(t *testing.T) {
	rec := &errorRecorder{TB: t}
	m := New(rec, Tenants)
	call(t, m, http.MethodGet, "/v2/api-key/scope", bearer, "")

	m.AssertAllRoutesHit()

	unhit := rec.take()
	if len(unhit) != 1 {
		t.Fatalf("test errors = %q, want one listing the unhit routes", unhit)
	}
	for _, rt := range Tenants {
		if !strings.Contains(unhit[0], rt.pattern) {
			t.Errorf("error %q does not name unhit route %q", unhit[0], rt.pattern)
		}
	}
	if strings.Contains(unhit[0], "/v2/api-key/scope") {
		t.Errorf("error %q names the scope route, which was hit", unhit[0])
	}

	call(t, m, http.MethodPost, tenantsPath, bearer, `{"key":"acme","name":"Acme"}`)
	call(t, m, http.MethodGet, tenantsPath+"/acme", bearer, "")
	call(t, m, http.MethodPatch, tenantsPath+"/acme", bearer, `{"name":"Acme 2"}`)
	call(t, m, http.MethodDelete, tenantsPath+"/acme", bearer, "")

	m.AssertAllRoutesHit()

	if got := rec.take(); len(got) != 0 {
		t.Errorf("test errors = %q after every route was hit, want none", got)
	}
}

// TestAssertRoutesHit checks that AssertRoutesHit requires only the routes of the
// sets it names, while AssertAllRoutesHit still requires every route served.
func TestAssertRoutesHit(t *testing.T) {
	rec := &errorRecorder{TB: t}
	m := New(rec, Tenants, Resources)
	call(t, m, http.MethodPost, tenantsPath, bearer, `{"key":"acme","name":"Acme"}`)
	call(t, m, http.MethodGet, tenantsPath+"/acme", bearer, "")
	call(t, m, http.MethodPatch, tenantsPath+"/acme", bearer, `{"name":"Acme 2"}`)

	m.AssertRoutesHit(Tenants, Resources)

	got := rec.take()
	if len(got) != 1 || !strings.Contains(got[0], "routes never called") {
		t.Fatalf("test errors = %q, want one listing the unhit routes", got)
	}
	for _, rt := range append(Routes{routeFor(t, Tenants, "Tenants.Delete")}, Resources...) {
		if !strings.Contains(got[0], `"`+rt.pattern+`"`) {
			t.Errorf("error %q does not name unhit route %q", got[0], rt.pattern)
		}
	}
	for _, operation := range []string{"Tenants.Create", "Tenants.Get", "Tenants.Update"} {
		if rt := routeFor(t, Tenants, operation); strings.Contains(got[0], `"`+rt.pattern+`"`) {
			t.Errorf("error %q names route %q, which was hit", got[0], rt.pattern)
		}
	}

	call(t, m, http.MethodDelete, tenantsPath+"/acme", bearer, "")
	m.AssertRoutesHit(Tenants)

	if got := rec.take(); len(got) != 0 {
		t.Errorf("test errors = %q after every tenant route was hit, want none", got)
	}

	m.AssertAllRoutesHit()

	if got := rec.take(); len(got) != 1 || !strings.Contains(got[0], Resources[0].pattern) {
		t.Errorf("AssertAllRoutesHit test errors = %q, want one naming the resource routes", got)
	}
}

// TestAssertRoutesHitRejectsBadArguments checks that AssertRoutesHit fails the
// test when it has no routes to check, or names routes the mock does not serve,
// even though every route the mock serves was hit.
func TestAssertRoutesHitRejectsBadArguments(t *testing.T) {
	tests := []struct {
		name      string
		sets      []Routes
		wantError string
	}{
		{name: "no sets", wantError: "given no routes to check"},
		{name: "an empty set", sets: []Routes{{}}, wantError: "given no routes to check"},
		{
			name: "a set the mock does not serve", sets: []Routes{Tenants, Roles},
			wantError: "names routes the mock does not serve",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := New(rec, Tenants)
			call(t, m, http.MethodGet, "/v2/api-key/scope", bearer, "")
			call(t, m, http.MethodPost, tenantsPath, bearer, `{"key":"acme","name":"Acme"}`)
			call(t, m, http.MethodGet, tenantsPath+"/acme", bearer, "")
			call(t, m, http.MethodPatch, tenantsPath+"/acme", bearer, `{"name":"Acme 2"}`)
			call(t, m, http.MethodDelete, tenantsPath+"/acme", bearer, "")
			m.AssertAllRoutesHit()
			if got := rec.take(); len(got) != 0 {
				t.Fatalf("test errors = %q after every route was hit, want none", got)
			}

			m.AssertRoutesHit(tt.sets...)

			got := rec.take()
			if len(got) != 1 || !strings.Contains(got[0], tt.wantError) {
				t.Errorf("test errors = %q, want one containing %q", got, tt.wantError)
			}
		})
	}
}

// routeFor returns the route in set for an SDK operation, such as "Tenants.Get".
func routeFor(t *testing.T, set Routes, operation string) route {
	t.Helper()
	for _, rt := range set {
		if rt.operation == operation {
			return rt
		}
	}
	t.Fatalf("no route in the set for %s", operation)
	return route{}
}

// TestCollectionsAreCreatedOnFirstUse checks that a route set can store objects in
// a collection of its own without New knowing about it.
func TestCollectionsAreCreatedOnFirstUse(t *testing.T) {
	m := New(t)

	m.mu.Lock()
	m.collection("roles")["viewer"] = map[string]any{"key": "viewer"}
	m.collection("tenants")["acme"] = map[string]any{"key": "acme"}
	m.collection("roles")["editor"] = map[string]any{"key": "editor"}
	m.mu.Unlock()

	want := []string{"roles/editor", "roles/viewer", "tenants/acme"}
	if got := m.StoredKeys(); !slices.Equal(got, want) {
		t.Errorf("StoredKeys() = %q, want %q", got, want)
	}
}

func TestTenantState(t *testing.T) {
	m := New(t, Tenants)
	check := func(method, path, body string, wantStatus int, wantBody map[string]any) {
		t.Helper()
		status, got := call(t, m, method, path, bearer, body)
		if status != wantStatus {
			t.Fatalf("%s %s: status %d, want %d; body %s", method, path, status, wantStatus, got)
		}
		if wantBody == nil {
			return
		}
		var gotBody map[string]any
		if err := json.Unmarshal([]byte(got), &gotBody); err != nil {
			t.Fatalf("%s %s: body %q is not a JSON object: %v", method, path, got, err)
		}
		for field, want := range wantBody {
			if fmt.Sprint(gotBody[field]) != fmt.Sprint(want) {
				t.Errorf("%s %s: %s = %v, want %v", method, path, field, gotBody[field], want)
			}
		}
	}
	created := map[string]any{
		"id": "00000000-0000-4000-8000-000000000001", "key": "acme", "name": "Acme",
		"description": "d", "organization_id": OrganizationID, "project_id": ProjectID,
		"environment_id": EnvironmentID, "created_at": timestamp, "last_action_at": timestamp,
		"attributes": map[string]any{},
	}

	other := map[string]any{
		"id": "00000000-0000-4000-8000-000000000002", "key": "beta", "name": "Beta",
	}

	check(http.MethodPost, tenantsPath, `{"key":"acme","name":"Acme","description":"d"}`,
		http.StatusOK, created)
	check(http.MethodPost, tenantsPath, `{"key":"beta","name":"Beta"}`, http.StatusOK, other)
	check(http.MethodGet, tenantsPath+"/acme", "", http.StatusOK, created)
	check(http.MethodGet, tenantsPath+"/beta", "", http.StatusOK, other)
	check(http.MethodGet, tenantsPath+"/missing", "", http.StatusNotFound, nil)
	check(http.MethodPost, tenantsPath, `{"key":"acme","name":"Again"}`, http.StatusConflict, nil)
	check(http.MethodPatch, tenantsPath+"/acme", `{"key":"acme2"}`,
		http.StatusUnprocessableEntity, nil)
	check(http.MethodPatch, tenantsPath+"/acme", `{"attributes":"{}"}`,
		http.StatusUnprocessableEntity, nil)
	check(http.MethodGet, tenantsPath+"/acme", "", http.StatusOK, created)
	check(http.MethodPatch, tenantsPath+"/acme", `{"name":"Acme 2"}`, http.StatusOK,
		map[string]any{"name": "Acme 2", "description": "d"})
	check(http.MethodGet, tenantsPath+"/acme", "", http.StatusOK,
		map[string]any{"name": "Acme 2", "description": "d"})
	check(http.MethodGet, tenantsPath+"/beta", "", http.StatusOK, other)
	want := []string{"tenants/acme", "tenants/beta"}
	if got := m.StoredKeys(); !slices.Equal(got, want) {
		t.Errorf("StoredKeys() = %q, want %q", got, want)
	}

	check(http.MethodDelete, tenantsPath+"/acme", "", http.StatusNoContent, nil)

	check(http.MethodGet, tenantsPath+"/acme", "", http.StatusNotFound, nil)
	check(http.MethodPatch, tenantsPath+"/acme", `{"name":"x"}`, http.StatusNotFound, nil)
	check(http.MethodDelete, tenantsPath+"/acme", "", http.StatusNotFound, nil)
	check(http.MethodGet, tenantsPath+"/beta", "", http.StatusOK, other)
	if got, want := m.StoredKeys(), []string{"tenants/beta"}; !slices.Equal(got, want) {
		t.Errorf("StoredKeys() = %q after deleting acme, want %q", got, want)
	}

	check(http.MethodDelete, tenantsPath+"/beta", "", http.StatusNoContent, nil)

	if got := m.StoredKeys(); len(got) != 0 {
		t.Errorf("StoredKeys() = %q after deleting every tenant, want none", got)
	}
}

func TestTenantList(t *testing.T) {
	m := New(t, Tenants, TenantList)
	wantIDs(t, sendForList(t, m, tenantsPath, http.StatusOK))
	for _, key := range []string{"acme", "beta", "gamma"} {
		send(t, m, http.MethodPost, tenantsPath, `{"key": "`+key+`", "name": "`+key+`"}`,
			http.StatusOK)
	}

	all := sendForList(t, m, tenantsPath, http.StatusOK)

	wantIDs(t, all, ObjectID(1), ObjectID(2), ObjectID(3))
	acme := send(t, m, http.MethodGet, tenantsPath+"/acme", "", http.StatusOK)
	if !reflect.DeepEqual(all[0], acme) {
		t.Errorf("listed %v, want the tenant as stored: %v", all[0], acme)
	}
	wantIDs(t, sendForList(t, m, tenantsPath+"?page=2&per_page=2", http.StatusOK), ObjectID(3))
	wantIDs(t, sendForList(t, m, tenantsPath+"?page=3&per_page=2", http.StatusOK))
	wantIDs(t, sendForList(t, m, tenantsPath+"?page=1&per_page=1", http.StatusOK), ObjectID(1))
	sendForList(t, m, tenantsPath+"?per_page=101", http.StatusUnprocessableEntity)
	sendForList(t, m, tenantsPath+"?page=x", http.StatusUnprocessableEntity)
}

func TestTenantBodyErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantErrors int
	}{
		{name: "not JSON", body: `key=acme`, wantStatus: http.StatusUnprocessableEntity, wantErrors: 1},
		{name: "JSON null", body: `null`, wantStatus: http.StatusUnprocessableEntity, wantErrors: 1},
		{name: "no key", body: `{"name":"Acme"}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "no name", body: `{"key":"acme"}`, wantStatus: http.StatusUnprocessableEntity},
		{
			name: "an unknown field", body: `{"key":"acme","name":"Acme","tier":"gold"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "attributes not an object", body: `{"key":"acme","name":"Acme","attributes":[]}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := New(rec, Tenants)

			status, _ := call(t, m, http.MethodPost, tenantsPath, bearer, tt.body)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if got := rec.take(); len(got) != tt.wantErrors {
				t.Errorf("test errors = %q, want %d", got, tt.wantErrors)
			}
			if keys := m.StoredKeys(); len(keys) != 0 {
				t.Errorf("StoredKeys() = %q, want none after a rejected create", keys)
			}
		})
	}
}

func TestCheckJSONBody(t *testing.T) {
	sent := Request{Method: http.MethodPatch, Path: "/x", Body: []byte(`{"b":{"c":1},"a":"x"}`)}
	tests := []struct {
		name      string
		request   Request
		want      string
		wantError string
	}{
		{
			name:    "same value, other key order and spacing",
			request: sent, want: `{ "a": "x", "b": {"c": 1} }`,
		},
		{
			name: "missing field", request: sent, want: `{"a":"x","b":{"c":1},"d":null}`,
			wantError: `PATCH /x: request body = {"a":"x","b":{"c":1}}, want {"a":"x","b":{"c":1},"d":null}`,
		},
		{name: "changed nested value", request: sent, want: `{"a":"x","b":{"c":2}}`, wantError: "want"},
		{
			name:    "body not JSON",
			request: Request{Method: http.MethodPost, Path: "/x", Body: []byte("key=acme")},
			want:    `{}`, wantError: "is not JSON",
		},
		{name: "want not JSON", request: sent, want: `{`, wantError: "want"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.CheckJSONBody(tt.want)
			if tt.wantError == "" {
				if err != nil {
					t.Errorf("CheckJSONBody() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("CheckJSONBody() = %v, want an error containing %q", err, tt.wantError)
			}
		})
	}
}
