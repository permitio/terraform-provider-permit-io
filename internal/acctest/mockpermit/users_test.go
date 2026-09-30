package mockpermit

import (
	"net/http"
	"testing"
)

func TestUserState(t *testing.T) {
	m := New(t, Users)

	id := m.AddUser(`{"key": "alice", "email": "alice@example.com", "first_name": "Alice",
		"last_name": "Doe", "attributes": {"team": "docs"}}`)
	bobID := m.AddUser(`{"key": "bob"}`)

	if id != ObjectID(1) || bobID != ObjectID(2) {
		t.Errorf("AddUser() = %q and %q, want %q and %q", id, bobID, ObjectID(1), ObjectID(2))
	}
	wantAlice := `{
		"id": "` + ObjectID(1) + `", "key": "alice", "email": "alice@example.com",
		"first_name": "Alice", "last_name": "Doe", "attributes": {"team": "docs"},
		"organization_id": "` + OrganizationID + `", "project_id": "` + ProjectID + `",
		"environment_id": "` + EnvironmentID + `",
		"created_at": "` + timestamp + `", "updated_at": "` + timestamp + `"
	}`
	wantObject(t, send(t, m, http.MethodGet, factsPath+"/users/alice", "", http.StatusOK),
		wantAlice)
	wantObject(t, send(t, m, http.MethodGet, factsPath+"/users/"+ObjectID(1), "", http.StatusOK),
		wantAlice)
	wantFields(t, send(t, m, http.MethodGet, factsPath+"/users/bob", "", http.StatusOK),
		`{"key": "bob", "attributes": {}}`)
	send(t, m, http.MethodGet, factsPath+"/users/carol", "", http.StatusNotFound)
	wantStoredKeys(t, m, "users/alice", "users/bob")
}
