package mockpermit

import (
	"maps"
	"net/http"
)

const (
	tenantCollection = "tenants"
	tenantsPattern   = "/v2/facts/{proj_id}/{env_id}/tenants"
	tenantPattern    = tenantsPattern + "/{tenant_id}"
)

// Tenants serves the tenant operations permitio_tenant calls: create, get, update
// and delete by key.
var Tenants = Routes{
	{"POST " + tenantsPattern, "Tenants.Create", (*Server).createTenant},
	{"GET " + tenantPattern, "Tenants.Get", (*Server).getTenant},
	{"PATCH " + tenantPattern, "Tenants.Update", (*Server).updateTenant},
	{"DELETE " + tenantPattern, "Tenants.Delete", (*Server).deleteTenant},
}

func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok {
		return
	}
	key, _ := body["key"].(string)
	if key == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "key is required")
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
	maps.Copy(tenant, body)
	tenants[key] = tenant
	s.writeJSON(w, http.StatusOK, tenant)
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
	if !ok {
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
