package mockpermit

import (
	"maps"
	"net/http"
)

const (
	resourceInstanceCollection = "resource_instances"
	resourceInstancesPattern   = "/v2/facts/{proj_id}/{env_id}/resource_instances"
	resourceInstancePattern    = resourceInstancesPattern + "/{instance_id}"
)

// ResourceInstances serves the resource instance operations
// permitio_resource_instance calls: create, and get, update and delete by
// resource:key or ID. A create body names the resource and the tenant by key, and
// the fake returns the keys and IDs of both, as the API does. An instance without
// attributes has the spec's default of none. A PATCH with attributes replaces them
// whole, so attributes {} clear them, and one without keeps them, as the API does
// (PER-16603). The fake fails the test on a get, update or delete of an instance
// whose resource or tenant was deleted: the API deletes a resource or tenant with
// its related data, and whether that includes the instance is unconfirmed.
var ResourceInstances = Routes{
	{
		"POST " + resourceInstancesPattern, "ResourceInstances.Create",
		(*Server).createResourceInstance,
	},
	{"GET " + resourceInstancePattern, "ResourceInstances.Get", (*Server).getResourceInstance},
	{
		"PATCH " + resourceInstancePattern, "ResourceInstances.Update",
		(*Server).updateResourceInstance,
	},
	{
		"DELETE " + resourceInstancePattern, "ResourceInstances.Delete",
		(*Server).deleteResourceInstance,
	},
}

func (s *Server) createResourceInstance(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok || !s.knownFieldsOnly(w, body, "key", "tenant", "resource", "attributes") ||
		!s.objectIfGiven(w, body, "attributes") {
		return
	}
	key, resourceKey, tenantKey := str(body, "key"), str(body, "resource"), str(body, "tenant")
	if key == "" || resourceKey == "" || tenantKey == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"key, resource and tenant are required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	resource, ok := s.findKey(w, r, resourceCollection, resourceKey, "resource")
	if !ok {
		return
	}
	tenant, ok := s.findKey(w, r, tenantCollection, tenantKey, "tenant")
	if !ok {
		return
	}
	instances := s.collection(resourceInstanceCollection)
	storedKey := childKey(resourceKey, key)
	if _, exists := instances[storedKey]; exists {
		s.writeError(w, http.StatusConflict, "DUPLICATE_ENTITY",
			"resource instance "+storedKey+" already exists")
		return
	}
	instance := s.newObject()
	instance["attributes"] = map[string]any{}
	maps.Copy(instance, body)
	instance["resource_id"] = resource["id"]
	instance["tenant_id"] = tenant["id"]
	instances[storedKey] = instance
	s.writeJSON(w, http.StatusOK, instance)
}

func (s *Server) getResourceInstance(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if instance, ok := s.pathInstance(w, r); ok {
		s.writeJSON(w, http.StatusOK, instance)
	}
}

func (s *Server) updateResourceInstance(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok || !s.knownFieldsOnly(w, body, "attributes") ||
		!s.objectIfGiven(w, body, "attributes") {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if instance, ok := s.pathInstance(w, r); ok {
		maps.Copy(instance, body)
		s.writeJSON(w, http.StatusOK, instance)
	}
}

func (s *Server) deleteResourceInstance(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if instance, ok := s.pathInstance(w, r); ok {
		delete(s.collection(resourceInstanceCollection),
			childKey(str(instance, "resource"), str(instance, "key")))
		w.WriteHeader(http.StatusNoContent)
	}
}

// pathInstance returns the resource instance the request path names. It writes a
// 404 when there is none, and fails the test when the instance's resource or
// tenant was deleted; see ResourceInstances. The caller holds s.mu.
func (s *Server) pathInstance(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	instance, ok := s.find(resourceInstanceCollection, r.PathValue("instance_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "resource instance not found")
		return nil, false
	}
	_, tenantExists := s.find(tenantCollection, str(instance, "tenant_id"))
	if s.resourceDeleted(instance) || !tenantExists {
		s.refuseUnconfirmed(w, r, "the path names a resource instance whose resource or "+
			"tenant was deleted")
		return nil, false
	}
	return instance, true
}
