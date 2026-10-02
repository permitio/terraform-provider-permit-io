package mockpermit

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

const setRulesPath = "/v2/facts/" + ProjectID + "/" + EnvironmentID + "/set_rules"

// newSetsForRules returns a mock with a document resource with read (ID 2) and
// write (ID 3) actions, a folder resource (ID 4), the reviewers user set (ID 5),
// and the drafts resource set on documents (ID 6).
func newSetsForRules(t *testing.T, tb testing.TB) *Server {
	t.Helper()
	m := New(tb, Resources, ConditionSets, ConditionSetRules)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "document", "name": "Document",
		"actions": {"read": {"name": "Read"}, "write": {"name": "Write"}}}`, http.StatusOK)
	send(t, m, http.MethodPost, resourcesPath, `{"key": "folder", "name": "Folder",
		"actions": {}}`, http.StatusOK)
	send(t, m, http.MethodPost, conditionSetsPath, `{"key": "reviewers", "name": "Reviewers"}`,
		http.StatusOK)
	send(t, m, http.MethodPost, conditionSetsPath, `{"key": "drafts", "name": "Drafts",
		"type": "resourceset", "resource_id": "document"}`, http.StatusOK)
	return m
}

// sendList makes a request to the mock, fails the test unless the status is 200,
// and returns the decoded JSON list in the response body.
func sendList(t *testing.T, m *Server, method, path, body string) []any {
	t.Helper()
	status, got := call(t, m, method, path, bearer, body)
	if status != http.StatusOK {
		t.Fatalf("%s %s: status %d, want 200; body %s", method, path, status, got)
	}
	var decoded []any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("%s %s: body %q is not a JSON list: %v", method, path, got, err)
	}
	return decoded
}

// ruleQuery returns the set rules path with the filters in pairs as its query.
func ruleQuery(pairs ...string) string {
	query := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		query.Set(pairs[i], pairs[i+1])
	}
	return setRulesPath + "?" + query.Encode()
}

func TestSetRuleState(t *testing.T) {
	m := newSetsForRules(t, t)
	const readRule = `{"user_set": "reviewers", "permission": "document:read",
		"resource_set": "drafts", "is_role": false, "is_resource": false}`

	created := sendList(t, m, http.MethodPost, setRulesPath, readRule)

	wantRule := `{
		"id": "` + ObjectID(7) + `", "key": "reviewers,document:read,drafts",
		"user_set": "reviewers", "permission": "document:read", "resource_set": "drafts",
		"organization_id": "` + OrganizationID + `", "project_id": "` + ProjectID + `",
		"environment_id": "` + EnvironmentID + `",
		"created_at": "` + timestamp + `", "updated_at": "` + timestamp + `"
	}`
	if len(created) != 1 {
		t.Fatalf("POST %s returned %v, want one rule", setRulesPath, created)
	}
	rule, _ := created[0].(map[string]any)
	wantFields(t, rule, wantRule)
	byActionID := sendList(t, m, http.MethodPost, setRulesPath, `{"user_set": "reviewers",
		"permission": "`+ObjectID(3)+`", "resource_set": "drafts"}`)
	if len(byActionID) != 1 {
		t.Fatalf("POST by action ID returned %v, want one rule", byActionID)
	}
	writeRule, _ := byActionID[0].(map[string]any)
	wantFields(t, writeRule, `{"permission": "document:write"}`)
	wantStoredKeys(t, m, "condition_sets/drafts", "condition_sets/reviewers", "resources/document",
		"resources/folder", "set_rules/reviewers,document:read,drafts",
		"set_rules/reviewers,document:write,drafts")

	filters := []struct {
		name  string
		query []string
		want  []any
	}{
		{name: "no filter", want: []any{rule, writeRule}},
		{
			name: "every filter, action key",
			query: []string{
				"user_set", "reviewers", "permission", "read", "resource_set", "drafts",
			},
			want: []any{rule},
		},
		{name: "action ID", query: []string{"permission", ObjectID(3)}, want: []any{writeRule}},
		{name: "resource:action", query: []string{"permission", "document:read"}, want: []any{}},
		{name: "another user set", query: []string{"user_set", "admins"}, want: []any{}},
		{name: "another resource set", query: []string{"resource_set", "folders"}, want: []any{}},
	}
	for _, filter := range filters {
		got := sendList(t, m, http.MethodGet, ruleQuery(filter.query...), "")
		if !reflect.DeepEqual(got, filter.want) {
			t.Errorf("%s: GET %s = %v, want %v", filter.name, ruleQuery(filter.query...), got,
				filter.want)
		}
	}

	send(t, m, http.MethodDelete, setRulesPath, readRule, http.StatusNoContent)

	wantStoredKeys(t, m, "condition_sets/drafts", "condition_sets/reviewers", "resources/document",
		"resources/folder", "set_rules/reviewers,document:write,drafts")
	send(t, m, http.MethodDelete, setRulesPath, readRule, http.StatusNoContent)
	send(t, m, http.MethodDelete, setRulesPath, `{"user_set": "reviewers",
		"permission": "`+ObjectID(3)+`", "resource_set": "drafts"}`, http.StatusNoContent)
	wantStoredKeys(t, m, "condition_sets/drafts", "condition_sets/reviewers", "resources/document",
		"resources/folder")
}

func TestSetRuleRequestErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name: "no permission", body: `{"user_set": "reviewers", "resource_set": "drafts"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "unknown field", body: `{"user_set": "reviewers", "permission": "document:read",
				"resource_set": "drafts", "tenant": "acme"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "unknown user set", body: `{"user_set": "admins", "permission": "document:read",
				"resource_set": "drafts"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "unknown resource set", body: `{"user_set": "reviewers",
				"permission": "document:read", "resource_set": "published"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "sets swapped", body: `{"user_set": "drafts", "permission": "document:read",
				"resource_set": "reviewers"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "action the resource does not have", body: `{"user_set": "reviewers",
				"permission": "document:delete", "resource_set": "drafts"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "action of another resource", body: `{"user_set": "reviewers",
				"permission": "folder:read", "resource_set": "drafts"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "unknown action ID", body: `{"user_set": "reviewers",
				"permission": "` + ObjectID(99) + `", "resource_set": "drafts"}`,
			wantStatus: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, method := range []string{http.MethodPost, http.MethodDelete} {
				m := newSetsForRules(t, t)

				send(t, m, method, setRulesPath, tt.body, tt.wantStatus)

				wantStoredKeys(t, m, "condition_sets/drafts", "condition_sets/reviewers",
					"resources/document", "resources/folder")
			}
		})
	}
}

// TestSetRuleUnconfirmedBehaviour checks that the fake fails the test, rather than
// guess, on a set rule request whose effect on the API is unconfirmed.
func TestSetRuleUnconfirmedBehaviour(t *testing.T) {
	const readRule = `{"user_set": "reviewers", "permission": "document:read",
		"resource_set": "drafts"}`
	tests := []struct {
		name      string
		method    string
		path      string
		body      string
		wantError string
	}{
		{
			name: "permission already granted", method: http.MethodPost, body: readRule,
			wantError: "already granted",
		},
		{
			name: "is_role set", method: http.MethodPost, wantError: "sets is_role",
			body: `{"user_set": "reviewers", "permission": "document:write",
				"resource_set": "drafts", "is_role": true}`,
		},
		{
			name: "is_resource set", method: http.MethodDelete, wantError: "sets is_resource",
			body: `{"user_set": "reviewers", "permission": "document:read",
				"resource_set": "drafts", "is_resource": true}`,
		},
		{
			name: "user set by ID", method: http.MethodPost,
			wantError: "names the user set " + ObjectID(5) + " by ID",
			body: `{"user_set": "` + ObjectID(5) + `", "permission": "document:write",
				"resource_set": "drafts"}`,
		},
		{
			name: "resource set by ID", method: http.MethodDelete,
			wantError: "names the resource set " + ObjectID(6) + " by ID",
			body: `{"user_set": "reviewers", "permission": "document:read",
				"resource_set": "` + ObjectID(6) + `"}`,
		},
		{
			name: "list by user set ID", method: http.MethodGet,
			path: ruleQuery("user_set", ObjectID(5)), wantError: "names the user_set by ID",
		},
		{
			name: "list by resource set ID", method: http.MethodGet,
			path: ruleQuery("resource_set", ObjectID(6)), wantError: "names the resource_set by ID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := newSetsForRules(t, rec)
			sendList(t, m, http.MethodPost, setRulesPath, readRule)
			path := tt.path
			if path == "" {
				path = setRulesPath
			}

			send(t, m, tt.method, path, tt.body, http.StatusNotImplemented)

			wantUnconfirmed(t, rec, tt.wantError)
			wantStoredKeys(t, m, "condition_sets/drafts", "condition_sets/reviewers",
				"resources/document", "resources/folder",
				"set_rules/reviewers,document:read,drafts")
		})
	}
}

// TestSetRuleOfADeletedParent checks that the fake fails the test, rather than
// guess or serve a stale rule, on a request that names a resource set whose
// resource was deleted, and on a list that would return a rule whose user set or
// resource set, or the resource set's resource, was deleted. The rule stays stored.
func TestSetRuleOfADeletedParent(t *testing.T) {
	const (
		readRule = `{"user_set": "reviewers", "permission": "document:read",
			"resource_set": "drafts"}`
		deletedResource = "a resource set whose resource was deleted"
		orphaned        = "the rule reviewers,document:read,drafts matches the query, but its " +
			"user set or resource set, or the resource set's resource, was deleted"
	)
	tests := []struct {
		name        string
		deleted     string
		method      string
		path        string
		body        string
		wantError   string
		unconfirmed bool
	}{
		{
			name: "assign on a resource set of a deleted resource", deleted: "resources/document",
			method: http.MethodPost, path: setRulesPath,
			body: `{"user_set": "reviewers", "permission": "document:write",
				"resource_set": "drafts"}`,
			wantError: "the body names " + deletedResource, unconfirmed: true,
		},
		{
			name: "unassign on a resource set of a deleted resource", deleted: "resources/document",
			method: http.MethodDelete, path: setRulesPath, body: readRule,
			wantError: "the body names " + deletedResource, unconfirmed: true,
		},
		{
			name: "list by a resource set of a deleted resource", deleted: "resources/document",
			method: http.MethodGet, path: ruleQuery("resource_set", "drafts"),
			wantError: "the query names " + deletedResource, unconfirmed: true,
		},
		{
			name: "list a rule on a deleted resource", deleted: "resources/document",
			method: http.MethodGet, path: ruleQuery("user_set", "reviewers"), wantError: orphaned,
		},
		{
			name: "list a rule of a deleted user set", deleted: "condition_sets/reviewers",
			method: http.MethodGet, path: ruleQuery(), wantError: orphaned,
		},
		{
			name: "list a rule on a deleted resource set", deleted: "condition_sets/drafts",
			method: http.MethodGet, path: ruleQuery("permission", "read"), wantError: orphaned,
		},
	}
	deletePaths := map[string]string{
		"resources/document":       resourcesPath + "/document",
		"condition_sets/reviewers": conditionSetsPath + "/reviewers",
		"condition_sets/drafts":    conditionSetsPath + "/drafts",
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := newSetsForRules(t, rec)
			sendList(t, m, http.MethodPost, setRulesPath, readRule)
			send(t, m, http.MethodDelete, deletePaths[tt.deleted], "", http.StatusNoContent)
			var wantStored []string
			for _, key := range []string{
				"condition_sets/drafts", "condition_sets/reviewers", "resources/document",
				"resources/folder", "set_rules/reviewers,document:read,drafts",
			} {
				if key != tt.deleted {
					wantStored = append(wantStored, key)
				}
			}

			send(t, m, tt.method, tt.path, tt.body, http.StatusNotImplemented)

			if tt.unconfirmed {
				wantUnconfirmed(t, rec, tt.wantError)
			} else if got := rec.take(); len(got) != 1 || !strings.Contains(got[0], tt.wantError) ||
				strings.Contains(got[0], "unconfirmed") {
				t.Errorf("test errors = %q, want one containing %q", got, tt.wantError)
			}
			wantStoredKeys(t, m, wantStored...)
		})
	}
}

// TestSetRuleListUnmodelledParameter checks that the fake fails the test on a list
// query parameter it does not model, whether the spec documents it or not.
func TestSetRuleListUnmodelledParameter(t *testing.T) {
	for _, name := range []string{"page", "per_page", "tenant"} {
		t.Run(name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := newSetsForRules(t, rec)

			send(t, m, http.MethodGet, ruleQuery(name, "2"), "", http.StatusNotImplemented)

			got := rec.take()
			want := "the fake does not model the query parameter " + name + ";"
			if len(got) != 1 || !strings.Contains(got[0], want) ||
				strings.Contains(got[0], "unconfirmed") {
				t.Errorf("test errors = %q, want one containing %q", got, want)
			}
		})
	}
}
