package mockpermit

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"regexp"
	"slices"
)

const (
	proxyConfigCollection = "proxy_configs"
	proxyConfigsPattern   = "/v2/facts/{proj_id}/{env_id}/proxy_configs"
	proxyConfigPattern    = proxyConfigsPattern + "/{proxy_config_id}"
)

// ProxyConfigs serves the proxy config operations permitio_proxy_config calls:
// create, and get, update and delete by key or ID. It checks the secret against
// the auth mechanism as the spec describes it: a non-empty token for Bearer,
// user:password for Basic and an object of header values for Headers. It returns
// the secret as sent and the mapping rules in the order sent, with the spec's
// default of no headers; whether the API masks the secret or reorders the rules is
// unconfirmed. The spec says a PATCH overwrites the mapping rules it provides, but
// also gives each rule a should_delete flag, and its example names the rule to
// delete by its url and http_method. So the fake accepts only a PATCH that keeps
// the stored rules unchanged and in order and appends new ones, which both
// readings agree on, and fails the test on any other. Since the API may identify
// a rule by its url, url_type and http_method, the fake also fails the test on a
// create or PATCH with two rules that have the same three.
var ProxyConfigs = Routes{
	{"POST " + proxyConfigsPattern, "ProxyConfigs.Create", (*Server).createProxyConfig},
	{"GET " + proxyConfigPattern, "ProxyConfigs.Get", (*Server).getProxyConfig},
	{"PATCH " + proxyConfigPattern, "ProxyConfigs.Update", (*Server).updateProxyConfig},
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
	if repeatsARule(rules) {
		s.refuseUnconfirmed(w, r, ruleRepeated)
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

// updateProxyConfig overwrites each field the body provides, as the API documents
// for this PATCH, except that mapping rules may only be appended; see
// ProxyConfigs. A refused PATCH leaves the config as it was.
func (s *Server) updateProxyConfig(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeObject(w, r)
	if !ok || !s.knownFieldsOnly(w, body, "secret", "name", "mapping_rules", "auth_mechanism") {
		return
	}
	if _, given := body["name"]; given && str(body, "name") == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"name must be a non-empty string")
		return
	}
	_, patchesRules := body["mapping_rules"]
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
	updated["mapping_rules"] = config["mapping_rules"]
	if patchesRules {
		stored, _ := config["mapping_rules"].([]any)
		if !keepsAndAppends(stored, rules) || slices.ContainsFunc(rules, hasShouldDelete) {
			s.refuseUnconfirmed(w, r, "the body changes, removes or reorders a stored mapping "+
				"rule, or sets should_delete")
			return
		}
		if repeatsARule(rules) {
			s.refuseUnconfirmed(w, r, ruleRepeated)
			return
		}
		updated["mapping_rules"] = rules
	}
	s.collection(proxyConfigCollection)[str(config, "key")] = updated
	s.writeJSON(w, http.StatusOK, updated)
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
// each with a url, an http_method and a resource. It gives a rule without headers
// the spec's default of none. A missing or null list is empty.
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

// keepsAndAppends reports whether a list of mapping rules starts with the stored
// rules, unchanged and in order.
func keepsAndAppends(stored, rules []any) bool {
	if len(rules) < len(stored) {
		return false
	}
	for i, rule := range stored {
		if !reflect.DeepEqual(rule, rules[i]) {
			return false
		}
	}
	return true
}

// ruleRepeated is what a body does when repeatsARule reports true for its rules.
const ruleRepeated = "the body has two mapping rules with the same url, url_type and " +
	"http_method, which the API may take as one rule"

// ruleIdentity is what may identify a mapping rule to the API.
type ruleIdentity struct {
	url, urlType, httpMethod string
}

// repeatsARule reports whether two mapping rules from readMappingRules have the
// same url, url_type and http_method.
func repeatsARule(rules []any) bool {
	seen := map[ruleIdentity]bool{}
	for _, rule := range rules {
		fields, _ := rule.(map[string]any)
		identity := ruleIdentity{
			url: str(fields, "url"), urlType: str(fields, "url_type"),
			httpMethod: str(fields, "http_method"),
		}
		if seen[identity] {
			return true
		}
		seen[identity] = true
	}
	return false
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
