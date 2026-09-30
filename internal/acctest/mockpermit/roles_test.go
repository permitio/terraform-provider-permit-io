package mockpermit

import (
	"net/http"
	"testing"
)

const rolesPath = schemaPath + "/roles"

func TestRoleState(t *testing.T) {
	m := New(t, Roles)

	created := send(t, m, http.MethodPost, rolesPath,
		`{"key": "editor", "name": "Editor", "description": "d", "permissions": ["doc:read"]}`,
		http.StatusOK)

	wantRole := `{
		"id": "` + ObjectID(1) + `", "key": "editor", "name": "Editor", "description": "d",
		"permissions": ["doc:read"], "extends": [],
		"organization_id": "` + OrganizationID + `", "project_id": "` + ProjectID + `",
		"environment_id": "` + EnvironmentID + `",
		"created_at": "` + timestamp + `", "updated_at": "` + timestamp + `"
	}`
	wantFields(t, created, wantRole)
	if _, ok := created["granted_to"]; ok {
		t.Errorf("a top-level role has granted_to: %v", created)
	}
	wantFields(t, send(t, m, http.MethodGet, rolesPath+"/editor", "", http.StatusOK), wantRole)
	wantFields(t, send(t, m, http.MethodGet, rolesPath+"/"+ObjectID(1), "", http.StatusOK),
		wantRole)
	send(t, m, http.MethodPost, rolesPath, `{"key": "editor", "name": "Again"}`,
		http.StatusConflict)
	send(t, m, http.MethodPost, rolesPath, `{"name": "No key"}`, http.StatusUnprocessableEntity)
	send(t, m, http.MethodGet, rolesPath+"/missing", "", http.StatusNotFound)

	updated := send(t, m, http.MethodPatch, rolesPath+"/editor",
		`{"name": "Chief editor", "extends": ["viewer"]}`, http.StatusOK)
	wantFields(t, updated, `{"name": "Chief editor", "description": "d", "extends": ["viewer"],
		"permissions": ["doc:read"]}`)

	assigned := send(t, m, http.MethodPost, rolesPath+"/editor/permissions",
		`{"permissions": ["doc:write", "doc:read", "doc:delete"]}`, http.StatusOK)
	wantFields(t, assigned, `{"permissions": ["doc:read", "doc:write", "doc:delete"]}`)
	removed := send(t, m, http.MethodDelete, rolesPath+"/"+ObjectID(1)+"/permissions",
		`{"permissions": ["doc:read", "doc:share"]}`, http.StatusOK)
	wantFields(t, removed, `{"permissions": ["doc:write", "doc:delete"]}`)
	wantFields(t, send(t, m, http.MethodGet, rolesPath+"/editor", "", http.StatusOK),
		`{"name": "Chief editor", "permissions": ["doc:write", "doc:delete"]}`)
	send(t, m, http.MethodPost, rolesPath+"/editor/permissions", `{"permissions": "doc:read"}`,
		http.StatusUnprocessableEntity)
	send(t, m, http.MethodPost, rolesPath+"/missing/permissions", `{"permissions": []}`,
		http.StatusNotFound)
	wantStoredKeys(t, m, "roles/editor")

	send(t, m, http.MethodDelete, rolesPath+"/editor", "", http.StatusNoContent)

	wantStoredKeys(t, m)
	send(t, m, http.MethodGet, rolesPath+"/editor", "", http.StatusNotFound)
	send(t, m, http.MethodDelete, rolesPath+"/editor", "", http.StatusNotFound)
}

func TestResourceRoleState(t *testing.T) {
	m := New(t, Resources, Roles, ResourceRoles)
	documentRoles := resourcesPath + "/document/roles"
	send(t, m, http.MethodPost, documentRoles, `{"key": "editor", "name": "Editor"}`,
		http.StatusNotFound)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "document", "name": "Document",
		"actions": {"read": {"name": "Read"}}}`, http.StatusOK)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "folder", "name": "Folder",
		"actions": {}}`, http.StatusOK)

	created := send(t, m, http.MethodPost, documentRoles,
		`{"key": "editor", "name": "Editor", "permissions": ["read"], "extends": []}`,
		http.StatusOK)

	wantRole := `{
		"id": "` + ObjectID(4) + `", "key": "editor", "name": "Editor",
		"permissions": ["read"], "extends": [],
		"resource": "document", "resource_id": "` + ObjectID(1) + `",
		"granted_to": {"id": "` + ObjectID(4) + `", "users_with_role": [],
			"when": {"no_direct_roles_on_object": false}},
		"organization_id": "` + OrganizationID + `", "project_id": "` + ProjectID + `",
		"environment_id": "` + EnvironmentID + `",
		"created_at": "` + timestamp + `", "updated_at": "` + timestamp + `"
	}`
	wantFields(t, created, wantRole)
	wantFields(t, send(t, m, http.MethodGet, documentRoles+"/editor", "", http.StatusOK), wantRole)
	byID := resourcesPath + "/" + ObjectID(1) + "/roles/" + ObjectID(4)
	wantFields(t, send(t, m, http.MethodGet, byID, "", http.StatusOK), wantRole)
	send(t, m, http.MethodGet, resourcesPath+"/folder/roles/editor", "", http.StatusNotFound)
	send(t, m, http.MethodGet, resourcesPath+"/folder/roles/"+ObjectID(4), "", http.StatusNotFound)
	send(t, m, http.MethodGet, rolesPath+"/editor", "", http.StatusNotFound)
	send(t, m, http.MethodPost, documentRoles, `{"key": "editor", "name": "Again"}`,
		http.StatusConflict)
	send(t, m, http.MethodPost, resourcesPath+"/folder/roles", `{"key": "editor", "name": "E"}`,
		http.StatusOK)
	wantStoredKeys(t, m, "resource_roles/document:editor", "resource_roles/folder:editor",
		"resources/document", "resources/folder")

	send(t, m, http.MethodPatch, documentRoles+"/editor", `{"description": "d"}`, http.StatusOK)
	send(t, m, http.MethodPost, documentRoles+"/editor/permissions", `{"permissions": ["write"]}`,
		http.StatusOK)
	send(t, m, http.MethodDelete, documentRoles+"/editor/permissions", `{"permissions": ["read"]}`,
		http.StatusOK)

	wantFields(t, send(t, m, http.MethodGet, documentRoles+"/editor", "", http.StatusOK),
		`{"description": "d", "permissions": ["write"]}`)
	wantFields(t, send(t, m, http.MethodGet, resourcesPath+"/folder/roles/editor", "",
		http.StatusOK), `{"permissions": [], "resource": "folder"}`)

	send(t, m, http.MethodDelete, documentRoles+"/editor", "", http.StatusNoContent)

	wantStoredKeys(t, m, "resource_roles/folder:editor", "resources/document", "resources/folder")
	send(t, m, http.MethodGet, documentRoles+"/editor", "", http.StatusNotFound)
}
