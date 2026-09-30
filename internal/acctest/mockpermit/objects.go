package mockpermit

import (
	"net/http"
	"strings"
)

// newObject returns the fields the API sets on an object it creates: a fresh ID,
// the scope IDs and the timestamps. The caller holds s.mu.
func (s *Server) newObject() map[string]any {
	return map[string]any{
		"id":              s.newID(),
		"organization_id": OrganizationID,
		"project_id":      ProjectID,
		"environment_id":  EnvironmentID,
		"created_at":      timestamp,
		"updated_at":      timestamp,
	}
}

// find returns the object in a collection whose key or ID is keyOrID, since the
// API takes either in a path. The caller holds s.mu.
func (s *Server) find(collection, keyOrID string) (map[string]any, bool) {
	objects := s.collection(collection)
	if object, ok := objects[keyOrID]; ok {
		return object, true
	}
	for _, object := range objects {
		if object["id"] == keyOrID {
			return object, true
		}
	}
	return nil, false
}

// findKey returns the object in a collection whose key is key, for a field where
// the API takes a key. When no object has that key but one has it as its ID, it
// fails the test, since whether the API also takes an ID there is unconfirmed.
// Otherwise it writes a 404 naming what. The caller holds s.mu.
func (s *Server) findKey(w http.ResponseWriter, r *http.Request, collection, key, what string) (
	map[string]any, bool,
) {
	objects := s.collection(collection)
	if object, ok := objects[key]; ok {
		return object, true
	}
	for _, object := range objects {
		if object["id"] == key {
			s.refuseUnconfirmed(w, r, "the body names the "+what+" "+key+" by ID")
			return nil, false
		}
	}
	s.writeError(w, http.StatusNotFound, "NOT_FOUND", what+" "+key+" not found")
	return nil, false
}

// childKey is the key the fake stores an object under when it belongs to a
// resource, such as a resource role: the two keys joined by a colon, which API
// keys cannot contain.
func childKey(parentKey, key string) string {
	return parentKey + ":" + key
}

// findChild returns the object in a collection that belongs to the resource with
// key parentKey and whose key or ID is keyOrID, and the key it is stored under.
// The caller holds s.mu.
func (s *Server) findChild(collection, parentKey, keyOrID string) (string, map[string]any, bool) {
	objects := s.collection(collection)
	if object, ok := objects[childKey(parentKey, keyOrID)]; ok {
		return childKey(parentKey, keyOrID), object, true
	}
	for storedKey, object := range objects {
		if strings.HasPrefix(storedKey, childKey(parentKey, "")) && object["id"] == keyOrID {
			return storedKey, object, true
		}
	}
	return "", nil, false
}

// str returns an object's string field, or "" when it is missing or not a string.
func str(object map[string]any, field string) string {
	value, _ := object[field].(string)
	return value
}
