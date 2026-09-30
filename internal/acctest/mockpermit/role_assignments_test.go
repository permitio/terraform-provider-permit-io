package mockpermit

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const (
	factsPath           = "/v2/facts/" + ProjectID + "/" + EnvironmentID
	aliceRolesPath      = factsPath + "/users/alice/roles"
	roleAssignmentsPath = factsPath + "/role_assignments"
	editorInAcme        = `{"role": "editor", "tenant": "acme"}`
	readerOfHandbook    = `{"role": "reader", "tenant": "acme",
		"resource_instance": "document:handbook"}`
)

// newAssignmentFixture returns a mock with the document resource (ID 1), the acme
// and beta tenants (2 and 3), the top-level editor role (4), the reader role of
// documents (5), the handbook document of acme (6) and the user alice (7).
func newAssignmentFixture(t *testing.T, tb testing.TB) *Server {
	t.Helper()
	m := New(tb, Resources, ResourceRoles, Roles, Tenants, ResourceInstances, RoleAssignments)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "document", "name": "Document",
		"actions": {}}`, http.StatusOK)
	send(t, m, http.MethodPost, tenantsPath, `{"key": "acme", "name": "Acme"}`, http.StatusOK)
	send(t, m, http.MethodPost, tenantsPath, `{"key": "beta", "name": "Beta"}`, http.StatusOK)
	send(t, m, http.MethodPost, rolesPath, `{"key": "editor", "name": "Editor"}`, http.StatusOK)
	send(t, m, http.MethodPost, resourcesPath+"/document/roles",
		`{"key": "reader", "name": "Reader"}`, http.StatusOK)
	send(t, m, http.MethodPost, resourceInstancesPath, `{"key": "handbook",
		"resource": "document", "tenant": "acme"}`, http.StatusOK)
	m.AddUser(`{"key": "alice"}`)
	return m
}

// sendForList makes a list request to the mock, fails the test unless the status
// is wantStatus, and returns the decoded JSON list, or nil for another status.
func sendForList(t *testing.T, m *Server, path string, wantStatus int) []any {
	t.Helper()
	status, got := call(t, m, http.MethodGet, path, bearer, "")
	if status != wantStatus {
		t.Fatalf("GET %s: status %d, want %d; body %s", path, status, wantStatus, got)
	}
	if status != http.StatusOK {
		return nil
	}
	var decoded []any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil || decoded == nil {
		t.Fatalf("GET %s: body %q is not a JSON list: %v", path, got, err)
	}
	return decoded
}

// wantIDs fails the test unless list holds objects with exactly these IDs, in order.
func wantIDs(t *testing.T, list []any, want ...string) {
	t.Helper()
	got := []string{}
	for _, item := range list {
		object, _ := item.(map[string]any)
		got = append(got, str(object, "id"))
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("IDs = %q, want %q", got, want)
	}
}

func TestRoleAssignmentState(t *testing.T) {
	m := newAssignmentFixture(t, t)

	inTenant := send(t, m, http.MethodPost, aliceRolesPath, editorInAcme, http.StatusOK)
	onInstance := send(t, m, http.MethodPost, factsPath+"/users/"+ObjectID(7)+"/roles",
		readerOfHandbook, http.StatusOK)

	wantInTenant := `{
		"id": "` + ObjectID(8) + `", "user": "alice", "user_id": "` + ObjectID(7) + `",
		"role": "editor", "role_id": "` + ObjectID(4) + `",
		"tenant": "acme", "tenant_id": "` + ObjectID(2) + `",
		"organization_id": "` + OrganizationID + `", "project_id": "` + ProjectID + `",
		"environment_id": "` + EnvironmentID + `", "created_at": "` + timestamp + `"
	}`
	wantFields(t, inTenant, wantInTenant)
	for _, field := range []string{"resource_instance", "resource_instance_id", "updated_at"} {
		if _, ok := inTenant[field]; ok {
			t.Errorf("a tenant-level assignment has %s: %v", field, inTenant)
		}
	}
	wantFields(t, onInstance, `{
		"id": "`+ObjectID(9)+`", "user": "alice", "role": "reader", "role_id": "`+ObjectID(5)+`",
		"tenant": "acme", "resource_instance": "document:handbook",
		"resource_instance_id": "`+ObjectID(6)+`"
	}`)
	wantStoredKeys(t, m, "resource_instances/document:handbook", "resource_roles/document:reader",
		"resources/document", "role_assignments/alice:editor:acme:",
		"role_assignments/alice:reader:acme:document:handbook", "roles/editor", "tenants/acme",
		"tenants/beta", "users/alice")

	all := sendForList(t, m, roleAssignmentsPath+"?user=alice", http.StatusOK)
	wantIDs(t, all, ObjectID(8), ObjectID(9))
	if !reflect.DeepEqual(all[0], inTenant) {
		t.Errorf("listed %v, want the assignment as created: %v", all[0], inTenant)
	}
	wantIDs(t, sendForList(t, m, roleAssignmentsPath+"?role=editor&tenant=acme&user=alice"+
		"&page=1&per_page=1", http.StatusOK), ObjectID(8))
	wantIDs(t, sendForList(t, m, roleAssignmentsPath+"?tenant=acme&page=2&per_page=1",
		http.StatusOK), ObjectID(9))
	wantIDs(t, sendForList(t, m, roleAssignmentsPath+"?page=3&per_page=1", http.StatusOK))
	wantIDs(t, sendForList(t, m, roleAssignmentsPath+"?tenant=beta", http.StatusOK))
	wantIDs(t, sendForList(t, m, roleAssignmentsPath+"?user=bob", http.StatusOK))

	send(t, m, http.MethodDelete, aliceRolesPath, editorInAcme, http.StatusNoContent)

	wantIDs(t, sendForList(t, m, roleAssignmentsPath, http.StatusOK), ObjectID(9))
	send(t, m, http.MethodDelete, aliceRolesPath, editorInAcme, http.StatusNotFound)
	send(t, m, http.MethodDelete, aliceRolesPath, readerOfHandbook, http.StatusNoContent)
	wantIDs(t, sendForList(t, m, roleAssignmentsPath, http.StatusOK))
	wantStoredKeys(t, m, "resource_instances/document:handbook", "resource_roles/document:reader",
		"resources/document", "roles/editor", "tenants/acme", "tenants/beta", "users/alice")
}

// TestAddInstanceRoleAssignments seeds assignments of the reader role on new
// documents to alice and checks that the list pages through them, with the
// assignment made after them on the last page.
func TestAddInstanceRoleAssignments(t *testing.T) {
	m := newAssignmentFixture(t, t)

	added := m.AddInstanceRoleAssignments("alice", "reader", "document", "acme", 3)
	send(t, m, http.MethodPost, aliceRolesPath, readerOfHandbook, http.StatusOK)

	wantAdded := []string{
		"resource_instances/document:seed-1", "resource_instances/document:seed-2",
		"resource_instances/document:seed-3",
		"role_assignments/alice:reader:acme:document:seed-1",
		"role_assignments/alice:reader:acme:document:seed-2",
		"role_assignments/alice:reader:acme:document:seed-3",
	}
	if !slices.Equal(added, wantAdded) {
		t.Errorf("AddInstanceRoleAssignments() = %q, want %q", added, wantAdded)
	}
	if requests := m.Requests(http.MethodPost, aliceRolesPath); len(requests) != 1 {
		t.Errorf("POST %s: %d requests recorded, want only the test's own", aliceRolesPath,
			len(requests))
	}
	// Seeding makes the instances 8, 10 and 12, and the assignments 9, 11 and 13.
	query := roleAssignmentsPath + "?user=alice&role=reader&tenant=acme&per_page=2"
	wantIDs(t, sendForList(t, m, query+"&page=1", http.StatusOK), ObjectID(9), ObjectID(11))
	onLastPage := sendForList(t, m, query+"&page=2", http.StatusOK)
	wantIDs(t, onLastPage, ObjectID(13), ObjectID(14))
	if object, _ := onLastPage[1].(map[string]any); str(object, "resource_instance") !=
		"document:handbook" {
		t.Errorf("last listed assignment = %v, want the one on document:handbook", object)
	}
	wantIDs(t, sendForList(t, m, query+"&page=3", http.StatusOK))
}

func TestRoleAssignmentRequestErrors(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name: "assign without a role", method: http.MethodPost, path: aliceRolesPath,
			body: `{"tenant": "acme"}`, wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "assign with an unknown field", method: http.MethodPost, path: aliceRolesPath,
			body:       `{"role": "editor", "tenant": "acme", "resource": "document"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "assign to an unknown user", method: http.MethodPost,
			path: factsPath + "/users/bob/roles", body: editorInAcme,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "assign in an unknown tenant", method: http.MethodPost, path: aliceRolesPath,
			body: `{"role": "editor", "tenant": "other"}`, wantStatus: http.StatusNotFound,
		},
		{
			name: "assign an unknown role", method: http.MethodPost, path: aliceRolesPath,
			body: `{"role": "owner", "tenant": "acme"}`, wantStatus: http.StatusNotFound,
		},
		{
			name: "assign a top-level role on an instance", method: http.MethodPost,
			path: aliceRolesPath,
			body: `{"role": "editor", "tenant": "acme",
				"resource_instance": "document:handbook"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "unassign without a tenant", method: http.MethodDelete, path: aliceRolesPath,
			body: `{"role": "editor"}`, wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "unassign a role the user does not have", method: http.MethodDelete,
			path: aliceRolesPath, body: `{"role": "editor", "tenant": "beta"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "list a page past the limit", method: http.MethodGet,
			path:       roleAssignmentsPath + "?per_page=1001",
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "list page zero", method: http.MethodGet, path: roleAssignmentsPath + "?page=0",
			wantStatus: http.StatusUnprocessableEntity,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := newAssignmentFixture(t, rec)
			send(t, m, http.MethodPost, aliceRolesPath, editorInAcme, http.StatusOK)

			status, body := call(t, m, tt.method, tt.path, bearer, tt.body)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d; body %s", status, tt.wantStatus, body)
			}
			if got := rec.take(); len(got) != 0 {
				t.Errorf("test errors = %q, want none", got)
			}
			wantIDs(t, sendForList(t, m, roleAssignmentsPath, http.StatusOK), ObjectID(8))
		})
	}
}

// TestRoleAssignmentUnconfirmedBehaviour checks that the fake fails the test,
// rather than guess, on requests whose effect on the API is unconfirmed, and leaves
// the assignments as they were.
func TestRoleAssignmentUnconfirmedBehaviour(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		body      string
		before    func(t *testing.T, m *Server)
		wantError string
	}{
		{
			name: "assign without a tenant", method: http.MethodPost, path: aliceRolesPath,
			body: `{"role": "editor"}`, wantError: "names no tenant",
		},
		{
			name: "assign a role the user has", method: http.MethodPost, path: aliceRolesPath,
			body: editorInAcme, wantError: "already has this role assignment",
		},
		{
			name: "assign on an instance that does not exist", method: http.MethodPost,
			path:      aliceRolesPath,
			body:      `{"role": "reader", "tenant": "acme", "resource_instance": "document:memo"}`,
			wantError: "resource instance document:memo, which does not exist",
		},
		{
			name: "assign on an instance of another tenant", method: http.MethodPost,
			path: aliceRolesPath,
			body: `{"role": "reader", "tenant": "beta",
				"resource_instance": "document:handbook"}`,
			wantError: "resource instance document:handbook of another tenant",
		},
		{
			name: "assign a role by ID", method: http.MethodPost, path: aliceRolesPath,
			body:      `{"role": "` + ObjectID(4) + `", "tenant": "acme"}`,
			wantError: "names the role " + ObjectID(4) + " by ID",
		},
		{
			name: "assign a resource role by ID", method: http.MethodPost, path: aliceRolesPath,
			body: `{"role": "` + ObjectID(5) + `", "tenant": "acme",
				"resource_instance": "document:handbook"}`,
			wantError: "names the role " + ObjectID(5) + " by ID",
		},
		{
			name: "assign in a tenant by ID", method: http.MethodPost, path: aliceRolesPath,
			body:      `{"role": "editor", "tenant": "` + ObjectID(2) + `"}`,
			wantError: "names the tenant " + ObjectID(2) + " by ID",
		},
		{
			name: "list by a user ID", method: http.MethodGet,
			path: roleAssignmentsPath + "?user=" + ObjectID(7), wantError: "names the user by ID",
		},
		{
			name: "list an assignment whose role was deleted", method: http.MethodGet,
			path: roleAssignmentsPath + "?user=alice",
			before: func(t *testing.T, m *Server) {
				send(t, m, http.MethodDelete, rolesPath+"/editor", "", http.StatusNoContent)
			},
			wantError: "has a user, role, tenant or resource instance that was deleted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := newAssignmentFixture(t, rec)
			send(t, m, http.MethodPost, aliceRolesPath, editorInAcme, http.StatusOK)
			if tt.before != nil {
				tt.before(t, m)
			}
			want := m.StoredKeys()

			status, body := call(t, m, tt.method, tt.path, bearer, tt.body)

			if status != http.StatusNotImplemented {
				t.Errorf("status = %d, want 501; body %s", status, body)
			}
			wantUnconfirmed(t, rec, tt.wantError)
			wantStoredKeys(t, m, want...)
		})
	}
}

// TestListUnmodelledParameter checks that the fake fails the test on a list query
// parameter it does not model, whether the spec documents it or not, and on a
// filter given more than once.
func TestListUnmodelledParameter(t *testing.T) {
	tests := map[string]string{
		"resource_instance;":            roleAssignmentsPath + "?resource_instance=document:h",
		"user given more than once;":    roleAssignmentsPath + "?user=alice&user=bob",
		"search;":                       tenantsPath + "?search=acme",
		"page given more than once;":    tenantsPath + "?page=1&page=2",
		"per_page given more than once": developerRolesPath + "?per_page=1&per_page=2",
	}
	for parameter, path := range tests {
		t.Run(path, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := New(rec, TenantList, RoleAssignments, GroupRoles)

			sendForList(t, m, path, http.StatusNotImplemented)

			got := rec.take()
			want := "the fake does not model the query parameter " + parameter
			if len(got) != 1 || !strings.Contains(got[0], want) ||
				strings.Contains(got[0], "unconfirmed") {
				t.Errorf("test errors = %q, want one containing %q", got, want)
			}
		})
	}
}
