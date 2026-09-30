package mockpermit

import (
	"maps"
	"net/http"
	"slices"
)

const (
	tenantCollection = "tenants"
	tenantsPattern   = "/v2/facts/{proj_id}/{env_id}/tenants"
	tenantPattern    = tenantsPattern + "/{tenant_id}"
)

// Tenants serves the tenant operations permitio_tenant calls: create, get, update
// and delete by key. A tenant without attributes has the spec's default of none.
var Tenants = Routes{
	{"POST " + tenantsPattern, "Tenants.Create", (*Server).createTenant},
	{"GET " + tenantPattern, "Tenants.Get", (*Server).getTenant},
	{"PATCH " + tenantPattern, "Tenants.Update", (*Server).updateTenant},
	{"DELETE " + tenantPattern, "Tenants.Delete", (*Server).deleteTenant},
}

// TenantList serves the tenant list that
// permitio_group_resource_instance_role_assignment reads the project and
// environment IDs from. It is not in Tenants because permitio_tenant never lists
// tenants, and a test of it requires every route of Tenants.
var TenantList = Routes{
	{"GET " + tenantsPattern, "Tenants.List", (*Server).listTenants},
}

func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok || !s.knownFieldsOnly(w, body, "key", "name", "description", "attributes") ||
		!s.objectIfGiven(w, body, "attributes") {
		return
	}
	key := str(body, "key")
	if key == "" || str(body, "name") == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"key and name are required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tenants := s.collection(tenantCollection)
	if _, exists := tenants[key]; exists {
		s.writeError(w, http.StatusConflict, "DUPLICATE_ENTITY", "tenant "+key+" already exists")
		return
	}
	tenant := s.newObject()
	tenant["last_action_at"] = timestamp
	tenant["attributes"] = map[string]any{}
	maps.Copy(tenant, body)
	tenants[key] = tenant
	s.writeJSON(w, http.StatusOK, tenant)
}

// listTenants returns a page of the tenants in the order they were created, as a
// list. It fails the test on a query parameter other than page and per_page, such
// as the spec's search and include_total_count, which the fake does not model.
func (s *Server) listTenants(w http.ResponseWriter, r *http.Request) {
	page, ok := s.listQuery(w, r, 100)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tenants := slices.SortedFunc(maps.Values(s.collection(tenantCollection)), byID)
	s.writeJSON(w, http.StatusOK, page.of(tenants))
}

func (s *Server) getTenant(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tenant, ok := s.collection(tenantCollection)[r.PathValue("tenant_id")]
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "tenant not found")
		return
	}
	s.writeJSON(w, http.StatusOK, tenant)
}

func (s *Server) updateTenant(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok || !s.knownFieldsOnly(w, body, "name", "description", "attributes") ||
		!s.objectIfGiven(w, body, "attributes") {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tenant, ok := s.collection(tenantCollection)[r.PathValue("tenant_id")]
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "tenant not found")
		return
	}
	maps.Copy(tenant, body)
	s.writeJSON(w, http.StatusOK, tenant)
}

func (s *Server) deleteTenant(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tenants, key := s.collection(tenantCollection), r.PathValue("tenant_id")
	if _, ok := tenants[key]; !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "tenant not found")
		return
	}
	delete(tenants, key)
	w.WriteHeader(http.StatusNoContent)
}
