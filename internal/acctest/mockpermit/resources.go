package mockpermit

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
)

const (
	resourceCollection  = "resources"
	resourcesPattern    = "/v2/schema/{proj_id}/{env_id}/resources"
	resourcePattern     = resourcesPattern + "/{resource_id}"
	attributeCollection = "resource_attributes"
	attributesPattern   = resourcePattern + "/attributes"
	attributePattern    = attributesPattern + "/{attribute_id}"
)

// UserResourceID is the ID of __user, the built-in resource that holds user
// attributes. It exists in every environment, so the fake serves it as the parent
// of attributes without storing it.
const (
	UserResourceID  = "00000000-0000-4000-9000-000000000001"
	userResourceKey = "__user"
)

// blocks are a resource's actions or attribute definitions by key.
type blocks = map[string]map[string]any

// Resources serves the resource operations permitio_resource calls: create, and
// get, update and delete by key or ID. The fake keeps a resource's actions in the
// resource and its attributes as resource attributes, and returns both in the
// resource the way the API does.
var Resources = Routes{
	{"POST " + resourcesPattern, "Resources.Create", (*Server).createResource},
	{"GET " + resourcePattern, "Resources.Get", (*Server).getResource},
	{"PATCH " + resourcePattern, "Resources.Update", (*Server).updateResource},
	{"DELETE " + resourcePattern, "Resources.Delete", (*Server).deleteResource},
}

// ResourceAttributes serves the attribute operations permitio_user_attribute
// calls: create, and get, update and delete by key or ID. It serves them for any
// resource; permitio_user_attribute uses the built-in __user resource.
var ResourceAttributes = Routes{
	{"POST " + attributesPattern, "ResourceAttributes.Create", (*Server).createAttribute},
	{"GET " + attributePattern, "ResourceAttributes.Get", (*Server).getAttribute},
	{"PATCH " + attributePattern, "ResourceAttributes.Update", (*Server).updateAttribute},
	{"DELETE " + attributePattern, "ResourceAttributes.Delete", (*Server).deleteAttribute},
}

func (s *Server) createResource(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok {
		return
	}
	key := str(body, "key")
	actions, actionsErr := readBlocks("actions", body["actions"])
	attributes, attributesErr := readBlocks("attributes", body["attributes"])
	if err := errors.Join(actionsErr, attributesErr); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error())
		return
	}
	if key == "" || actions == nil {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"key and actions are required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	resources := s.collection(resourceCollection)
	if _, exists := resources[key]; exists {
		s.writeError(w, http.StatusConflict, "DUPLICATE_ENTITY", "resource "+key+" already exists")
		return
	}
	resource := s.newObject()
	maps.Copy(resource, body)
	delete(resource, "attributes")
	resource["actions"] = s.withIDs(nil, actions)
	resources[key] = resource
	s.storeAttributes(resource, s.withIDs(nil, attributes))
	s.writeJSON(w, http.StatusOK, s.resourceRead(resource))
}

func (s *Server) getResource(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resource, ok := s.find(resourceCollection, r.PathValue("resource_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
		return
	}
	s.writeJSON(w, http.StatusOK, s.resourceRead(resource))
}

// updateResource overwrites each field the body provides, as the API documents
// for this PATCH, keeping the IDs of the actions and attributes whose keys stay.
// Like the API, it keeps the actions or attributes when the body leaves them out,
// keeps the attributes when the body nulls them, answers a null actions with a
// 422, and deletes every attribute for an empty attributes object. The API also
// deletes the resource sets whose conditions name a deleted attribute; the fake
// keeps them.
func (s *Server) updateResource(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok {
		return
	}
	actionsValue, patchesActions := body["actions"]
	if patchesActions && actionsValue == nil {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"actions may not be null")
		return
	}
	actions, actionsErr := readBlocks("actions", actionsValue)
	attributesValue, patchesAttributes := body["attributes"]
	patchesAttributes = patchesAttributes && attributesValue != nil
	attributes, attributesErr := readBlocks("attributes", attributesValue)
	if err := errors.Join(actionsErr, attributesErr); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	resource, ok := s.find(resourceCollection, r.PathValue("resource_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
		return
	}
	storedActions, _ := resource["actions"].(blocks)
	storedAttributes := s.attributeBlocks(resource)
	var unconfirmed []string
	if patchesActions {
		unconfirmed = append(unconfirmed, leftOut("actions", storedActions, actions)...)
	}
	if patchesAttributes && len(attributes) > 0 {
		unconfirmed = append(unconfirmed, leftOut("attributes", storedAttributes, attributes)...)
	}
	if len(unconfirmed) > 0 {
		s.refuseUnconfirmed(w, r, fmt.Sprintf("the body leaves out %q", unconfirmed))
		return
	}
	maps.Copy(resource, body)
	resource["actions"] = storedActions
	if patchesActions {
		resource["actions"] = s.withIDs(storedActions, actions)
	}
	if patchesAttributes {
		s.deleteAttributes(resource)
		s.storeAttributes(resource, s.withIDs(storedAttributes, attributes))
	}
	delete(resource, "attributes")
	s.writeJSON(w, http.StatusOK, s.resourceRead(resource))
}

// deleteResource deletes a resource with its actions and attributes. The API also
// deletes the roles, relations and derivations on it; the fake keeps them, so a
// test that deletes a resource before them sees them in StoredKeys. It keeps the
// resource sets and instances on it too, and fails the test on a request for one;
// see ConditionSets and ResourceInstances.
func (s *Server) deleteResource(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resource, ok := s.find(resourceCollection, r.PathValue("resource_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
		return
	}
	s.deleteAttributes(resource)
	delete(s.collection(resourceCollection), str(resource, "key"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createAttribute(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok {
		return
	}
	key := str(body, "key")
	if key == "" || body["type"] == nil {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"key and type are required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	resource, ok := s.pathResource(w, r)
	if !ok {
		return
	}
	if _, _, exists := s.findChild(attributeCollection, str(resource, "key"), key); exists {
		s.writeError(w, http.StatusConflict, "DUPLICATE_ENTITY", "attribute "+key+" already exists")
		return
	}
	s.storeAttributes(resource, s.withIDs(nil, blocks{key: body}))
	_, attribute, _ := s.findChild(attributeCollection, str(resource, "key"), key)
	s.writeJSON(w, http.StatusOK, attribute)
}

func (s *Server) getAttribute(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, attribute, ok := s.pathAttribute(w, r); ok {
		s.writeJSON(w, http.StatusOK, attribute)
	}
}

func (s *Server) updateAttribute(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, attribute, ok := s.pathAttribute(w, r); ok {
		maps.Copy(attribute, body)
		s.writeJSON(w, http.StatusOK, attribute)
	}
}

func (s *Server) deleteAttribute(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if storedKey, _, ok := s.pathAttribute(w, r); ok {
		delete(s.collection(attributeCollection), storedKey)
		w.WriteHeader(http.StatusNoContent)
	}
}

// pathAttribute returns the attribute the request path names and the key it is
// stored under, or writes a 404 and returns false. The caller holds s.mu.
func (s *Server) pathAttribute(w http.ResponseWriter, r *http.Request) (
	string, map[string]any, bool,
) {
	resource, ok := s.pathResource(w, r)
	if !ok {
		return "", nil, false
	}
	storedKey, attribute, ok := s.findChild(attributeCollection, str(resource, "key"),
		r.PathValue("attribute_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "attribute not found")
	}
	return storedKey, attribute, ok
}

// pathResource returns the resource the request path names as the parent of the
// object it operates on, or writes a 404 and returns false. It knows the built-in
// __user resource. The caller holds s.mu.
func (s *Server) pathResource(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	return s.parentResource(w, r.PathValue("resource_id"))
}

// parentResource returns the resource with this key or ID, or writes a 404 and
// returns false. It knows the built-in __user resource. The caller holds s.mu.
func (s *Server) parentResource(w http.ResponseWriter, keyOrID string) (map[string]any, bool) {
	if keyOrID == userResourceKey || keyOrID == UserResourceID {
		return map[string]any{"key": userResourceKey, "id": UserResourceID}, true
	}
	resource, ok := s.find(resourceCollection, keyOrID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "resource "+keyOrID+" not found")
	}
	return resource, ok
}

// resourceRead returns a resource as the API returns it, with its attributes.
// The caller holds s.mu.
func (s *Server) resourceRead(resource map[string]any) map[string]any {
	read := maps.Clone(resource)
	read["attributes"] = s.attributeBlocks(resource)
	return read
}

// attributeBlocks returns a resource's attributes shaped like the attribute
// blocks in a resource. The caller holds s.mu.
func (s *Server) attributeBlocks(resource map[string]any) blocks {
	result := blocks{}
	for _, attribute := range s.collection(attributeCollection) {
		if attribute["resource_key"] != resource["key"] {
			continue
		}
		block := map[string]any{}
		for _, field := range []string{"id", "key", "type", "description"} {
			if value, ok := attribute[field]; ok {
				block[field] = value
			}
		}
		result[str(attribute, "key")] = block
	}
	return result
}

// storeAttributes stores attribute blocks, which already have IDs, as attributes
// of the resource. The caller holds s.mu.
func (s *Server) storeAttributes(resource map[string]any, definitions blocks) {
	attributes := s.collection(attributeCollection)
	for key, block := range definitions {
		attribute := map[string]any{
			"resource_id":     resource["id"],
			"resource_key":    resource["key"],
			"organization_id": OrganizationID,
			"project_id":      ProjectID,
			"environment_id":  EnvironmentID,
			"created_at":      timestamp,
			"updated_at":      timestamp,
			"built_in":        false,
		}
		maps.Copy(attribute, block)
		attributes[childKey(str(resource, "key"), key)] = attribute
	}
}

// deleteAttributes deletes every attribute of the resource. The caller holds s.mu.
func (s *Server) deleteAttributes(resource map[string]any) {
	ofResource := func(_ string, attribute map[string]any) bool {
		return attribute["resource_key"] == resource["key"]
	}
	maps.DeleteFunc(s.collection(attributeCollection), ofResource)
}

// readBlocks reads a resource's actions or attributes from a request body: an
// object of objects by key. It returns nil for a field that is missing or null.
func readBlocks(field string, value any) (blocks, error) {
	if value == nil {
		return nil, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", field)
	}
	result := blocks{}
	for key, blockValue := range object {
		block, ok := blockValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.%s is not an object", field, key)
		}
		result[key] = block
	}
	return result, nil
}

// leftOut lists what a PATCH that provides a resource's actions or attributes
// leaves out: a stored block, or a field of a stored block. The API deletes a
// block left out of a PATCH, together with the role permissions and resource sets
// that use it, which the fake does not model, and what it does with a field left
// out of a block is unconfirmed, so the fake refuses both rather than guess.
func leftOut(field string, stored, patch blocks) []string {
	var missing []string
	for key, block := range stored {
		patchBlock, ok := patch[key]
		if !ok {
			missing = append(missing, field+"."+key)
			continue
		}
		for name := range block {
			if _, ok := patchBlock[name]; !ok && name != "id" && name != "key" {
				missing = append(missing, field+"."+key+"."+name)
			}
		}
	}
	slices.Sort(missing)
	return missing
}

// withIDs returns copies of the blocks with a key and an ID in each: the ID of the
// stored block with the same key, or a new one. New IDs are given in key order, so
// they do not depend on map order. The caller holds s.mu.
func (s *Server) withIDs(stored, patch blocks) blocks {
	result := blocks{}
	for _, key := range slices.Sorted(maps.Keys(patch)) {
		block := maps.Clone(patch[key])
		block["key"] = key
		if old, ok := stored[key]; ok {
			block["id"] = old["id"]
		} else {
			block["id"] = s.newID()
		}
		result[key] = block
	}
	return result
}
