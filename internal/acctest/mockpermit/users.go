package mockpermit

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
)

const (
	userCollection = "users"
	userPattern    = "/v2/facts/{proj_id}/{env_id}/users/{user_id}"
)

// Users serves the user operation the permitio_user data source calls: get by key
// or ID. The provider does not create users, so a test adds the users it needs with
// AddUser.
var Users = Routes{
	{"GET " + userPattern, "Users.Get", (*Server).getUser},
}

// AddUser stores a user the way the API creates one from body, a UserCreate JSON
// object, and returns the user's ID. The provider has no user resource, so a test of
// the user data source or of a role assignment adds its users this way, and after
// destroy checks with CheckStored that only they are left. A user without
// attributes has the spec's default of none. The fake leaves the user's roles and
// tenants out of the user it returns, since the provider never reads them. AddUser
// fails the test on a body the API would reject, and on role_assignments, which the
// fake does not model.
func (s *Server) AddUser(body string) string {
	s.t.Helper()
	var user map[string]any
	if err := json.Unmarshal([]byte(body), &user); err != nil || user == nil {
		s.t.Fatalf("mockpermit: AddUser: %q is not a JSON object: %v", body, err)
	}
	for field := range user {
		if !slices.Contains([]string{"key", "email", "first_name", "last_name", "attributes"},
			field) {
			s.t.Fatalf("mockpermit: AddUser: the fake does not model the user field %q", field)
		}
	}
	key := str(user, "key")
	if _, ok := user["attributes"].(map[string]any); key == "" ||
		user["attributes"] != nil && !ok {
		s.t.Fatalf("mockpermit: AddUser: %s has no key, or attributes that are not an object",
			body)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	users := s.collection(userCollection)
	if _, exists := users[key]; exists {
		s.t.Fatalf("mockpermit: AddUser: the user %s already exists", key)
	}
	stored := s.newObject()
	stored["attributes"] = map[string]any{}
	maps.Copy(stored, user)
	users[key] = stored
	return str(stored, "id")
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.find(userCollection, r.PathValue("user_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}
	s.writeJSON(w, http.StatusOK, user)
}
