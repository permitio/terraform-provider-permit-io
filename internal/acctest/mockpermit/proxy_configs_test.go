package mockpermit

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
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
	send(t, m, http.MethodPatch, proxyConfigsPath+"/missing", `{"name": "x", "secret": "t"}`,
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
		{
			name: "update with an empty name", method: http.MethodPatch,
			body: `{"name": "", "secret": "token"}`,
		},
		{
			name: "update with an unknown field", method: http.MethodPatch,
			body: `{"key": "k2", "secret": "token"}`,
		},
		{name: "update without a secret", method: http.MethodPatch, body: `{"name": "N"}`},
		{
			name: "update to Basic keeping the Bearer secret", method: http.MethodPatch,
			body: `{"auth_mechanism": "Basic"}`,
		},
		{
			name: "update with a Headers secret and no auth mechanism", method: http.MethodPatch,
			body: `{"secret": {"x-api-key": "k"}}`,
		},
		{
			name:   "update to Basic with a secret that is not user:password",
			method: http.MethodPatch, body: `{"auth_mechanism": "Basic", "secret": "token"}`,
		},
		{
			name: "update with null mapping rules", method: http.MethodPatch,
			body: `{"secret": "token", "mapping_rules": null}`,
		},
		{
			name: "update with an invalid mapping rule", method: http.MethodPatch,
			body: `{"secret": "token", "mapping_rules": [{"url": "u"}]}`,
		},
		{
			name: "update with an empty url_type", method: http.MethodPatch,
			body: `{"secret": "token", "mapping_rules": [
				{"url": "u", "http_method": "get", "resource": "r", "url_type": ""}]}`,
		},
		{
			name: "update with should_delete not a boolean", method: http.MethodPatch,
			body: `{"secret": "token", "mapping_rules": [
				{"url": "u", "http_method": "get", "resource": "r", "should_delete": "yes"}]}`,
		},
		{
			name: "update removing a rule without its resource", method: http.MethodPatch,
			body: `{"secret": "token", "mapping_rules": [{"url": "https://api.example.com/invoices",
				"http_method": "get", "should_delete": true}]}`,
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

// TestProxyConfigPatchMergesMappingRules checks that a PATCH merges the mapping
// rules it sends into the stored ones by url and http_method, as the API does.
func TestProxyConfigPatchMergesMappingRules(t *testing.T) {
	const (
		storedRefunds = `{"url": "https://api.example.com/refunds", "http_method": "post",
			"resource": "refund", "headers": {}}`
		listInvoices = `{"url": "https://api.example.com/invoices", "http_method": "get",
			"resource": "invoice", "action": "list"}`
		storedListInvoices = `{"url": "https://api.example.com/invoices", "http_method": "get",
			"resource": "invoice", "action": "list", "headers": {}}`
		regexInvoices = `{"url": "https://api.example.com/invoices", "url_type": "regex",
			"http_method": "get", "resource": "invoice"}`
		storedRegexInvoices = `{"url": "https://api.example.com/invoices", "url_type": "regex",
			"http_method": "get", "resource": "invoice", "headers": {}}`
		fees = `{"url": "https://api.example.com/fees", "http_method": "get",
			"resource": "fee"}`
		storedFees = `{"url": "https://api.example.com/fees", "http_method": "get",
			"resource": "fee", "headers": {}}`
	)
	// shouldDelete returns rule, which must end with }, with should_delete set.
	shouldDelete := func(rule string, value bool) string {
		return fmt.Sprintf(`%s, "should_delete": %t}`, strings.TrimSuffix(rule, "}"), value)
	}
	tests := []struct {
		name string
		// rules are the rules the PATCH sends; "-" sends no mapping_rules.
		rules string
		want  string
	}{
		{
			name: "rule removed", rules: shouldDelete(refundsRule, true),
			want: invoicesRule,
		},
		{
			name: "no rules", rules: "",
			want: invoicesRule + `, ` + storedRefunds,
		},
		{
			name: "mapping_rules left out", rules: "-",
			want: invoicesRule + `, ` + storedRefunds,
		},
		{
			name: "new rule added at the end, the others kept", rules: fees,
			want: invoicesRule + `, ` + storedRefunds + `, ` + storedFees,
		},
		{
			name: "rule replaced where it is", rules: listInvoices,
			want: storedListInvoices + `, ` + storedRefunds,
		},
		{
			name:  "rules sent in another order keep their places",
			rules: refundsRule + `, ` + invoicesRule,
			want:  invoicesRule + `, ` + storedRefunds,
		},
		{
			name:  "rule with should_delete false stored without it",
			rules: shouldDelete(fees, false),
			want:  invoicesRule + `, ` + storedRefunds + `, ` + storedFees,
		},
		{
			name: "should_delete of a rule that is not stored", rules: shouldDelete(fees, true),
			want: invoicesRule + `, ` + storedRefunds,
		},
		{
			name:  "rule removed and sent again moves to the end",
			rules: shouldDelete(invoicesRule, true) + `, ` + invoicesRule,
			want:  storedRefunds + `, ` + invoicesRule,
		},
		{
			name:  "two sent rules with the same url and http_method, the last kept",
			rules: fees + `, ` + listInvoices + `, ` + fees + `, ` + invoicesRule,
			want:  invoicesRule + `, ` + storedRefunds + `, ` + storedFees,
		},
		{
			name: "url_type does not tell rules apart", rules: regexInvoices,
			want: storedRegexInvoices + `, ` + storedRefunds,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(t, ProxyConfigs)
			send(t, m, http.MethodPost, proxyConfigsPath, `{"key": "billing",
				"name": "Billing", "secret": "token",
				"mapping_rules": [`+invoicesRule+`, `+refundsRule+`]}`, http.StatusOK)
			body := `{"auth_mechanism": "Bearer", "secret": "token", "mapping_rules": [` +
				tt.rules + `]}`
			if tt.rules == "-" {
				body = `{"auth_mechanism": "Bearer", "secret": "token"}`
			}

			updated := send(t, m, http.MethodPatch, proxyConfigsPath+"/billing", body,
				http.StatusOK)

			want := `{"mapping_rules": [` + tt.want + `]}`
			wantFields(t, updated, want)
			wantFields(t, send(t, m, http.MethodGet, proxyConfigsPath+"/billing", "",
				http.StatusOK), want)
		})
	}
}

// TestProxyConfigRepeatedRules checks that a create stores two mapping rules with
// the same url and http_method, as the API does, and that any PATCH of the mapping
// rules, even an empty list, keeps only the last of the two.
func TestProxyConfigRepeatedRules(t *testing.T) {
	const (
		readInvoices = `{"url": "https://api.example.com/invoices", "http_method": "get",
			"resource": "invoice", "action": "read", "headers": {}}`
		listInvoices = `{"url": "https://api.example.com/invoices", "http_method": "get",
			"resource": "invoice", "action": "list", "headers": {}}`
	)
	m := New(t, ProxyConfigs)

	created := send(t, m, http.MethodPost, proxyConfigsPath, `{"key": "billing",
		"name": "Billing", "secret": "token",
		"mapping_rules": [`+readInvoices+`, `+listInvoices+`]}`, http.StatusOK)
	updated := send(t, m, http.MethodPatch, proxyConfigsPath+"/billing",
		`{"secret": "token", "mapping_rules": []}`, http.StatusOK)

	wantFields(t, created, `{"mapping_rules": [`+readInvoices+`, `+listInvoices+`]}`)
	wantFields(t, updated, `{"mapping_rules": [`+listInvoices+`]}`)
}
