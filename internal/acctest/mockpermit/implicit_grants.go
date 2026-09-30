package mockpermit

import (
	"maps"
	"net/http"
	"slices"
	"strings"
)

const (
	implicitGrantCollection = "implicit_grants"
	implicitGrantsPattern   = resourceRolePattern + "/implicit_grants"
)

// ImplicitGrants serves the derivation operations permitio_role_derivation calls:
// create and delete an implicit grant of a resource role. The body names a role on
// another resource and the relation that links the two resources; the relation
// belongs to the role's resource, with the other resource as its subject. The fake
// lists the grants of a role in the role's granted_to, which is where the provider
// reads a derivation back.
var ImplicitGrants = Routes{
	{"POST " + implicitGrantsPattern, "ImplicitGrants.Create", (*Server).createImplicitGrant},
	{"DELETE " + implicitGrantsPattern, "ImplicitGrants.Delete", (*Server).deleteImplicitGrant},
}

func (s *Server) createImplicitGrant(w http.ResponseWriter, r *http.Request) {
	s.withGrant(w, r, func(storedKey string, grant map[string]any) {
		grants := s.collection(implicitGrantCollection)
		if _, exists := grants[storedKey]; exists {
			s.writeError(w, http.StatusConflict, "DUPLICATE_ENTITY", "the grant already exists")
			return
		}
		grants[storedKey] = grant
		s.writeJSON(w, http.StatusOK, grant["rule"])
	})
}

func (s *Server) deleteImplicitGrant(w http.ResponseWriter, r *http.Request) {
	s.withGrant(w, r, func(storedKey string, _ map[string]any) {
		grants := s.collection(implicitGrantCollection)
		if _, exists := grants[storedKey]; !exists {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "the grant does not exist")
			return
		}
		delete(grants, storedKey)
		w.WriteHeader(http.StatusNoContent)
	})
}

// withGrant resolves the implicit grant a create or delete request describes and
// calls do, holding s.mu, with the key the grant is stored under and the grant. It
// writes a 422 or 404 instead when the body is incomplete or names an object that
// does not exist.
func (s *Server) withGrant(w http.ResponseWriter, r *http.Request,
	do func(storedKey string, grant map[string]any),
) {
	body, ok := s.decodeObject(w, r)
	if !ok {
		return
	}
	if str(body, "role") == "" || str(body, "on_resource") == "" ||
		str(body, "linked_by_relation") == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"role, on_resource and linked_by_relation are required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	resource, _, toRole, ok := s.pathRole(w, r)
	if !ok {
		return
	}
	onResource, ok := s.parentResource(w, str(body, "on_resource"))
	if !ok {
		return
	}
	_, role, ok := s.findChild(resourceRoleCollection, str(onResource, "key"), str(body, "role"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "role not found on on_resource")
		return
	}
	_, relation, ok := s.findChild(relationCollection, str(resource, "key"),
		str(body, "linked_by_relation"))
	if !ok || relation["subject_resource"] != onResource["key"] {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND",
			"no relation with this key from on_resource to the role's resource")
		return
	}
	when := body["when"]
	if when == nil {
		when = map[string]any{"no_direct_roles_on_object": false}
	}
	grant := map[string]any{
		"resource": resource["key"],
		"to_role":  toRole["key"],
		"rule": map[string]any{
			"role_id":            role["id"],
			"resource_id":        onResource["id"],
			"relation_id":        relation["id"],
			"role":               role["key"],
			"on_resource":        onResource["key"],
			"linked_by_relation": relation["key"],
			"when":               when,
		},
	}
	storedKey := strings.Join([]string{
		str(resource, "key"), str(toRole, "key"), str(onResource, "key"), str(role, "key"),
		str(relation, "key"),
	}, ":")
	do(storedKey, grant)
}

// grantsTo returns the rules of the implicit grants of a resource role, in a fixed
// order. The caller holds s.mu.
func (s *Server) grantsTo(resourceKey, roleKey string) []any {
	grants := s.collection(implicitGrantCollection)
	rules := []any{}
	for _, storedKey := range slices.Sorted(maps.Keys(grants)) {
		grant := grants[storedKey]
		if grant["resource"] == resourceKey && grant["to_role"] == roleKey {
			rules = append(rules, grant["rule"])
		}
	}
	return rules
}
