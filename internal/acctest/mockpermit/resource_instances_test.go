package mockpermit

import (
	"net/http"
	"reflect"
	"testing"
)

const resourceInstancesPath = "/v2/facts/" + ProjectID + "/" + EnvironmentID +
	"/resource_instances"

// newDocumentsOfAcme returns a mock with the document resource (ID 1) and the acme
// tenant (ID 2).
func newDocumentsOfAcme(t *testing.T, tb testing.TB) *Server {
	t.Helper()
	m := New(tb, Resources, Tenants, ResourceInstances)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "document", "name": "Document",
		"actions": {}}`, http.StatusOK)
	send(t, m, http.MethodPost, tenantsPath, `{"key": "acme", "name": "Acme"}`, http.StatusOK)
	return m
}

func TestResourceInstanceState(t *testing.T) {
	m := newDocumentsOfAcme(t, t)

	created := send(t, m, http.MethodPost, resourceInstancesPath, `{"key": "handbook",
		"resource": "document", "tenant": "acme", "attributes": {"pages": 12}}`, http.StatusOK)

	wantInstance := `{
		"id": "` + ObjectID(3) + `", "key": "handbook", "resource": "document",
		"resource_id": "` + ObjectID(1) + `", "tenant": "acme", "tenant_id": "` + ObjectID(2) + `",
		"attributes": {"pages": 12},
		"organization_id": "` + OrganizationID + `", "project_id": "` + ProjectID + `",
		"environment_id": "` + EnvironmentID + `",
		"created_at": "` + timestamp + `", "updated_at": "` + timestamp + `"
	}`
	wantFields(t, created, wantInstance)
	wantFields(t, send(t, m, http.MethodGet, resourceInstancesPath+"/document:handbook", "",
		http.StatusOK), wantInstance)
	wantFields(t, send(t, m, http.MethodGet, resourceInstancesPath+"/"+ObjectID(3), "",
		http.StatusOK), wantInstance)
	send(t, m, http.MethodGet, resourceInstancesPath+"/document:missing", "", http.StatusNotFound)
	send(t, m, http.MethodPost, resourceInstancesPath, `{"key": "handbook",
		"resource": "document", "tenant": "acme"}`, http.StatusConflict)
	wantFields(t, send(t, m, http.MethodPost, resourceInstancesPath, `{"key": "memo",
		"resource": "document", "tenant": "acme"}`, http.StatusOK),
		`{"id": "`+ObjectID(4)+`", "attributes": {}}`)
	wantStoredKeys(t, m, "resource_instances/document:handbook", "resource_instances/document:memo",
		"resources/document", "tenants/acme")

	updated := send(t, m, http.MethodPatch, resourceInstancesPath+"/document:handbook",
		`{"attributes": {"pages": 14, "draft": true}}`, http.StatusOK)

	wantUpdated := `{"id": "` + ObjectID(3) + `", "key": "handbook",
		"attributes": {"pages": 14, "draft": true}}`
	wantFields(t, updated, wantUpdated)
	wantFields(t, send(t, m, http.MethodGet, resourceInstancesPath+"/document:handbook", "",
		http.StatusOK), wantUpdated)
	wantFields(t, send(t, m, http.MethodPatch, resourceInstancesPath+"/"+ObjectID(3), `{}`,
		http.StatusOK), wantUpdated)
	send(t, m, http.MethodPatch, resourceInstancesPath+"/document:missing", `{}`,
		http.StatusNotFound)

	send(t, m, http.MethodDelete, resourceInstancesPath+"/document:handbook", "",
		http.StatusNoContent)

	wantStoredKeys(t, m, "resource_instances/document:memo", "resources/document", "tenants/acme")
	send(t, m, http.MethodGet, resourceInstancesPath+"/document:handbook", "", http.StatusNotFound)
	send(t, m, http.MethodDelete, resourceInstancesPath+"/document:handbook", "",
		http.StatusNotFound)
	send(t, m, http.MethodDelete, resourceInstancesPath+"/"+ObjectID(4), "", http.StatusNoContent)
	wantStoredKeys(t, m, "resources/document", "tenants/acme")
}

func TestResourceInstanceRequestErrors(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
	}{
		{
			name: "create without tenant", method: http.MethodPost,
			body:       `{"key": "memo", "resource": "document"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "create without key", method: http.MethodPost,
			body:       `{"resource": "document", "tenant": "acme"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "create with an unknown field", method: http.MethodPost,
			body:       `{"key": "memo", "resource": "document", "tenant": "acme", "name": "Memo"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "create with attributes not an object", method: http.MethodPost,
			body: `{"key": "memo", "resource": "document", "tenant": "acme",
				"attributes": "{}"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "create on an unknown resource", method: http.MethodPost,
			body:       `{"key": "memo", "resource": "folder", "tenant": "acme"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "create in an unknown tenant", method: http.MethodPost,
			body:       `{"key": "memo", "resource": "document", "tenant": "other"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "update with an unknown field", method: http.MethodPatch,
			body: `{"tenant": "other"}`, wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "update with attributes not an object", method: http.MethodPatch,
			body: `{"attributes": []}`, wantStatus: http.StatusUnprocessableEntity,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newDocumentsOfAcme(t, t)
			before := send(t, m, http.MethodPost, resourceInstancesPath, `{"key": "handbook",
				"resource": "document", "tenant": "acme", "attributes": {"pages": 12}}`,
				http.StatusOK)
			path := resourceInstancesPath
			if tt.method == http.MethodPatch {
				path += "/document:handbook"
			}

			send(t, m, tt.method, path, tt.body, tt.wantStatus)

			wantStoredKeys(t, m, "resource_instances/document:handbook", "resources/document",
				"tenants/acme")
			after := send(t, m, http.MethodGet, resourceInstancesPath+"/document:handbook", "",
				http.StatusOK)
			if !reflect.DeepEqual(after, before) {
				t.Errorf("instance after the rejected request = %v, want it unchanged: %v",
					after, before)
			}
		})
	}
}

// TestResourceInstanceUnconfirmedBehaviour checks that the fake fails the test,
// rather than guess, on a create that names the resource or the tenant by ID,
// where the API documents a key.
func TestResourceInstanceUnconfirmedBehaviour(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError string
	}{
		{
			name:      "resource by ID",
			body:      `{"key": "memo", "resource": "` + ObjectID(1) + `", "tenant": "acme"}`,
			wantError: "names the resource " + ObjectID(1) + " by ID",
		},
		{
			name:      "tenant by ID",
			body:      `{"key": "memo", "resource": "document", "tenant": "` + ObjectID(2) + `"}`,
			wantError: "names the tenant " + ObjectID(2) + " by ID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := newDocumentsOfAcme(t, rec)

			send(t, m, http.MethodPost, resourceInstancesPath, tt.body, http.StatusNotImplemented)

			wantUnconfirmed(t, rec, tt.wantError)
			wantStoredKeys(t, m, "resources/document", "tenants/acme")
		})
	}
}

// TestResourceInstanceOfADeletedParent checks that the fake fails the test, rather
// than guess, on a request for an instance whose resource or tenant was deleted,
// and keeps the instance.
func TestResourceInstanceOfADeletedParent(t *testing.T) {
	parents := map[string]struct {
		path      string
		wantAfter string
	}{
		"resource": {path: resourcesPath + "/document", wantAfter: "tenants/acme"},
		"tenant":   {path: tenantsPath + "/acme", wantAfter: "resources/document"},
	}
	bodies := map[string]string{
		http.MethodGet: "", http.MethodPatch: `{"attributes": {}}`, http.MethodDelete: "",
	}
	for parentName, parent := range parents {
		for method, body := range bodies {
			t.Run(parentName+" "+method, func(t *testing.T) {
				rec := &errorRecorder{TB: t}
				m := newDocumentsOfAcme(t, rec)
				send(t, m, http.MethodPost, resourceInstancesPath, `{"key": "handbook",
					"resource": "document", "tenant": "acme"}`, http.StatusOK)
				send(t, m, http.MethodDelete, parent.path, "", http.StatusNoContent)

				send(t, m, method, resourceInstancesPath+"/document:handbook", body,
					http.StatusNotImplemented)

				wantUnconfirmed(t, rec, "names a resource instance whose resource or tenant "+
					"was deleted")
				wantStoredKeys(t, m, "resource_instances/document:handbook", parent.wantAfter)
			})
		}
	}
}
