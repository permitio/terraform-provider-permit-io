package mockpermit

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

const (
	developerRolesPath = schemaPath + "/groups/developers/roles"
	editorOfWS1        = `{"role": "editor", "resource": "workspace", "resource_instance": "ws-1",
		"tenant": "acme"}`
	viewerOfWS1 = `{"role": "viewer", "resource": "workspace", "resource_instance": "ws-1",
		"tenant": "acme"}`
)

// newGroupFixture returns a mock with the workspace resource (ID 1), its editor and
// viewer roles (2 and 3), the acme and beta tenants (4 and 5), the ws-1 workspace
// of acme (6) and the developers group of acme (7).
func newGroupFixture(t *testing.T, tb testing.TB) *Server {
	t.Helper()
	m := New(tb, Resources, ResourceRoles, Tenants, ResourceInstances, GroupRoles)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "workspace", "name": "Workspace",
		"actions": {}}`, http.StatusOK)
	for _, role := range []string{"editor", "viewer"} {
		send(t, m, http.MethodPost, resourcesPath+"/workspace/roles",
			`{"key": "`+role+`", "name": "`+role+`"}`, http.StatusOK)
	}
	send(t, m, http.MethodPost, tenantsPath, `{"key": "acme", "name": "Acme"}`, http.StatusOK)
	send(t, m, http.MethodPost, tenantsPath, `{"key": "beta", "name": "Beta"}`, http.StatusOK)
	send(t, m, http.MethodPost, resourceInstancesPath, `{"key": "ws-1",
		"resource": "workspace", "tenant": "acme"}`, http.StatusOK)
	m.AddGroup("developers", "acme")
	return m
}

// wantObject fails the test unless got is the JSON object want, with no other
// fields.
func wantObject(t *testing.T, got map[string]any, want string) {
	t.Helper()
	var wantValue map[string]any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("want %q is not a JSON object: %v", want, err)
	}
	if !reflect.DeepEqual(got, wantValue) {
		t.Errorf("got %s, want %s", compactJSON(got), compactJSON(wantValue))
	}
}

func TestGroupRoleState(t *testing.T) {
	m := newGroupFixture(t, t)

	assigned := send(t, m, http.MethodPost, developerRolesPath, editorOfWS1, http.StatusOK)
	send(t, m, http.MethodPost, schemaPath+"/groups/"+ObjectID(7)+"/roles",
		`{"role": "viewer", "resource": "workspace", "resource_instance": "ws-1",
		  "tenant": "acme", "ignored": true}`, http.StatusOK)

	wantFields(t, assigned, `{"group_resource_type_key": "group",
		"group_instance_key": "developers", "group_tenant": "acme"}`)
	if len(assigned) != 3 {
		t.Errorf("assigned = %v, want only the three group fields", assigned)
	}
	wantStoredKeys(t, m, "group_roles/developers:editor:workspace:ws-1:acme",
		"group_roles/developers:viewer:workspace:ws-1:acme", "groups/developers",
		"resource_instances/workspace:ws-1", "resource_roles/workspace:editor",
		"resource_roles/workspace:viewer", "resources/workspace", "tenants/acme", "tenants/beta")
	roleOf := func(key string) string {
		return `{"key": "` + key + `",
			"resource": {"id": "` + ObjectID(1) + `", "key": "workspace"},
			"resource_instance": {"id": "` + ObjectID(6) + `", "key": "ws-1"}}`
	}
	wantPage := func(query, wantBody string) {
		t.Helper()
		wantObject(t, send(t, m, http.MethodGet, developerRolesPath+query, "",
			http.StatusOK), wantBody)
	}
	wantPage("", `{"data": [`+roleOf("editor")+`, `+roleOf("viewer")+`],
		"total_count": 2, "page_count": 1}`)
	wantPage("?page=2&per_page=1", `{"data": [`+roleOf("viewer")+`],
		"total_count": 2, "page_count": 2}`)
	wantPage("?page=3&per_page=1", `{"data": [], "total_count": 2, "page_count": 2}`)
	send(t, m, http.MethodGet, developerRolesPath+"?per_page=101", "",
		http.StatusUnprocessableEntity)

	send(t, m, http.MethodDelete, developerRolesPath, editorOfWS1, http.StatusNoContent)

	wantPage("", `{"data": [`+roleOf("viewer")+`], "total_count": 1, "page_count": 1}`)
	send(t, m, http.MethodDelete, developerRolesPath, viewerOfWS1, http.StatusNoContent)
	wantPage("", `{"data": [], "total_count": 0, "page_count": 0}`)
	for method, body := range map[string]string{
		http.MethodGet: "", http.MethodPost: editorOfWS1, http.MethodDelete: editorOfWS1,
	} {
		send(t, m, method, schemaPath+"/groups/testers/roles", body, http.StatusNotFound)
	}
	wantStoredKeys(t, m, "groups/developers", "resource_instances/workspace:ws-1",
		"resource_roles/workspace:editor", "resource_roles/workspace:viewer",
		"resources/workspace", "tenants/acme", "tenants/beta")
}

func TestGroupRoleRequestErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "no tenant",
			body:       `{"role": "viewer", "resource": "workspace", "resource_instance": "ws-1"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "an unknown resource",
			body: `{"role": "viewer", "resource": "folder", "resource_instance": "ws-1",
				"tenant": "acme"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "an unknown role",
			body: `{"role": "owner", "resource": "workspace", "resource_instance": "ws-1",
				"tenant": "acme"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "an unknown tenant",
			body: `{"role": "viewer", "resource": "workspace", "resource_instance": "ws-1",
				"tenant": "other"}`,
			wantStatus: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		for _, method := range []string{http.MethodPost, http.MethodDelete} {
			t.Run(method+" "+tt.name, func(t *testing.T) {
				rec := &errorRecorder{TB: t}
				m := newGroupFixture(t, rec)
				send(t, m, http.MethodPost, developerRolesPath, editorOfWS1, http.StatusOK)
				want := m.StoredKeys()

				send(t, m, method, developerRolesPath, tt.body, tt.wantStatus)

				if got := rec.take(); len(got) != 0 {
					t.Errorf("test errors = %q, want none", got)
				}
				wantStoredKeys(t, m, want...)
			})
		}
	}
}

// TestGroupRoleUnconfirmedBehaviour checks that the fake fails the test, rather
// than guess, on requests whose effect on the API is unconfirmed, and leaves the
// group's roles as they were.
func TestGroupRoleUnconfirmedBehaviour(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		body      string
		wantError string
	}{
		{
			name: "assign a role the group has", method: http.MethodPost, body: editorOfWS1,
			wantError: "the group already has this role",
		},
		{
			name: "remove a role the group does not have", method: http.MethodDelete,
			body: viewerOfWS1, wantError: "the group does not have this role",
		},
		{
			name: "assign on an instance that does not exist", method: http.MethodPost,
			body: `{"role": "viewer", "resource": "workspace", "resource_instance": "ws-2",
				"tenant": "acme"}`,
			wantError: "resource instance workspace:ws-2, which does not exist",
		},
		{
			name: "assign on an instance of another tenant", method: http.MethodPost,
			body: `{"role": "viewer", "resource": "workspace", "resource_instance": "ws-1",
				"tenant": "beta"}`,
			wantError: "resource instance workspace:ws-1 of another tenant",
		},
		{
			name: "assign a role by ID", method: http.MethodPost,
			body: `{"role": "` + ObjectID(3) + `", "resource": "workspace",
				"resource_instance": "ws-1", "tenant": "acme"}`,
			wantError: "names the role " + ObjectID(3) + " by ID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := newGroupFixture(t, rec)
			send(t, m, http.MethodPost, developerRolesPath, editorOfWS1, http.StatusOK)
			want := m.StoredKeys()

			send(t, m, tt.method, developerRolesPath, tt.body, http.StatusNotImplemented)

			wantUnconfirmed(t, rec, tt.wantError)
			wantStoredKeys(t, m, want...)
		})
	}
}
