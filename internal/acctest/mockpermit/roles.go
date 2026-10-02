package mockpermit

import (
	"maps"
	"net/http"
	"slices"
)

const (
	roleCollection         = "roles"
	rolesPattern           = "/v2/schema/{proj_id}/{env_id}/roles"
	rolePattern            = rolesPattern + "/{role_id}"
	resourceRoleCollection = "resource_roles"
	resourceRolesPattern   = resourcePattern + "/roles"
	resourceRolePattern    = resourceRolesPattern + "/{role_id}"
)

// Roles serves the top-level role operations permitio_role calls: create; get,
// update and delete by key or ID; and assign and remove permissions.
var Roles = Routes{
	{"POST " + rolesPattern, "Roles.Create", (*Server).createRole},
	{"GET " + rolePattern, "Roles.Get", (*Server).getRole},
	{"PATCH " + rolePattern, "Roles.Update", (*Server).updateRole},
	{"DELETE " + rolePattern, "Roles.Delete", (*Server).deleteRole},
	{
		"POST " + rolePattern + "/permissions", "Roles.AssignPermissions",
		(*Server).assignPermissions,
	},
	{
		"DELETE " + rolePattern + "/permissions", "Roles.RemovePermissions",
		(*Server).removePermissions,
	},
}

// ResourceRoles serves the same operations as Roles for the roles of a resource,
// which permitio_role manages when its resource is set. A resource role's
// granted_to lists the derivations that grant it; see ImplicitGrants.
var ResourceRoles = Routes{
	{"POST " + resourceRolesPattern, "ResourceRoles.Create", (*Server).createRole},
	{"GET " + resourceRolePattern, "ResourceRoles.Get", (*Server).getRole},
	{"PATCH " + resourceRolePattern, "ResourceRoles.Update", (*Server).updateRole},
	{"DELETE " + resourceRolePattern, "ResourceRoles.Delete", (*Server).deleteRole},
	{
		"POST " + resourceRolePattern + "/permissions", "ResourceRoles.AssignPermissions",
		(*Server).assignPermissions,
	},
	{
		"DELETE " + resourceRolePattern + "/permissions", "ResourceRoles.RemovePermissions",
		(*Server).removePermissions,
	},
}

func (s *Server) createRole(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok {
		return
	}
	key := str(body, "key")
	if key == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "key is required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	resource, ok := s.roleParent(w, r)
	if !ok {
		return
	}
	if _, _, exists := s.findRole(resource, key); exists {
		s.writeError(w, http.StatusConflict, "DUPLICATE_ENTITY", "role "+key+" already exists")
		return
	}
	role := s.newObject()
	role["permissions"] = []any{}
	role["extends"] = []any{}
	maps.Copy(role, body)
	if resource == nil {
		s.collection(roleCollection)[key] = role
	} else {
		role["resource_id"] = resource["id"]
		role["resource"] = resource["key"]
		s.collection(resourceRoleCollection)[childKey(str(resource, "key"), key)] = role
	}
	s.writeJSON(w, http.StatusOK, s.roleRead(resource, role))
}

func (s *Server) getRole(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if resource, _, role, ok := s.pathRole(w, r); ok {
		s.writeJSON(w, http.StatusOK, s.roleRead(resource, role))
	}
}

func (s *Server) updateRole(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if resource, _, role, ok := s.pathRole(w, r); ok {
		maps.Copy(role, body)
		s.writeJSON(w, http.StatusOK, s.roleRead(resource, role))
	}
}

func (s *Server) deleteRole(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resource, storedKey, _, ok := s.pathRole(w, r)
	if !ok {
		return
	}
	if resource == nil {
		delete(s.collection(roleCollection), storedKey)
	} else {
		delete(s.collection(resourceRoleCollection), storedKey)
	}
	w.WriteHeader(http.StatusNoContent)
}

// assignPermissions adds the permissions in the body that the role does not have
// yet, as the API does.
func (s *Server) assignPermissions(w http.ResponseWriter, r *http.Request) {
	s.changePermissions(w, r, func(current []any, permissions []string) []any {
		for _, permission := range permissions {
			if !slices.Contains(current, any(permission)) {
				current = append(current, permission)
			}
		}
		return current
	})
}

// removePermissions removes the permissions in the body from the role, skipping
// any it does not have, as the API does.
func (s *Server) removePermissions(w http.ResponseWriter, r *http.Request) {
	s.changePermissions(w, r, func(current []any, permissions []string) []any {
		return slices.DeleteFunc(current, func(permission any) bool {
			name, _ := permission.(string)
			return slices.Contains(permissions, name)
		})
	})
}

func (s *Server) changePermissions(w http.ResponseWriter, r *http.Request,
	change func(current []any, permissions []string) []any,
) {
	body, ok := s.decodeObject(w, r)
	if !ok {
		return
	}
	permissions, ok := stringList(body["permissions"])
	if !ok {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"permissions must be a list of strings")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	resource, _, role, ok := s.pathRole(w, r)
	if !ok {
		return
	}
	current, _ := role["permissions"].([]any)
	role["permissions"] = change(slices.Clone(current), permissions)
	s.writeJSON(w, http.StatusOK, s.roleRead(resource, role))
}

// roleParent returns the resource whose roles the request is about, or nil for a
// request about top-level roles. It writes a 404 and returns false when the
// resource does not exist. The caller holds s.mu.
func (s *Server) roleParent(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	if r.PathValue("resource_id") == "" {
		return nil, true
	}
	return s.pathResource(w, r)
}

// pathRole returns the role the request path names, its resource (nil for a
// top-level role) and the key it is stored under, or writes a 404 and returns
// false. The caller holds s.mu.
func (s *Server) pathRole(w http.ResponseWriter, r *http.Request) (
	resource map[string]any, storedKey string, role map[string]any, ok bool,
) {
	resource, ok = s.roleParent(w, r)
	if !ok {
		return nil, "", nil, false
	}
	storedKey, role, ok = s.findRole(resource, r.PathValue("role_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "role not found")
	}
	return resource, storedKey, role, ok
}

// findRole returns the role with this key or ID among the top-level roles, or
// among the roles of resource when it is not nil, and the key it is stored under.
// The caller holds s.mu.
func (s *Server) findRole(resource map[string]any, keyOrID string) (string, map[string]any, bool) {
	if resource == nil {
		role, ok := s.find(roleCollection, keyOrID)
		return str(role, "key"), role, ok
	}
	return s.findChild(resourceRoleCollection, str(resource, "key"), keyOrID)
}

// roleRead returns a role as the API returns it. A resource role has granted_to,
// with the derivations that grant it. The caller holds s.mu.
func (s *Server) roleRead(resource, role map[string]any) map[string]any {
	if resource == nil {
		return role
	}
	read := maps.Clone(role)
	read["granted_to"] = map[string]any{
		"id":              role["id"],
		"users_with_role": s.grantsTo(str(resource, "key"), str(role, "key")),
		"when":            map[string]any{"no_direct_roles_on_object": false},
	}
	return read
}

// stringList reads a JSON list of strings.
func stringList(value any) ([]string, bool) {
	list, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(list))
	for _, item := range list {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}
