package mockpermit

import (
	"fmt"
	"net/http"
	"testing"
)

// newFilesAndFolders returns a mock with the file and folder resources, an editor
// role on files, a manager role on folders, and the parent relation that links a
// file to its folder, created in that order.
func newFilesAndFolders(t *testing.T) *Server {
	t.Helper()
	m := New(t, Resources, ResourceRoles, ResourceRelations, ImplicitGrants)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "file", "name": "File",
		"actions": {}}`, http.StatusOK)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "folder", "name": "Folder",
		"actions": {}}`, http.StatusOK)
	send(t, m, http.MethodPost, resourcesPath+"/file/roles", `{"key": "editor", "name": "E"}`,
		http.StatusOK)
	send(t, m, http.MethodPost, resourcesPath+"/folder/roles", `{"key": "manager", "name": "M"}`,
		http.StatusOK)
	send(t, m, http.MethodPost, resourcesPath+"/file/relations",
		`{"key": "parent", "name": "Parent", "subject_resource": "folder"}`, http.StatusOK)
	return m
}

func TestImplicitGrantState(t *testing.T) {
	m := newFilesAndFolders(t)
	editor := resourcesPath + "/file/roles/editor"
	viewer := resourcesPath + "/file/roles/viewer"
	send(t, m, http.MethodPost, resourcesPath+"/file/roles", `{"key": "viewer", "name": "V"}`,
		http.StatusOK)
	const grant = `{"role": "manager", "on_resource": "folder", "linked_by_relation": "parent"}`
	noGrants := func(roleID string) string {
		return `{"granted_to": {"id": "` + roleID + `", "users_with_role": [],
			"when": {"no_direct_roles_on_object": false}}}`
	}

	created := send(t, m, http.MethodPost, editor+"/implicit_grants", grant, http.StatusOK)

	wantRule := `{
		"role_id": "` + ObjectID(4) + `", "resource_id": "` + ObjectID(2) + `",
		"relation_id": "` + ObjectID(5) + `",
		"role": "manager", "on_resource": "folder", "linked_by_relation": "parent",
		"when": {"no_direct_roles_on_object": false}
	}`
	wantFields(t, created, wantRule)
	grantedTo := send(t, m, http.MethodGet, editor, "", http.StatusOK)["granted_to"]
	granted, _ := grantedTo.(map[string]any)
	rules, _ := granted["users_with_role"].([]any)
	if len(rules) != 1 {
		t.Fatalf("granted_to = %v, want one rule", grantedTo)
	}
	rule, _ := rules[0].(map[string]any)
	wantFields(t, rule, wantRule)
	wantFields(t, send(t, m, http.MethodGet, viewer, "", http.StatusOK), noGrants(ObjectID(6)))
	wantFields(t, send(t, m, http.MethodGet, resourcesPath+"/folder/roles/manager", "",
		http.StatusOK), noGrants(ObjectID(4)))
	send(t, m, http.MethodPost, editor+"/implicit_grants", grant, http.StatusConflict)
	wantStoredKeys(t, m, "implicit_grants/file:editor:folder:manager:parent",
		"relations/file:parent", "resource_roles/file:editor", "resource_roles/file:viewer",
		"resource_roles/folder:manager", "resources/file", "resources/folder")

	send(t, m, http.MethodDelete, editor+"/implicit_grants", grant, http.StatusNoContent)

	wantFields(t, send(t, m, http.MethodGet, editor, "", http.StatusOK), noGrants(ObjectID(3)))
	send(t, m, http.MethodDelete, editor+"/implicit_grants", grant, http.StatusNotFound)
	wantStoredKeys(t, m, "relations/file:parent", "resource_roles/file:editor",
		"resource_roles/file:viewer", "resource_roles/folder:manager", "resources/file",
		"resources/folder")
}

func TestImplicitGrantErrors(t *testing.T) {
	tests := []struct {
		name                           string
		toRole, role, onResource, link string
		wantStatus                     int
	}{
		{
			name: "no role", toRole: "file/roles/editor", onResource: "folder", link: "parent",
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "unknown resource", toRole: "disk/roles/editor",
			role: "manager", onResource: "folder", link: "parent",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "unknown role to grant", toRole: "file/roles/owner",
			role: "manager", onResource: "folder", link: "parent",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "unknown on_resource", toRole: "file/roles/editor",
			role: "manager", onResource: "disk", link: "parent",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "role not on on_resource", toRole: "file/roles/editor",
			role: "editor", onResource: "folder", link: "parent",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "unknown relation", toRole: "file/roles/editor",
			role: "manager", onResource: "folder", link: "owner",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "relation from another resource", toRole: "file/roles/editor",
			role: "editor", onResource: "file", link: "parent",
			wantStatus: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newFilesAndFolders(t)
			body := fmt.Sprintf(`{"role": %q, "on_resource": %q, "linked_by_relation": %q}`,
				tt.role, tt.onResource, tt.link)

			send(t, m, http.MethodPost, resourcesPath+"/"+tt.toRole+"/implicit_grants", body,
				tt.wantStatus)

			wantStoredKeys(t, m, "relations/file:parent", "resource_roles/file:editor",
				"resource_roles/folder:manager", "resources/file", "resources/folder")
		})
	}
}
