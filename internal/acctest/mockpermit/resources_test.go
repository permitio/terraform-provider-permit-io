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
	schemaPath         = "/v2/schema/" + ProjectID + "/" + EnvironmentID
	resourcesPath      = schemaPath + "/resources"
	userAttributesPath = resourcesPath + "/__user/attributes"
)

// send makes a request to the mock, fails the test unless the status is
// wantStatus, and returns the decoded JSON response body, or nil when there is
// none.
func send(t *testing.T, m *Server, method, path, body string, wantStatus int) map[string]any {
	t.Helper()
	status, got := call(t, m, method, path, bearer, body)
	if status != wantStatus {
		t.Fatalf("%s %s: status %d, want %d; body %s", method, path, status, wantStatus, got)
	}
	if status == http.StatusNoContent {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("%s %s: body %q is not a JSON object: %v", method, path, got, err)
	}
	return decoded
}

// wantFields fails the test unless each top-level field of the JSON object want
// has the same value in got. Fields want leaves out are not checked.
func wantFields(t *testing.T, got map[string]any, want string) {
	t.Helper()
	var wantObject map[string]any
	if err := json.Unmarshal([]byte(want), &wantObject); err != nil {
		t.Fatalf("want %q is not a JSON object: %v", want, err)
	}
	for field, wantValue := range wantObject {
		if gotValue, ok := got[field]; !ok || !reflect.DeepEqual(gotValue, wantValue) {
			gotJSON, _ := json.Marshal(gotValue)
			wantJSON, _ := json.Marshal(wantValue)
			t.Errorf("%s = %s, want %s", field, gotJSON, wantJSON)
		}
	}
}

func wantStoredKeys(t *testing.T, m *Server, want ...string) {
	t.Helper()
	if got := m.StoredKeys(); !slices.Equal(got, want) {
		t.Errorf("StoredKeys() = %q, want %q", got, want)
	}
}

func TestResourceState(t *testing.T) {
	m := New(t, Resources)

	created := send(t, m, http.MethodPost, resourcesPath, `{
		"key": "document", "name": "Document", "urn": "prn:document",
		"actions": {
			"write": {"name": "Write", "description": "Change it"},
			"read": {"name": "Read"}
		},
		"attributes": {"pages": {"type": "number"}, "owner": {"type": "string", "description": "o"}}
	}`, http.StatusOK)

	wantResource := `{
		"id": "` + ObjectID(1) + `", "key": "document", "name": "Document", "urn": "prn:document",
		"organization_id": "` + OrganizationID + `", "project_id": "` + ProjectID + `",
		"environment_id": "` + EnvironmentID + `",
		"created_at": "` + timestamp + `", "updated_at": "` + timestamp + `",
		"actions": {
			"read": {"id": "` + ObjectID(2) + `", "key": "read", "name": "Read"},
			"write": {"id": "` + ObjectID(3) + `", "key": "write", "name": "Write",
				"description": "Change it"}
		},
		"attributes": {
			"owner": {"id": "` + ObjectID(4) + `", "key": "owner", "type": "string",
				"description": "o"},
			"pages": {"id": "` + ObjectID(5) + `", "key": "pages", "type": "number"}
		}
	}`
	wantFields(t, created, wantResource)
	wantFields(t, send(t, m, http.MethodGet, resourcesPath+"/document", "", http.StatusOK),
		wantResource)
	wantFields(t, send(t, m, http.MethodGet, resourcesPath+"/"+ObjectID(1), "", http.StatusOK),
		wantResource)
	wantStoredKeys(t, m, "resource_attributes/document:owner", "resource_attributes/document:pages",
		"resources/document")

	send(t, m, http.MethodPost, resourcesPath,
		`{"key": "document", "name": "Again", "actions": {}}`, http.StatusConflict)
	send(t, m, http.MethodGet, resourcesPath+"/missing", "", http.StatusNotFound)

	updated := send(t, m, http.MethodPatch, resourcesPath+"/document", `{
		"name": "Shared",
		"actions": {
			"read": {"name": "View"},
			"write": {"name": "Write", "description": "Change the text"},
			"delete": {"name": "Delete"}
		},
		"attributes": {
			"owner": {"type": "json", "description": "o"},
			"pages": {"type": "number", "description": "p"},
			"labels": {"type": "array"}
		}
	}`, http.StatusOK)

	wantUpdated := `{
		"id": "` + ObjectID(1) + `", "key": "document", "name": "Shared", "urn": "prn:document",
		"actions": {
			"delete": {"id": "` + ObjectID(6) + `", "key": "delete", "name": "Delete"},
			"read": {"id": "` + ObjectID(2) + `", "key": "read", "name": "View"},
			"write": {"id": "` + ObjectID(3) + `", "key": "write", "name": "Write",
				"description": "Change the text"}
		},
		"attributes": {
			"labels": {"id": "` + ObjectID(7) + `", "key": "labels", "type": "array"},
			"owner": {"id": "` + ObjectID(4) + `", "key": "owner", "type": "json",
				"description": "o"},
			"pages": {"id": "` + ObjectID(5) + `", "key": "pages", "type": "number",
				"description": "p"}
		}
	}`
	wantFields(t, updated, wantUpdated)
	wantFields(t, send(t, m, http.MethodGet, resourcesPath+"/document", "", http.StatusOK),
		wantUpdated)

	updated = send(t, m, http.MethodPatch, resourcesPath+"/"+ObjectID(1), `{"description": "d"}`,
		http.StatusOK)
	wantFields(t, updated, `{"name": "Shared", "description": "d"}`)
	wantFields(t, updated, wantUpdated)

	send(t, m, http.MethodDelete, resourcesPath+"/document", "", http.StatusNoContent)

	wantStoredKeys(t, m)
	send(t, m, http.MethodGet, resourcesPath+"/document", "", http.StatusNotFound)
	send(t, m, http.MethodPatch, resourcesPath+"/document", `{"name": "x"}`, http.StatusNotFound)
	send(t, m, http.MethodDelete, resourcesPath+"/document", "", http.StatusNotFound)
}

func TestResourceDeleteKeepsOtherResourcesAttributes(t *testing.T) {
	m := New(t, Resources, ResourceAttributes)
	for _, key := range []string{"document", "folder"} {
		send(t, m, http.MethodPost, resourcesPath, `{"key": "`+key+`", "name": "N",
			"actions": {}, "attributes": {"owner": {"type": "string"}}}`, http.StatusOK)
	}
	send(t, m, http.MethodPost, userAttributesPath, `{"key": "owner", "type": "string"}`,
		http.StatusOK)

	send(t, m, http.MethodDelete, resourcesPath+"/document", "", http.StatusNoContent)

	wantStoredKeys(t, m, "resource_attributes/__user:owner", "resource_attributes/folder:owner",
		"resources/folder")
}

func TestResourceRequestErrors(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   string
	}{
		{name: "create without key", method: http.MethodPost, body: `{"name": "D", "actions": {}}`},
		{
			name: "create without actions", method: http.MethodPost,
			body: `{"key": "d", "name": "D"}`,
		},
		{
			name: "create with actions not an object", method: http.MethodPost,
			body: `{"key": "d", "name": "D", "actions": []}`,
		},
		{
			name: "create with an attribute not an object", method: http.MethodPost,
			body: `{"key": "d", "name": "D", "actions": {}, "attributes": {"a": "string"}}`,
		},
		{
			name: "update with an action not an object", method: http.MethodPatch,
			body: `{"actions": {"read": "Read"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(t, Resources)
			send(t, m, http.MethodPost, resourcesPath, `{"key": "document", "name": "Document",
				"actions": {"read": {"name": "Read"}}}`, http.StatusOK)

			path := resourcesPath
			if tt.method == http.MethodPatch {
				path += "/document"
			}
			send(t, m, tt.method, path, tt.body, http.StatusUnprocessableEntity)

			wantStoredKeys(t, m, "resources/document")
			wantFields(t, send(t, m, http.MethodGet, resourcesPath+"/document", "", http.StatusOK),
				`{"actions": {"read": {"id": "`+ObjectID(2)+`", "key": "read", "name": "Read"}}}`)
		})
	}
}

// TestResourcePatchUnconfirmedBehaviour checks that the fake fails the test,
// rather than guess, on a resource PATCH whose effect on the API is unconfirmed.
func TestResourcePatchUnconfirmedBehaviour(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError string
	}{
		{
			name: "action left out", body: `{"actions": {"read": {"name": "Read"}}}`,
			wantError: `["actions.write"]`,
		},
		{
			name: "every action left out", body: `{"actions": {}}`,
			wantError: `["actions.read" "actions.write"]`,
		},
		{
			name:      "action description left out",
			body:      `{"actions": {"read": {"name": "Read"}, "write": {"name": "Write"}}}`,
			wantError: `["actions.write.description"]`,
		},
		{
			name:      "attribute left out",
			body:      `{"attributes": {"pages": {"type": "number"}}}`,
			wantError: `["attributes.owner"]`,
		},
		{
			name:      "attribute description left out",
			body:      `{"attributes": {"owner": {"type": "string"}}}`,
			wantError: `["attributes.owner.description"]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := New(rec, Resources)
			const resource = `{"key": "document", "name": "Document",
				"actions": {
					"read": {"name": "Read"},
					"write": {"name": "Write", "description": "w"}
				},
				"attributes": {"owner": {"type": "string", "description": "o"}}}`
			before := send(t, m, http.MethodPost, resourcesPath, resource, http.StatusOK)

			send(t, m, http.MethodPatch, resourcesPath+"/document", tt.body,
				http.StatusNotImplemented)

			got := rec.take()
			if len(got) != 1 || !strings.Contains(got[0], tt.wantError) ||
				!strings.Contains(got[0], "unconfirmed") {
				t.Errorf("test errors = %q, want one naming %s as unconfirmed", got, tt.wantError)
			}
			after := send(t, m, http.MethodGet, resourcesPath+"/document", "", http.StatusOK)
			if !reflect.DeepEqual(after, before) {
				t.Errorf("resource after the refused PATCH = %v, want it unchanged: %v",
					after, before)
			}
		})
	}
}

// TestResourcePatchNestedMaps checks what a resource PATCH does with a nested map,
// as the API does: attributes left out or null stay, {} deletes every attribute,
// and a null actions is a 422, also on a resource without actions.
func TestResourcePatchNestedMaps(t *testing.T) {
	m := New(t, Resources)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "document", "name": "Document",
		"actions": {}, "attributes": {"owner": {"type": "string"}}}`, http.StatusOK)
	withOwner := `{"attributes": {"owner": {"id": "` + ObjectID(2) + `", "key": "owner",
		"type": "string"}}}`

	for _, body := range []string{`{"name": "Shared"}`, `{"attributes": null}`} {
		wantFields(t, send(t, m, http.MethodPatch, resourcesPath+"/document", body,
			http.StatusOK), withOwner)
	}
	send(t, m, http.MethodPatch, resourcesPath+"/document", `{"actions": null}`,
		http.StatusUnprocessableEntity)
	wantStoredKeys(t, m, "resource_attributes/document:owner", "resources/document")

	cleared := send(t, m, http.MethodPatch, resourcesPath+"/document", `{"attributes": {}}`,
		http.StatusOK)

	want := `{"id": "` + ObjectID(1) + `", "key": "document", "name": "Shared", "actions": {},
		"attributes": {}}`
	wantFields(t, cleared, want)
	wantFields(t, send(t, m, http.MethodGet, resourcesPath+"/document", "", http.StatusOK), want)
	wantStoredKeys(t, m, "resources/document")
}

func TestUserAttributeState(t *testing.T) {
	m := New(t, ResourceAttributes)

	created := send(t, m, http.MethodPost, userAttributesPath,
		`{"key": "department", "type": "string", "description": "d"}`, http.StatusOK)

	wantAttribute := `{
		"id": "` + ObjectID(1) + `", "key": "department", "type": "string", "description": "d",
		"resource_id": "` + UserResourceID + `", "resource_key": "__user", "built_in": false,
		"organization_id": "` + OrganizationID + `", "project_id": "` + ProjectID + `",
		"environment_id": "` + EnvironmentID + `",
		"created_at": "` + timestamp + `", "updated_at": "` + timestamp + `"
	}`
	wantFields(t, created, wantAttribute)
	wantFields(t, send(t, m, http.MethodGet, userAttributesPath+"/department", "", http.StatusOK),
		wantAttribute)
	byID := resourcesPath + "/" + UserResourceID + "/attributes/" + ObjectID(1)
	wantFields(t, send(t, m, http.MethodGet, byID, "", http.StatusOK), wantAttribute)
	send(t, m, http.MethodPost, userAttributesPath, `{"key": "department", "type": "bool"}`,
		http.StatusConflict)
	send(t, m, http.MethodPost, userAttributesPath, `{"key": "team"}`,
		http.StatusUnprocessableEntity)
	send(t, m, http.MethodPost, resourcesPath+"/missing/attributes",
		`{"key": "team", "type": "string"}`, http.StatusNotFound)
	wantStoredKeys(t, m, "resource_attributes/__user:department")

	updated := send(t, m, http.MethodPatch, userAttributesPath+"/"+ObjectID(1),
		`{"type": "array", "description": "ds"}`, http.StatusOK)

	wantFields(t, updated, `{"key": "department", "type": "array", "description": "ds"}`)
	wantFields(t, send(t, m, http.MethodGet, userAttributesPath+"/department", "", http.StatusOK),
		`{"id": "`+ObjectID(1)+`", "type": "array", "description": "ds"}`)

	send(t, m, http.MethodDelete, userAttributesPath+"/department", "", http.StatusNoContent)

	wantStoredKeys(t, m)
	send(t, m, http.MethodGet, userAttributesPath+"/department", "", http.StatusNotFound)
	send(t, m, http.MethodPatch, userAttributesPath+"/department", `{"type": "bool"}`,
		http.StatusNotFound)
	send(t, m, http.MethodDelete, userAttributesPath+"/department", "", http.StatusNotFound)
}

// TestAttributesOfAResource checks that the attributes a resource defines and the
// ones created through the attribute routes are the same objects.
func TestAttributesOfAResource(t *testing.T) {
	m := New(t, Resources, ResourceAttributes)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "document", "name": "Document",
		"actions": {}, "attributes": {"owner": {"type": "string"}}}`, http.StatusOK)

	send(t, m, http.MethodPost, resourcesPath+"/document/attributes",
		`{"key": "pages", "type": "number"}`, http.StatusOK)
	send(t, m, http.MethodPatch, resourcesPath+"/document/attributes/owner",
		`{"description": "o"}`, http.StatusOK)

	wantFields(t, send(t, m, http.MethodGet, resourcesPath+"/document", "", http.StatusOK), `{
		"attributes": {
			"owner": {"id": "`+ObjectID(2)+`", "key": "owner", "type": "string",
				"description": "o"},
			"pages": {"id": "`+ObjectID(3)+`", "key": "pages", "type": "number"}
		}
	}`)
	wantFields(t, send(t, m, http.MethodGet, resourcesPath+"/document/attributes/owner", "",
		http.StatusOK), `{"resource_id": "`+ObjectID(1)+`", "resource_key": "document"}`)
}
