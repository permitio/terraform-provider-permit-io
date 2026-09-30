package mockpermit

import (
	"net/http"
	"reflect"
	"testing"
)

const (
	proxyConfigsPath = "/v2/facts/" + ProjectID + "/" + EnvironmentID + "/proxy_configs"
	// invoicesRule and refundsRule are mapping rules as a request sends them.
	invoicesRule = `{"url": "https://api.example.com/invoices", "http_method": "get",
		"resource": "invoice", "action": "read", "priority": 1, "headers": {"x-a": "b"}}`
	refundsRule = `{"url": "https://api.example.com/refunds", "http_method": "post",
		"resource": "refund"}`
)

func TestProxyConfigState(t *testing.T) {
	m := New(t, ProxyConfigs)

	created := send(t, m, http.MethodPost, proxyConfigsPath, `{"key": "billing",
		"name": "Billing", "secret": "token", "mapping_rules": [`+invoicesRule+`]}`,
		http.StatusOK)

	wantConfig := `{
		"id": "` + ObjectID(1) + `", "key": "billing", "name": "Billing",
		"auth_mechanism": "Bearer", "secret": "token",
		"mapping_rules": [` + invoicesRule + `],
		"organization_id": "` + OrganizationID + `", "project_id": "` + ProjectID + `",
		"environment_id": "` + EnvironmentID + `",
		"created_at": "` + timestamp + `", "updated_at": "` + timestamp + `"
	}`
	wantFields(t, created, wantConfig)
	wantFields(t, send(t, m, http.MethodGet, proxyConfigsPath+"/billing", "", http.StatusOK),
		wantConfig)
	wantFields(t, send(t, m, http.MethodGet, proxyConfigsPath+"/"+ObjectID(1), "",
		http.StatusOK), wantConfig)
	send(t, m, http.MethodGet, proxyConfigsPath+"/missing", "", http.StatusNotFound)
	send(t, m, http.MethodPost, proxyConfigsPath, `{"key": "billing", "name": "Again",
		"secret": "token"}`, http.StatusConflict)
	wantFields(t, send(t, m, http.MethodPost, proxyConfigsPath, `{"key": "crm", "name": "CRM",
		"auth_mechanism": "Headers", "secret": {"x-api-key": "k"}}`, http.StatusOK),
		`{"id": "`+ObjectID(2)+`", "auth_mechanism": "Headers", "secret": {"x-api-key": "k"},
		"mapping_rules": []}`)
	wantFields(t, send(t, m, http.MethodPost, proxyConfigsPath, `{"key": "erp", "name": "ERP",
		"mapping_rules": [`+refundsRule+`], "auth_mechanism": "Basic", "secret": "u:p"}`,
		http.StatusOK), `{"mapping_rules": [{"url": "https://api.example.com/refunds",
		"http_method": "post", "resource": "refund", "headers": {}}]}`)
	wantStoredKeys(t, m, "proxy_configs/billing", "proxy_configs/crm", "proxy_configs/erp")

	updated := send(t, m, http.MethodPatch, proxyConfigsPath+"/billing", `{"name": "Billing 2",
		"auth_mechanism": "Basic", "secret": "user:password",
		"mapping_rules": [`+invoicesRule+`, `+refundsRule+`]}`, http.StatusOK)

	wantUpdated := `{"id": "` + ObjectID(1) + `", "key": "billing", "name": "Billing 2",
		"auth_mechanism": "Basic", "secret": "user:password",
		"mapping_rules": [` + invoicesRule + `, {"url": "https://api.example.com/refunds",
		"http_method": "post", "resource": "refund", "headers": {}}]}`
	wantFields(t, updated, wantUpdated)
	wantFields(t, send(t, m, http.MethodGet, proxyConfigsPath+"/billing", "", http.StatusOK),
		wantUpdated)
	wantFields(t, send(t, m, http.MethodPatch, proxyConfigsPath+"/"+ObjectID(1),
		`{"secret": "other:password"}`, http.StatusOK), `{"name": "Billing 2",
		"auth_mechanism": "Basic", "secret": "other:password"}`)
	send(t, m, http.MethodPatch, proxyConfigsPath+"/missing", `{"name": "x"}`,
		http.StatusNotFound)

	send(t, m, http.MethodDelete, proxyConfigsPath+"/billing", "", http.StatusNoContent)

	wantStoredKeys(t, m, "proxy_configs/crm", "proxy_configs/erp")
	send(t, m, http.MethodGet, proxyConfigsPath+"/billing", "", http.StatusNotFound)
	send(t, m, http.MethodDelete, proxyConfigsPath+"/billing", "", http.StatusNotFound)
	send(t, m, http.MethodDelete, proxyConfigsPath+"/"+ObjectID(2), "", http.StatusNoContent)
	wantStoredKeys(t, m, "proxy_configs/erp")
}

func TestProxyConfigRequestErrors(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   string
	}{
		{name: "create without key", method: http.MethodPost, body: `{"name": "N", "secret": "t"}`},
		{name: "create without name", method: http.MethodPost, body: `{"key": "k", "secret": "t"}`},
		{name: "create without secret", method: http.MethodPost, body: `{"key": "k", "name": "N"}`},
		{
			name: "create with an empty Bearer secret", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "auth_mechanism": "Bearer", "secret": ""}`,
		},
		{
			name: "create with a Basic secret without a password", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "auth_mechanism": "Basic", "secret": "user:"}`,
		},
		{
			name: "create with a Headers secret not an object", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "auth_mechanism": "Headers", "secret": ""}`,
		},
		{
			name: "create with a Headers secret of numbers", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "auth_mechanism": "Headers", "secret": {"a": 1}}`,
		},
		{
			name: "create with an unknown auth mechanism", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "auth_mechanism": "Digest", "secret": "t"}`,
		},
		{
			name: "create with an unknown field", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "secret": "t", "auth_secret": "t"}`,
		},
		{
			name: "create with mapping rules not a list", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "secret": "t", "mapping_rules": {}}`,
		},
		{
			name: "create with a mapping rule not an object", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "secret": "t", "mapping_rules": ["x"]}`,
		},
		{
			name: "create with a mapping rule without a url", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "secret": "t", "mapping_rules": [
				{"http_method": "get", "resource": "r"}]}`,
		},
		{
			name: "create with a mapping rule without a resource", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "secret": "t", "mapping_rules": [
				{"url": "u", "http_method": "get"}]}`,
		},
		{
			name: "create with an unknown http method", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "secret": "t", "mapping_rules": [
				{"url": "u", "http_method": "GET", "resource": "r"}]}`,
		},
		{
			name: "create with a mapping rule with an unknown field", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "secret": "t", "mapping_rules": [
				{"url": "u", "http_method": "get", "resource": "r", "regex": true}]}`,
		},
		{
			name: "create with an empty url_type", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "secret": "t", "mapping_rules": [
				{"url": "u", "http_method": "get", "resource": "r", "url_type": ""}]}`,
		},
		{
			name: "create with a url_type other than regex", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "secret": "t", "mapping_rules": [
				{"url": "u", "http_method": "get", "resource": "r", "url_type": "glob"}]}`,
		},
		{
			name: "create with mapping rule headers not strings", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "secret": "t", "mapping_rules": [
				{"url": "u", "http_method": "get", "resource": "r", "headers": {"a": 1}}]}`,
		},
		{
			name: "create with should_delete", method: http.MethodPost,
			body: `{"key": "k", "name": "N", "secret": "t", "mapping_rules": [
				{"url": "u", "http_method": "get", "resource": "r", "should_delete": false}]}`,
		},
		{name: "update with an empty name", method: http.MethodPatch, body: `{"name": ""}`},
		{name: "update with an unknown field", method: http.MethodPatch, body: `{"key": "k2"}`},
		{
			name: "update to Basic keeping the Bearer secret", method: http.MethodPatch,
			body: `{"auth_mechanism": "Basic"}`,
		},
		{
			name: "update with an invalid mapping rule", method: http.MethodPatch,
			body: `{"mapping_rules": [{"url": "u"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(t, ProxyConfigs)
			before := send(t, m, http.MethodPost, proxyConfigsPath, `{"key": "billing",
				"name": "Billing", "secret": "token", "mapping_rules": [`+invoicesRule+`]}`,
				http.StatusOK)
			path := proxyConfigsPath
			if tt.method == http.MethodPatch {
				path += "/billing"
			}

			send(t, m, tt.method, path, tt.body, http.StatusUnprocessableEntity)

			wantStoredKeys(t, m, "proxy_configs/billing")
			after := send(t, m, http.MethodGet, proxyConfigsPath+"/billing", "", http.StatusOK)
			if !reflect.DeepEqual(after, before) {
				t.Errorf("config after the rejected request = %v, want it unchanged: %v",
					after, before)
			}
		})
	}
}

// TestProxyConfigPatchUnconfirmedBehaviour checks that the fake fails the test,
// rather than guess, on a PATCH of mapping rules that the two readings of the spec
// disagree on.
func TestProxyConfigPatchUnconfirmedBehaviour(t *testing.T) {
	const (
		changedInvoicesRule = `{"url": "https://api.example.com/invoices",
			"http_method": "get", "resource": "invoice", "action": "list", "priority": 1,
			"headers": {"x-a": "b"}}`
		listInvoicesRule = `{"url": "https://api.example.com/invoices", "http_method": "get",
			"resource": "invoice", "action": "list"}`
		feesRule = `{"url": "https://api.example.com/fees", "http_method": "get",
			"resource": "fee"}`
		stored  = "changes, removes or reorders a stored mapping rule"
		repeats = "two mapping rules with the same url, url_type and http_method"
	)
	tests := []struct {
		name      string
		rules     string
		wantError string
	}{
		{name: "rule removed", rules: refundsRule, wantError: stored},
		{name: "all rules removed", rules: "", wantError: stored},
		{
			name: "rule changed", rules: changedInvoicesRule + `, ` + refundsRule,
			wantError: stored,
		},
		{
			name: "rules reordered", rules: refundsRule + `, ` + invoicesRule,
			wantError: stored,
		},
		{
			name: "should_delete on a new rule",
			rules: invoicesRule + `, ` + refundsRule + `, {"url": "https://api.example.com/fees",
				"http_method": "get", "resource": "fee", "should_delete": true}`,
			wantError: stored,
		},
		{
			name:      "new rule with the url and method of a stored rule",
			rules:     invoicesRule + `, ` + refundsRule + `, ` + listInvoicesRule,
			wantError: repeats,
		},
		{
			name:      "two new rules with the same url and method",
			rules:     invoicesRule + `, ` + refundsRule + `, ` + feesRule + `, ` + feesRule,
			wantError: repeats,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := New(rec, ProxyConfigs)
			before := send(t, m, http.MethodPost, proxyConfigsPath, `{"key": "billing",
				"name": "Billing", "secret": "token",
				"mapping_rules": [`+invoicesRule+`, `+refundsRule+`]}`, http.StatusOK)

			send(t, m, http.MethodPatch, proxyConfigsPath+"/billing",
				`{"name": "Billing 2", "mapping_rules": [`+tt.rules+`]}`, http.StatusNotImplemented)

			wantUnconfirmed(t, rec, tt.wantError)
			after := send(t, m, http.MethodGet, proxyConfigsPath+"/billing", "", http.StatusOK)
			if !reflect.DeepEqual(after, before) {
				t.Errorf("config after the refused PATCH = %v, want it unchanged: %v",
					after, before)
			}
		})
	}
}

// TestProxyConfigCreateWithRepeatedRule checks that the fake fails the test, rather
// than guess, on a create with two mapping rules that have the same url, url_type
// and http_method, and accepts rules that differ in any of the three.
func TestProxyConfigCreateWithRepeatedRule(t *testing.T) {
	const invoicesURL = `"url": "https://api.example.com/invoices", "resource": "invoice"`
	tests := []struct {
		name    string
		rules   string
		repeats bool
	}{
		{
			name: "same url and method",
			rules: `{` + invoicesURL + `, "http_method": "get", "action": "read"},
				{` + invoicesURL + `, "http_method": "get", "action": "list"}`,
			repeats: true,
		},
		{
			name: "same url, url_type and method",
			rules: `{` + invoicesURL + `, "http_method": "get", "url_type": "regex"},
				{` + invoicesURL + `, "http_method": "get", "url_type": "regex"}`,
			repeats: true,
		},
		{
			name: "same url, other method",
			rules: `{` + invoicesURL + `, "http_method": "get"},
				{` + invoicesURL + `, "http_method": "post"}`,
		},
		{
			name: "same url and method, other url_type",
			rules: `{` + invoicesURL + `, "http_method": "get"},
				{` + invoicesURL + `, "http_method": "get", "url_type": "regex"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &errorRecorder{TB: t}
			m := New(rec, ProxyConfigs)
			wantStatus := http.StatusOK
			if tt.repeats {
				wantStatus = http.StatusNotImplemented
			}

			send(t, m, http.MethodPost, proxyConfigsPath, `{"key": "billing", "name": "Billing",
				"secret": "token", "mapping_rules": [`+tt.rules+`]}`, wantStatus)

			if !tt.repeats {
				if got := rec.take(); len(got) != 0 {
					t.Errorf("test errors = %q, want none", got)
				}
				wantStoredKeys(t, m, "proxy_configs/billing")
				return
			}
			wantUnconfirmed(t, rec, "two mapping rules with the same url, url_type and http_method")
			wantStoredKeys(t, m)
		})
	}
}
