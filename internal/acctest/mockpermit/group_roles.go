package mockpermit

import (
	"maps"
	"net/http"
	"slices"
	"strings"
)

const (
	groupCollection     = "groups"
	groupRoleCollection = "group_roles"
	groupRolesPattern   = "/v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/roles"
	// groupRolesPackage is the provider package that sends the group role requests
	// with net/http.
	groupRolesPackage = "group_resource_instance_role_assignments"
)

// GroupRoles serves the group role requests that
// permitio_group_resource_instance_role_assignment builds itself with net/http:
// assign a role on a resource instance to a group, remove it, and list the group's
// roles a page at a time. The group must have been added with AddGroup. A body
// names the role, its resource, the resource instance and the tenant by key. The
// API also creates a relation, relationships and a derivation for an assignment;
// the fake does not model them, since the provider never reads them. It fails the
// test rather than guess on an assignment the group already has, on the removal of
// one it does not have, and on one on a resource instance that does not exist or is
// in another tenant.
var GroupRoles = Routes{
	{"POST " + groupRolesPattern, groupRolesPackage + ".Create (HTTP)", (*Server).assignGroupRole},
	{
		"GET " + groupRolesPattern, groupRolesPackage + ".listRolesPage (HTTP)",
		(*Server).listGroupRoles,
	},
	{
		"DELETE " + groupRolesPattern, groupRolesPackage + ".Delete (HTTP)",
		(*Server).removeGroupRole,
	},
}

// AddGroup stores a group of the tenant with this key, which the fake does not
// check, and returns its ID. The provider has no group resource, so a test of a
// group role assignment adds its group this way, and after destroy checks with
// CheckStored that only it is left.
func (s *Server) AddGroup(key, tenant string) string {
	s.t.Helper()
	if key == "" || tenant == "" {
		s.t.Fatalf("mockpermit: AddGroup: the key %q and the tenant %q must not be empty",
			key, tenant)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	groups := s.collection(groupCollection)
	if _, exists := groups[key]; exists {
		s.t.Fatalf("mockpermit: AddGroup: the group %s already exists", key)
	}
	id := s.newID()
	groups[key] = map[string]any{
		"id": id, "group_resource_type_key": "group", "group_instance_key": key,
		"group_tenant": tenant,
	}
	return id
}

func (s *Server) assignGroupRole(w http.ResponseWriter, r *http.Request) {
	s.withGroupRole(w, r, func(group map[string]any, storedKey string, role map[string]any) {
		roles := s.collection(groupRoleCollection)
		if _, exists := roles[storedKey]; exists {
			s.refuseUnconfirmed(w, r, "the group already has this role")
			return
		}
		role["id"] = s.newID()
		roles[storedKey] = role
		read := maps.Clone(group)
		delete(read, "id")
		s.writeJSON(w, http.StatusOK, read)
	})
}

func (s *Server) removeGroupRole(w http.ResponseWriter, r *http.Request) {
	s.withGroupRole(w, r, func(_ map[string]any, storedKey string, _ map[string]any) {
		roles := s.collection(groupRoleCollection)
		if _, exists := roles[storedKey]; !exists {
			s.refuseUnconfirmed(w, r, "the group does not have this role")
			return
		}
		delete(roles, storedKey)
		w.WriteHeader(http.StatusNoContent)
	})
}

// withGroupRole reads a GroupAddRole body and resolves the group in the path and
// the objects the body names. It then calls do, holding s.mu, with the group, the
// key the fake stores the assignment under, and the role as the list returns it.
// It writes an error instead when the body is not a GroupAddRole or names an object
// that does not exist. Unlike most request models in the spec, GroupAddRole allows
// other fields, so the fake ignores them.
func (s *Server) withGroupRole(w http.ResponseWriter, r *http.Request,
	do func(group map[string]any, storedKey string, role map[string]any),
) {
	body, ok := s.decodeObject(w, r)
	if !ok {
		return
	}
	fields := []string{"role", "resource", "resource_instance", "tenant"}
	for _, field := range fields {
		if str(body, field) == "" {
			s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
				"role, resource, resource_instance and tenant are required")
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	group, ok := s.find(groupCollection, r.PathValue("group_instance_key"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "group not found")
		return
	}
	resource, ok := s.findKey(w, r, resourceCollection, str(body, "resource"), "resource")
	if !ok {
		return
	}
	tenant, ok := s.findKey(w, r, tenantCollection, str(body, "tenant"), "tenant")
	if !ok {
		return
	}
	instance, ok := s.tenantInstance(w, r, str(resource, "key"), str(body, "resource_instance"),
		str(tenant, "key"))
	if !ok {
		return
	}
	role, ok := s.resourceRole(w, r, str(resource, "key"), str(body, "role"))
	if !ok {
		return
	}
	var keys []string
	for _, field := range fields {
		keys = append(keys, str(body, field))
	}
	do(group, str(group, "group_instance_key")+":"+strings.Join(keys, ":"), map[string]any{
		"key":               role["key"],
		"resource":          map[string]any{"id": resource["id"], "key": resource["key"]},
		"resource_instance": map[string]any{"id": instance["id"], "key": instance["key"]},
	})
}

// listGroupRoles returns a page of the roles of the group in the path, in the
// order they were assigned, with the total count.
func (s *Server) listGroupRoles(w http.ResponseWriter, r *http.Request) {
	page, ok := s.listQuery(w, r, 100)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	group, ok := s.find(groupCollection, r.PathValue("group_instance_key"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "group not found")
		return
	}
	var assigned []map[string]any
	prefix := str(group, "group_instance_key") + ":"
	for storedKey, stored := range s.collection(groupRoleCollection) {
		if strings.HasPrefix(storedKey, prefix) {
			assigned = append(assigned, stored)
		}
	}
	slices.SortFunc(assigned, byID)
	roles := []map[string]any{}
	for _, stored := range assigned {
		role := maps.Clone(stored)
		delete(role, "id")
		roles = append(roles, role)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"data":        page.of(roles),
		"total_count": len(roles),
		"page_count":  page.count(roles),
	})
}
