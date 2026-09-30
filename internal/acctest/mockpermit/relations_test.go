package mockpermit

import (
	"net/http"
	"testing"
)

func TestRelationState(t *testing.T) {
	m := New(t, Resources, ResourceRelations)
	fileRelations := resourcesPath + "/file/relations"
	send(t, m, http.MethodPost, resourcesPath, `{"key": "file", "name": "File",
		"actions": {}}`, http.StatusOK)
	send(t, m, http.MethodPost, fileRelations,
		`{"key": "parent", "name": "Parent", "subject_resource": "folder"}`, http.StatusNotFound)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "folder", "name": "Folder",
		"actions": {}}`, http.StatusOK)

	created := send(t, m, http.MethodPost, fileRelations,
		`{"key": "parent", "name": "Parent", "description": "d", "subject_resource": "`+
			ObjectID(2)+`"}`, http.StatusOK)

	wantRelation := `{
		"id": "` + ObjectID(3) + `", "key": "parent", "name": "Parent", "description": "d",
		"subject_resource": "folder", "subject_resource_id": "` + ObjectID(2) + `",
		"object_resource": "file", "object_resource_id": "` + ObjectID(1) + `",
		"organization_id": "` + OrganizationID + `", "project_id": "` + ProjectID + `",
		"environment_id": "` + EnvironmentID + `",
		"created_at": "` + timestamp + `", "updated_at": "` + timestamp + `"
	}`
	wantFields(t, created, wantRelation)
	wantFields(t, send(t, m, http.MethodGet, fileRelations+"/parent", "", http.StatusOK),
		wantRelation)
	byID := resourcesPath + "/" + ObjectID(1) + "/relations/" + ObjectID(3)
	wantFields(t, send(t, m, http.MethodGet, byID, "", http.StatusOK), wantRelation)
	send(t, m, http.MethodGet, resourcesPath+"/folder/relations/parent", "", http.StatusNotFound)
	send(t, m, http.MethodGet, resourcesPath+"/folder/relations/"+ObjectID(3), "",
		http.StatusNotFound)
	send(t, m, http.MethodPost, fileRelations,
		`{"key": "parent", "name": "Again", "subject_resource": "folder"}`, http.StatusConflict)
	send(t, m, http.MethodPost, fileRelations, `{"key": "child", "name": "Child"}`,
		http.StatusUnprocessableEntity)
	wantStoredKeys(t, m, "relations/file:parent", "resources/file", "resources/folder")

	send(t, m, http.MethodDelete, fileRelations+"/parent", "", http.StatusNoContent)

	wantStoredKeys(t, m, "resources/file", "resources/folder")
	send(t, m, http.MethodGet, fileRelations+"/parent", "", http.StatusNotFound)
	send(t, m, http.MethodDelete, fileRelations+"/parent", "", http.StatusNotFound)
}
