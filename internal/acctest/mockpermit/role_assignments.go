package mockpermit

import (
	"maps"
	"net/http"
	"slices"
	"strings"
)

const (
	roleAssignmentCollection = "role_assignments"
	userRolesPattern         = userPattern + "/roles"
	roleAssignmentsPattern   = "/v2/facts/{proj_id}/{env_id}/role_assignments"
)

// RoleAssignments serves the role assignment operations permitio_role_assignment
// and permitio_resource_instance_role_assignment call: assign a role to a user, in a
// tenant or on a resource instance, and unassign it; and list the assignments of a
// user, role and tenant a page at a time. The SDK sends the same request to assign
// a role in a tenant and on an instance, and to unassign one, so those operations
// share their routes. The user must have been added with AddUser. A body names the
// role, the tenant and the resource instance by key, and the fake returns the keys
// and IDs of all of them, as the API does. It fails the test rather than guess on an
// assignment the user already has, and on one on a resource instance that does not
// exist, which the API creates implicitly, or that is in another tenant. The list
// includes assignments on resource instances, as the API documents, and fails the
// test on a matching assignment whose user, role, tenant or instance was deleted.
var RoleAssignments = Routes{
	{"POST " + userRolesPattern, "Users.AssignRole", (*Server).assignUserRole},
	{"POST " + userRolesPattern, "Users.AssignResourceRole", (*Server).assignUserRole},
	{"DELETE " + userRolesPattern, "Users.UnassignRole", (*Server).unassignUserRole},
	{"DELETE " + userRolesPattern, "Users.UnassignResourceRole", (*Server).unassignUserRole},
	{"GET " + roleAssignmentsPattern, "RoleAssignments.List", (*Server).listRoleAssignments},
}

func (s *Server) assignUserRole(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok || !s.knownFieldsOnly(w, body, "role", "tenant", "resource_instance") {
		return
	}
	if str(body, "role") == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "role is required")
		return
	}
	if str(body, "tenant") == "" {
		s.refuseUnconfirmed(w, r, "the body names no tenant")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	storedKey, assignment, ok := s.userRole(w, r, body)
	if !ok {
		return
	}
	assignments := s.collection(roleAssignmentCollection)
	if _, exists := assignments[storedKey]; exists {
		s.refuseUnconfirmed(w, r, "the user already has this role assignment")
		return
	}
	stored := s.newObject()
	delete(stored, "updated_at")
	maps.Copy(stored, assignment)
	assignments[storedKey] = stored
	s.writeJSON(w, http.StatusOK, stored)
}

// unassignUserRole removes the assignment, or answers 404 when the user does not
// have it, as the API documents.
func (s *Server) unassignUserRole(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok || !s.knownFieldsOnly(w, body, "role", "tenant", "resource_instance") {
		return
	}
	if str(body, "role") == "" || str(body, "tenant") == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"role and tenant are required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	storedKey, _, ok := s.userRole(w, r, body)
	if !ok {
		return
	}
	assignments := s.collection(roleAssignmentCollection)
	if _, exists := assignments[storedKey]; !exists {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "the role is not assigned")
		return
	}
	delete(assignments, storedKey)
	w.WriteHeader(http.StatusNoContent)
}

// userRole resolves the user in the path and the role, tenant and resource
// instance in the body of an assignment request. It returns the key the fake
// stores the assignment under and the fields the API returns for them, or writes an
// error and returns false. The caller holds s.mu.
func (s *Server) userRole(w http.ResponseWriter, r *http.Request, body map[string]any) (
	string, map[string]any, bool,
) {
	user, ok := s.find(userCollection, r.PathValue("user_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return "", nil, false
	}
	tenant, ok := s.findKey(w, r, tenantCollection, str(body, "tenant"), "tenant")
	if !ok {
		return "", nil, false
	}
	assignment := map[string]any{
		"user": user["key"], "user_id": user["id"],
		"tenant": tenant["key"], "tenant_id": tenant["id"],
	}
	instanceRef := str(body, "resource_instance")
	var role map[string]any
	if instanceRef == "" {
		role, ok = s.findKey(w, r, roleCollection, str(body, "role"), "role")
	} else {
		resourceKey, instanceKey, _ := strings.Cut(instanceRef, ":")
		var instance map[string]any
		instance, ok = s.tenantInstance(w, r, resourceKey, instanceKey, str(tenant, "key"))
		if ok {
			role, ok = s.resourceRole(w, r, resourceKey, str(body, "role"))
			assignment["resource_instance"] = instanceRef
			assignment["resource_instance_id"] = instance["id"]
		}
	}
	if !ok {
		return "", nil, false
	}
	assignment["role"] = role["key"]
	assignment["role_id"] = role["id"]
	storedKey := strings.Join([]string{
		str(user, "key"), str(role, "key"), str(tenant, "key"), instanceRef,
	}, ":")
	return storedKey, assignment, true
}

// listRoleAssignments returns a page of the assignments that match the user, role
// and tenant filters, in the order they were made. A filter compares keys.
func (s *Server) listRoleAssignments(w http.ResponseWriter, r *http.Request) {
	page, ok := s.listQuery(w, r, 1000, "user", "role", "tenant")
	if !ok {
		return
	}
	query := r.URL.Query()
	s.mu.Lock()
	defer s.mu.Unlock()
	filters := map[string][]string{
		"user":   {userCollection},
		"role":   {roleCollection, resourceRoleCollection},
		"tenant": {tenantCollection},
	}
	for _, name := range slices.Sorted(maps.Keys(filters)) {
		for _, collection := range filters[name] {
			object, found := s.find(collection, query.Get(name))
			if query.Has(name) && found && object["key"] != query.Get(name) {
				s.refuseUnconfirmed(w, r, "the query names the "+name+" by ID")
				return
			}
		}
	}
	matching := []map[string]any{}
	assignments := s.collection(roleAssignmentCollection)
	for _, assignment := range slices.SortedFunc(maps.Values(assignments), byID) {
		if query.Has("user") && assignment["user"] != query.Get("user") ||
			query.Has("role") && assignment["role"] != query.Get("role") ||
			query.Has("tenant") && assignment["tenant"] != query.Get("tenant") {
			continue
		}
		if s.orphanedAssignment(assignment) {
			s.refuseUnconfirmed(w, r, "an assignment that matches the query has a user, role, "+
				"tenant or resource instance that was deleted")
			return
		}
		matching = append(matching, assignment)
	}
	s.writeJSON(w, http.StatusOK, page.of(matching))
}

// orphanedAssignment reports whether the user, role, tenant or resource instance
// of an assignment was deleted after it was made. The caller holds s.mu.
func (s *Server) orphanedAssignment(assignment map[string]any) bool {
	roleID := str(assignment, "role_id")
	instanceID := str(assignment, "resource_instance_id")
	instance, instanceExists := s.find(resourceInstanceCollection, instanceID)
	return !s.exists(userCollection, str(assignment, "user_id")) ||
		!s.exists(tenantCollection, str(assignment, "tenant_id")) ||
		!s.exists(roleCollection, roleID) && !s.exists(resourceRoleCollection, roleID) ||
		instanceID != "" && (!instanceExists || s.resourceDeleted(instance))
}

// tenantInstance returns the resource instance with this key of the resource, when
// it exists and is in the tenant. Otherwise it fails the test, since what the API
// does then is unconfirmed, and returns false. The caller holds s.mu.
func (s *Server) tenantInstance(w http.ResponseWriter, r *http.Request,
	resourceKey, key, tenantKey string,
) (map[string]any, bool) {
	name := childKey(resourceKey, key)
	instance, ok := s.collection(resourceInstanceCollection)[name]
	switch {
	case !ok || s.resourceDeleted(instance):
		s.refuseUnconfirmed(w, r, "the body names the resource instance "+name+
			", which does not exist")
	case str(instance, "tenant") != tenantKey:
		s.refuseUnconfirmed(w, r, "the body names the resource instance "+name+
			" of another tenant")
	default:
		return instance, true
	}
	return nil, false
}

// resourceRole returns the role with this key of the resource. It writes a 404
// when there is none, and fails the test when key is the ID of a resource role,
// since whether the API takes one there is unconfirmed. The caller holds s.mu.
func (s *Server) resourceRole(w http.ResponseWriter, r *http.Request, resourceKey, key string) (
	map[string]any, bool,
) {
	if role, ok := s.collection(resourceRoleCollection)[childKey(resourceKey, key)]; ok {
		return role, true
	}
	if s.exists(resourceRoleCollection, key) {
		s.refuseUnconfirmed(w, r, "the body names the role "+key+" by ID")
		return nil, false
	}
	s.writeError(w, http.StatusNotFound, "NOT_FOUND", "role "+key+" not found")
	return nil, false
}
