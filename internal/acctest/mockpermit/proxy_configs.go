package mockpermit

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
)

const (
	proxyConfigCollection = "proxy_configs"
	proxyConfigsPattern   = "/v2/facts/{proj_id}/{env_id}/proxy_configs"
	proxyConfigPattern    = proxyConfigsPattern + "/{proxy_config_id}"
	// proxyConfigsPackage is the provider package that sends the proxy config
	// update with net/http.
	proxyConfigsPackage = "proxy_configs"
)

// ProxyConfigs serves the proxy config operations permitio_proxy_config calls:
// create, get and delete by key or ID through the SDK, and the update by key or ID
// that the provider sends itself with net/http. It checks the secret against the
// auth mechanism as the API does: a non-empty token for Bearer, user:password for
// Basic and an object of header values for Headers. It returns the secret as
// stored, unmasked, as the API does, and a create stores the mapping rules in the
// order sent, two with the same url and http_method included. An update merges the
// rules it sends into the stored ones as the API does; see mergeMappingRules. The
// API rejects an update without a secret, since it takes a missing auth_mechanism
// as Bearer and requires a secret with it, and one whose mapping_rules is null.
var ProxyConfigs = Routes{
	{"POST " + proxyConfigsPattern, "ProxyConfigs.Create", (*Server).createProxyConfig},
	{"GET " + proxyConfigPattern, "ProxyConfigs.Get", (*Server).getProxyConfig},
	{
		"PATCH " + proxyConfigPattern, proxyConfigsPackage + ".update (HTTP)",
		(*Server).updateProxyConfig,
	},
	{"DELETE " + proxyConfigPattern, "ProxyConfigs.Delete", (*Server).deleteProxyConfig},
}

// basicAuth is the spec's pattern for a Basic secret.
var basicAuth = regexp.MustCompile(`^.+:.+$`)

// The spec's HTTP methods for a mapping rule.
var httpMethods = []string{"get", "post", "put", "patch", "delete", "head", "options"}

func (s *Server) createProxyConfig(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok || !s.knownFieldsOnly(w, body, "secret", "key", "name", "mapping_rules",
		"auth_mechanism") {
		return
	}
	key := str(body, "key")
	if key == "" || str(body, "name") == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"key and name are required")
		return
	}
	config := map[string]any{"auth_mechanism": "Bearer"}
	maps.Copy(config, body)
	rules, err := readMappingRules(body["mapping_rules"])
	if err == nil && slices.ContainsFunc(rules, hasShouldDelete) {
		err = errors.New("should_delete is only for an update")
	}
	err = errors.Join(err, checkSecret(config["auth_mechanism"], config["secret"]))
	if err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error())
		return
	}
	config["mapping_rules"] = rules
	s.mu.Lock()
	defer s.mu.Unlock()
	configs := s.collection(proxyConfigCollection)
	if _, exists := configs[key]; exists {
		s.writeError(w, http.StatusConflict, "DUPLICATE_ENTITY",
			"proxy config "+key+" already exists")
		return
	}
	maps.Copy(config, s.newObject())
	configs[key] = config
	s.writeJSON(w, http.StatusOK, config)
}

func (s *Server) getProxyConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	config, ok := s.find(proxyConfigCollection, r.PathValue("proxy_config_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "proxy config not found")
		return
	}
	s.writeJSON(w, http.StatusOK, config)
}

// updateProxyConfig overwrites each field the body provides, except the mapping
// rules, which it merges into the stored ones; see ProxyConfigs. It checks the body
// before it looks the config up, as the API does, and then the secret against the
// auth mechanism the config ends up with. A refused PATCH leaves the config as it
// was.
func (s *Server) updateProxyConfig(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok || !s.knownFieldsOnly(w, body, "secret", "name", "mapping_rules", "auth_mechanism") {
		return
	}
	if err := checkUpdate(body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error())
		return
	}
	rules, err := readMappingRules(body["mapping_rules"])
	if err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	config, ok := s.find(proxyConfigCollection, r.PathValue("proxy_config_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "proxy config not found")
		return
	}
	updated := maps.Clone(config)
	maps.Copy(updated, body)
	if err := checkSecret(updated["auth_mechanism"], updated["secret"]); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", err.Error())
		return
	}
	stored, _ := config["mapping_rules"].([]any)
	updated["mapping_rules"] = stored
	if _, patchesRules := body["mapping_rules"]; patchesRules {
		updated["mapping_rules"] = mergeMappingRules(stored, rules)
	}
	s.collection(proxyConfigCollection)[str(config, "key")] = updated
	s.writeJSON(w, http.StatusOK, updated)
}

// checkUpdate returns an error for a PATCH body the API rejects whatever config it
// names: an empty name, no secret or one that does not fit the auth mechanism, which
// is Bearer when the body has none, or null mapping rules.
func checkUpdate(body map[string]any) error {
	if _, given := body["name"]; given && str(body, "name") == "" {
		return errors.New("name must be a non-empty string")
	}
	if body["secret"] == nil {
		return errors.New("changing the auth_mechanism also requires changing the secret")
	}
	mechanism := body["auth_mechanism"]
	if mechanism == nil {
		mechanism = "Bearer"
	}
	if err := checkSecret(mechanism, body["secret"]); err != nil {
		return err
	}
	if rules, given := body["mapping_rules"]; given && rules == nil {
		return errors.New("mapping_rules must be a list")
	}
	return nil
}

func (s *Server) deleteProxyConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	config, ok := s.find(proxyConfigCollection, r.PathValue("proxy_config_id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "proxy config not found")
		return
	}
	delete(s.collection(proxyConfigCollection), str(config, "key"))
	w.WriteHeader(http.StatusNoContent)
}

// checkSecret returns an error unless the secret has the form the auth mechanism
// takes.
func checkSecret(mechanism, secret any) error {
	text, isText := secret.(string)
	switch mechanism {
	case "Bearer":
		if isText && text != "" {
			return nil
		}
		return errors.New("a Bearer secret must be a non-empty string")
	case "Basic":
		if isText && basicAuth.MatchString(text) {
			return nil
		}
		return errors.New("a Basic secret must be user:password")
	case "Headers":
		if headers, ok := secret.(map[string]any); ok && allStrings(headers) {
			return nil
		}
		return errors.New("a Headers secret must be an object of header values")
	}
	return fmt.Errorf("auth_mechanism %v is not Bearer, Basic or Headers", mechanism)
}

// readMappingRules reads the mapping rules of a request body: a list of objects,
// each with a url, an http_method and a resource, a url_type of regex or null if
// any, since the API rejects any other url_type, "" too, and a boolean
// should_delete if any. It gives a rule without headers the spec's default of none.
// A missing or null list is empty.
func readMappingRules(value any) ([]any, error) {
	rules := []any{}
	if value == nil {
		return rules, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, errors.New("mapping_rules is not a list")
	}
	for i, item := range list {
		rule, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("mapping_rules[%d] is not an object", i)
		}
		for field := range rule {
			if !slices.Contains([]string{"url", "url_type", "http_method", "resource", "headers",
				"action", "priority", "should_delete"}, field) {
				return nil, fmt.Errorf("mapping_rules[%d] has the unknown field %q", i, field)
			}
		}
		if str(rule, "url") == "" || str(rule, "resource") == "" ||
			!slices.Contains(httpMethods, str(rule, "http_method")) {
			return nil, fmt.Errorf("mapping_rules[%d] needs a url, a resource and one of the "+
				"http_method values %q", i, httpMethods)
		}
		if urlType, given := rule["url_type"]; given && urlType != nil && urlType != "regex" {
			return nil, fmt.Errorf("mapping_rules[%d].url_type must be regex or null", i)
		}
		if shouldDelete, given := rule["should_delete"]; given {
			if _, ok := shouldDelete.(bool); !ok {
				return nil, fmt.Errorf("mapping_rules[%d].should_delete is not a boolean", i)
			}
		}
		rule = maps.Clone(rule)
		if _, given := rule["headers"]; !given {
			rule["headers"] = map[string]any{}
		}
		if headers, ok := rule["headers"].(map[string]any); !ok || !allStrings(headers) {
			return nil, fmt.Errorf("mapping_rules[%d].headers is not an object of strings", i)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// mergeMappingRules returns the mapping rules a config holds after a PATCH that
// sends rules, merged into the stored ones as the API merges them. The API keys a
// rule by its url and http_method, so two stored rules with the same pair become
// one, the last, where the first was. Then, for each sent rule in order, it
// removes the rule with the same pair when should_delete is true, and otherwise
// replaces that rule with the sent one, without should_delete, where it is, or adds
// it at the end. So a rule the PATCH does not mention stays, and an empty list
// changes nothing.
func mergeMappingRules(stored, sent []any) []any {
	var order []string
	byKey := map[string]any{}
	put := func(rule any) {
		key := ruleKey(rule)
		if _, exists := byKey[key]; !exists {
			order = append(order, key)
		}
		byKey[key] = rule
	}
	for _, rule := range stored {
		put(rule)
	}
	for _, rule := range sent {
		fields, _ := rule.(map[string]any)
		if fields["should_delete"] == true {
			key := ruleKey(rule)
			delete(byKey, key)
			order = slices.DeleteFunc(order, func(k string) bool { return k == key })
			continue
		}
		fields = maps.Clone(fields)
		delete(fields, "should_delete")
		put(fields)
	}
	merged := []any{}
	for _, key := range order {
		merged = append(merged, byKey[key])
	}
	return merged
}

// ruleKey is what the API identifies a mapping rule by: its url and http_method.
func ruleKey(rule any) string {
	fields, _ := rule.(map[string]any)
	return str(fields, "url") + ":" + str(fields, "http_method")
}

// hasShouldDelete reports whether a mapping rule from readMappingRules has the
// should_delete flag, true or false.
func hasShouldDelete(rule any) bool {
	fields, _ := rule.(map[string]any)
	_, given := fields["should_delete"]
	return given
}

// allStrings reports whether every value of an object is a string.
func allStrings(object map[string]any) bool {
	for _, value := range object {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}
