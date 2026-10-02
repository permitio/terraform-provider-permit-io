package mockpermit

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
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

// byID orders objects by ID, which is the order the fake created them in.
func byID(a, b map[string]any) int {
	return strings.Compare(str(a, "id"), str(b, "id"))
}

// exists reports whether a collection holds an object whose key or ID is keyOrID.
// The caller holds s.mu.
func (s *Server) exists(collection, keyOrID string) bool {
	_, ok := s.find(collection, keyOrID)
	return ok
}

// listPage is the page a list request asks for: its number, from 1, and its size.
type listPage struct {
	number, size int
}

// listQuery reads the page and per_page query parameters of a list request, with
// the spec's defaults of 1 and 30. It answers 422 and returns false when either is
// not a whole number from 1, or per_page is over maxPerPage. It fails the test and
// returns false on a query parameter that is not page, per_page or one of filters,
// or that is given more than once, since the fake does not model it.
func (s *Server) listQuery(w http.ResponseWriter, r *http.Request, maxPerPage int,
	filters ...string,
) (listPage, bool) {
	query := r.URL.Query()
	for _, name := range slices.Sorted(maps.Keys(query)) {
		modelled := name == "page" || name == "per_page" || slices.Contains(filters, name)
		if !modelled || len(query[name]) > 1 {
			parameter := name
			if modelled {
				parameter += " given more than once"
			}
			s.t.Errorf("mockpermit: %s %s: the fake does not model the query parameter %s; "+
				"add it to the mock if the provider should send it",
				r.Method, r.URL.Path, parameter)
			s.writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
				"not modelled in mockpermit")
			return listPage{}, false
		}
	}
	page := listPage{number: 1, size: 30}
	for name, value := range map[string]*int{"page": &page.number, "per_page": &page.size} {
		if !query.Has(name) {
			continue
		}
		number, err := strconv.Atoi(query.Get(name))
		if err != nil || number < 1 || name == "per_page" && number > maxPerPage {
			s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
				fmt.Sprintf("%s must be a whole number from 1, and per_page at most %d",
					name, maxPerPage))
			return listPage{}, false
		}
		*value = number
	}
	return page, true
}

// of returns the objects on the page, as a list that is empty rather than null
// past the last page.
func (p listPage) of(objects []map[string]any) []map[string]any {
	onPage := []map[string]any{}
	if p.number-1 > len(objects)/p.size {
		return onPage
	}
	start := min((p.number-1)*p.size, len(objects))
	end := min(start+p.size, len(objects))
	return append(onPage, objects[start:end]...)
}

// count returns how many pages the objects fill.
func (p listPage) count(objects []map[string]any) int {
	return (len(objects) + p.size - 1) / p.size
}
