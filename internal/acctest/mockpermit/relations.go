package mockpermit

import (
	"maps"
	"net/http"
)

const (
	relationCollection = "relations"
	relationsPattern   = resourcePattern + "/relations"
	relationPattern    = relationsPattern + "/{relation_id}"
)

// ResourceRelations serves the relation operations permitio_relation calls on
// the relation's object resource: create, and get and delete by key or ID. The
// subject resource in a create body may be a key or an ID; the fake returns keys
// and IDs for both resources, as the API does.
var ResourceRelations = Routes{
	{"POST " + relationsPattern, "ResourceRelations.Create", (*Server).createRelation},
	{"GET " + relationPattern, "ResourceRelations.Get", (*Server).getRelation},
	{"DELETE " + relationPattern, "ResourceRelations.Delete", (*Server).deleteRelation},
}

func (s *Server) createRelation(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok {
		return
	}
	key := str(body, "key")
	if key == "" || str(body, "subject_resource") == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"key and subject_resource are required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	object, ok := s.pathResource(w, r)
	if !ok {
		return
	}
	subject, ok := s.parentResource(w, str(body, "subject_resource"))
	if !ok {
		return
	}
	relations := s.collection(relationCollection)
	storedKey := childKey(str(object, "key"), key)
	if _, exists := relations[storedKey]; exists {
		s.writeError(w, http.StatusConflict, "DUPLICATE_ENTITY", "relation "+key+" already exists")
		return
	}
	relation := s.newObject()
	maps.Copy(relation, body)
	relation["subject_resource"] = subject["key"]
	relation["subject_resource_id"] = subject["id"]
	relation["object_resource"] = object["key"]
	relation["object_resource_id"] = object["id"]
	relations[storedKey] = relation
	s.writeJSON(w, http.StatusOK, relation)
}

func (s *Server) getRelation(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, relation, ok := s.pathRelation(w, r); ok {
		s.writeJSON(w, http.StatusOK, relation)
	}
}

func (s *Server) deleteRelation(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if storedKey, _, ok := s.pathRelation(w, r); ok {
		delete(s.collection(relationCollection), storedKey)
		w.WriteHeader(http.StatusNoContent)
	}
}

// pathRelation returns the relation the request path names and the key it is
// stored under, or writes a 404 and returns false. The caller holds s.mu.
func (s *Server) pathRelation(w http.ResponseWriter, r *http.Request) (
	string, map[string]any, bool,
) {
	object, ok := s.pathResource(w, r)
	if !ok {
		return "", nil, false
	}
	storedKey, relation, ok := s.findChild(relationCollection, str(object, "key"),
		r.PathValue("relation_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "relation not found")
	}
	return storedKey, relation, ok
}
