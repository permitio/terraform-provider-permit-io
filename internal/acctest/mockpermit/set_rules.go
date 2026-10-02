package mockpermit

import (
	"maps"
	"net/http"
	"slices"
	"strings"
)

const (
	setRuleCollection = "set_rules"
	setRulesPattern   = "/v2/facts/{proj_id}/{env_id}/set_rules"
)

// ConditionSetRules serves the set rule operations permitio_condition_set_rule
// calls: assign a permission to a user set on a resource set, list the rules that
// match a filter, and unassign. The sets are named by key, and the permission as
// resource:action or by the action's ID; it must be an action of the resource set's
// resource. The list's permission filter matches the action's key or ID but not
// resource:action, as the API does. Unassigning a permission that is not granted
// is skipped, as the API documents. Assigning one that is already granted is
// skipped too, but what the API returns then is unconfirmed, so the fake fails the
// test on it. The API deletes a set's rules with the set, and may delete the rules
// on a resource set with its resource; the fake keeps such rules so that
// StoredKeys lists them, and fails the test on a request that names such a
// resource set or a list that would return such a rule.
var ConditionSetRules = Routes{
	{
		"POST " + setRulesPattern, "ConditionSets.AssignSetPermissions",
		(*Server).assignSetPermission,
	},
	{
		"GET " + setRulesPattern, "ConditionSets.ListSetPermissions",
		(*Server).listSetPermissions,
	},
	{
		"DELETE " + setRulesPattern, "ConditionSets.UnassignSetPermissions",
		(*Server).unassignSetPermission,
	},
}

// setRule is what a set rule body names: the user set, the resource set, and the
// resource and action of the permission, all by key.
type setRule struct {
	userSet, resourceSet, resource, action string
}

// storedKey is the key the fake stores the rule under, which it also returns as
// the rule's key.
func (rule setRule) storedKey() string {
	return strings.Join([]string{rule.userSet, rule.permission(), rule.resourceSet}, ",")
}

// permission is the rule's permission written resource:action.
func (rule setRule) permission() string {
	return rule.resource + ":" + rule.action
}

func (s *Server) assignSetPermission(w http.ResponseWriter, r *http.Request) {
	s.withSetRule(w, r, func(rule setRule) {
		rules := s.collection(setRuleCollection)
		if _, exists := rules[rule.storedKey()]; exists {
			s.refuseUnconfirmed(w, r, "the body assigns a permission that is already granted")
			return
		}
		stored := s.newObject()
		stored["key"] = rule.storedKey()
		stored["user_set"] = rule.userSet
		stored["permission"] = rule.permission()
		stored["resource_set"] = rule.resourceSet
		rules[rule.storedKey()] = stored
		s.writeJSON(w, http.StatusOK, []any{stored})
	})
}

func (s *Server) unassignSetPermission(w http.ResponseWriter, r *http.Request) {
	s.withSetRule(w, r, func(rule setRule) {
		delete(s.collection(setRuleCollection), rule.storedKey())
		w.WriteHeader(http.StatusNoContent)
	})
}

// listSetPermissions returns the rules that match every filter in the query, in
// a fixed order. It fails the test on any other query parameter, including the
// spec's page and per_page, which the fake does not model.
func (s *Server) listSetPermissions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	for name := range query {
		if !slices.Contains([]string{"user_set", "permission", "resource_set"}, name) {
			s.t.Errorf("mockpermit: %s %s: the fake does not model the query parameter %s; "+
				"add it to the mock if the provider should send it", r.Method, r.URL.Path, name)
			s.writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
				"not modelled in mockpermit")
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range []string{"user_set", "resource_set"} {
		if !query.Has(name) {
			continue
		}
		set, ok := s.find(conditionSetCollection, query.Get(name))
		switch {
		case ok && set["key"] != query.Get(name):
			s.refuseUnconfirmed(w, r, "the query names the "+name+" by ID")
			return
		case ok && s.resourceDeleted(set):
			s.refuseUnconfirmed(w, r, "the query names a resource set whose resource was deleted")
			return
		}
	}
	rules := s.collection(setRuleCollection)
	matching := []any{}
	for _, storedKey := range slices.Sorted(maps.Keys(rules)) {
		rule := rules[storedKey]
		if query.Has("user_set") && rule["user_set"] != query.Get("user_set") ||
			query.Has("resource_set") && rule["resource_set"] != query.Get("resource_set") ||
			query.Has("permission") && !s.permissionFilterMatches(rule, query.Get("permission")) {
			continue
		}
		if s.orphanedRule(rule) {
			s.t.Errorf("mockpermit: %s %s: the rule %s matches the query, but its user set or "+
				"resource set, or the resource set's resource, was deleted; the fake keeps such "+
				"rules only so that StoredKeys lists them", r.Method, r.URL.Path, storedKey)
			s.writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
				"not modelled in mockpermit")
			return
		}
		matching = append(matching, rule)
	}
	s.writeJSON(w, http.StatusOK, matching)
}

// orphanedRule reports whether a stored rule's user set or resource set, or the
// resource set's resource, was deleted after the rule was assigned. The caller
// holds s.mu.
func (s *Server) orphanedRule(rule map[string]any) bool {
	sets := s.collection(conditionSetCollection)
	_, userSetExists := sets[str(rule, "user_set")]
	resourceSet, resourceSetExists := sets[str(rule, "resource_set")]
	return !userSetExists || !resourceSetExists || s.resourceDeleted(resourceSet)
}

// permissionFilterMatches reports whether the list's permission filter matches a
// stored rule: the filter must be the key or the ID of the rule's action. The
// caller holds s.mu.
func (s *Server) permissionFilterMatches(rule map[string]any, filter string) bool {
	resourceKey, actionKey, _ := strings.Cut(str(rule, "permission"), ":")
	if filter == actionKey {
		return true
	}
	resource, ok := s.collection(resourceCollection)[resourceKey]
	if !ok {
		return false
	}
	actions, _ := resource["actions"].(blocks)
	return filter != "" && actions[actionKey]["id"] == filter
}

// withSetRule reads the rule a set rule body describes and calls do with it,
// holding s.mu. It writes a 422 or 404 instead when the body is incomplete or
// names a set or an action that does not exist.
func (s *Server) withSetRule(w http.ResponseWriter, r *http.Request, do func(rule setRule)) {
	body, ok := s.decodeObject(w, r)
	if !ok || !s.knownFieldsOnly(w, body, "user_set", "permission", "resource_set", "is_role",
		"is_resource") {
		return
	}
	userSetKey, permission := str(body, "user_set"), str(body, "permission")
	resourceSetKey := str(body, "resource_set")
	if userSetKey == "" || permission == "" || resourceSetKey == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"user_set, permission and resource_set are required")
		return
	}
	for _, flag := range []string{"is_role", "is_resource"} {
		if value, given := body[flag]; given && value != false {
			s.refuseUnconfirmed(w, r, "the body sets "+flag)
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	userSet, ok := s.findKey(w, r, conditionSetCollection, userSetKey, "user set")
	if !ok {
		return
	}
	resourceSet, ok := s.findKey(w, r, conditionSetCollection, resourceSetKey, "resource set")
	if !ok {
		return
	}
	if userSet["type"] != userSetType || resourceSet["type"] != resourceSetType {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"user_set must be a user set and resource_set a resource set")
		return
	}
	if s.resourceDeleted(resourceSet) {
		s.refuseUnconfirmed(w, r, "the body names a resource set whose resource was deleted")
		return
	}
	resourceKey, actionKey, ok := s.resourceSetAction(resourceSet, permission)
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND",
			"permission "+permission+" is not an action of the resource set's resource")
		return
	}
	do(setRule{
		userSet: userSetKey, resourceSet: resourceSetKey, resource: resourceKey, action: actionKey,
	})
}

// resourceSetAction returns the keys of the resource and action that a permission
// names on a resource set's resource, where the permission is resource:action or
// the action's ID. The caller holds s.mu.
func (s *Server) resourceSetAction(resourceSet map[string]any, permission string) (
	string, string, bool,
) {
	resource, ok := s.find(resourceCollection, str(resourceSet, "resource_id"))
	if !ok {
		return "", "", false
	}
	resourceKey := str(resource, "key")
	actions, _ := resource["actions"].(blocks)
	if permissionResource, actionKey, isName := strings.Cut(permission, ":"); isName {
		_, exists := actions[actionKey]
		return resourceKey, actionKey, exists && permissionResource == resourceKey
	}
	for actionKey, action := range actions {
		if action["id"] == permission {
			return resourceKey, actionKey, true
		}
	}
	return "", "", false
}
