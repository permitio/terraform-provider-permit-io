package proxy_configs_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// testRule is a mapping rule of the billing proxy config, on the invoice resource.
type testRule struct {
	url, method string
}

var (
	invoicesGet  = testRule{"https://billing.example.com/v1/invoices", "get"}
	invoicesPost = testRule{"https://billing.example.com/v1/invoices", "post"}
	refundsGet   = testRule{"https://billing.example.com/v1/refunds", "get"}
	refundsPost  = testRule{"https://billing.example.com/v1/refunds", "post"}
	creditsGet   = testRule{"https://billing.example.com/v1/credits", "get"}
	creditsV2Get = testRule{"https://billing.example.com/v2/credits", "get"}
)

// hcl returns the rule as an element of the mapping_rules list in a configuration.
func (r testRule) hcl() string {
	return fmt.Sprintf(`
    {
      url         = %q
      http_method = %q
      resource    = "invoice"
    },`, r.url, r.method)
}

// fields returns the rule's JSON fields as a request sends them.
func (r testRule) fields() string {
	return fmt.Sprintf(`"url": %q, "http_method": %q, "resource": "invoice"`, r.url, r.method)
}

// TestProxyConfigMappingRuleChanges changes the mapping rules of a proxy config in
// every way a user can: reorder them, insert one in the middle, remove one, and
// change a rule's url and its http_method, which the API takes as removing the
// rule and adding another, and last remove them all. The API merges the rules of an
// update with the stored ones by url and http_method: it keeps a stored rule the
// update does not mention, keeps a rule the update changes in its place, and adds
// a new one at the end. So the provider sends should_delete for each rule of the
// prior state and then the planned rules, with the secret and auth mechanism the
// API needs on every update, and the API ends up with exactly the planned rules in
// the planned order. Each step must leave nothing to plan.
func TestProxyConfigMappingRuleChanges(t *testing.T) {
	m := mockpermit.New(t, mockpermit.ProxyConfigs)
	steps := []struct {
		name    string
		planned []testRule
	}{
		{name: "create", planned: []testRule{invoicesGet, invoicesPost, refundsGet}},
		{name: "reorder", planned: []testRule{refundsGet, invoicesGet, invoicesPost}},
		{
			name:    "insert in the middle",
			planned: []testRule{refundsGet, creditsGet, invoicesGet, invoicesPost},
		},
		{name: "remove", planned: []testRule{refundsGet, creditsGet, invoicesPost}},
		{name: "change the url", planned: []testRule{refundsGet, creditsV2Get, invoicesPost}},
		{
			name:    "change the http_method",
			planned: []testRule{refundsPost, creditsV2Get, invoicesPost},
		},
		{name: "remove all", planned: []testRule{}},
	}

	var testSteps []resource.TestStep
	for i, step := range steps {
		var rules strings.Builder
		for _, rule := range step.planned {
			rules.WriteString(rule.hcl())
		}
		checks := []resource.TestCheckFunc{
			checkStateRules(step.name, step.planned),
			checkStoredRules(m, step.name, step.planned),
		}
		if i > 0 {
			checks = append(checks,
				checkUpdate(m, step.name, i, steps[i-1].planned, step.planned))
		}
		testSteps = append(testSteps, resource.TestStep{
			Config: proxyConfigWithRules(rules.String()),
			Check:  resource.ComposeAggregateTestCheckFunc(checks...),
		})
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps:                    testSteps,
	})
}

// TestProxyConfigRemovesRulesAddedOutsideTerraform checks that a plan removes a
// mapping rule added to the proxy config outside Terraform. The refresh must keep
// the state's order for the rules the state has, even when the API holds another
// order, and add the new rule at the end, so the plan shows it and the update
// removes it and leaves the API with the configured rules in order.
func TestProxyConfigRemovesRulesAddedOutsideTerraform(t *testing.T) {
	m := mockpermit.New(t, mockpermit.ProxyConfigs)
	config := proxyConfigWithRules(invoicesGet.hcl() + invoicesPost.hcl())
	configured := []testRule{invoicesGet, invoicesPost}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  checkStoredRules(m, "create", configured),
			},
			{
				// Moves invoicesGet to the end and adds refundsGet, as someone
				// editing the proxy config in Permit could.
				PreConfig: func() {
					patchStoredRules(t, m, `[{`+invoicesGet.fields()+`, "should_delete": true},
						{`+invoicesGet.fields()+`}, {`+refundsGet.fields()+`}]`)
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStateRules("re-apply", configured),
					checkStoredRules(m, "re-apply", configured),
					// The second update is the provider's; the first is PreConfig's.
					checkUpdate(m, "re-apply", 2,
						[]testRule{invoicesGet, invoicesPost, refundsGet}, configured),
				),
			},
		},
	})
}

// TestProxyConfigRejectsDuplicateMappingRules checks that a plan fails on two
// mapping rules with the same url and http_method, before the provider sends
// anything. The API identifies a rule by the two, whatever its url_type: a create
// would store both, and an update would keep one.
func TestProxyConfigRejectsDuplicateMappingRules(t *testing.T) {
	m := mockpermit.New(t, mockpermit.ProxyConfigs)
	tests := map[string]string{
		"same url and http_method": invoicesGet.hcl() + refundsGet.hcl() + `
    {
      url         = "https://billing.example.com/v1/invoices"
      http_method = "get"
      resource    = "invoice"
      action      = "list"
    },`,
		"same url and http_method, other url_type": invoicesGet.hcl() + `
    {
      url         = "https://billing.example.com/v1/invoices"
      url_type    = "regex"
      http_method = "get"
      resource    = "invoice"
    },`,
	}
	for name, rules := range tests {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: providerFactories,
				Steps: []resource.TestStep{
					{
						Config: proxyConfigWithRules(rules),
						ExpectError: regexp.MustCompile(`Duplicate mapping rule(.|\n)*has\s+the\s+same` +
							`\s+url\s+"https://billing.example.com/v1/invoices"\s+and\s+http_method` +
							`\s+"get"\s+as\s+mapping_rules\[0\]`),
					},
				},
			})
		})
	}

	if requests := m.Requests(http.MethodPost, proxyConfigsPath); len(requests) != 0 {
		t.Errorf("the provider sent %d creates, want none", len(requests))
	}
}

// checkStateRules checks that the state holds the rules in this order.
func checkStateRules(step string, want []testRule) resource.TestCheckFunc {
	const address = "permitio_proxy_config.billing"
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(address, "mapping_rules.#", fmt.Sprint(len(want))),
	}
	for i, rule := range want {
		checks = append(checks,
			resource.TestCheckResourceAttr(address, fmt.Sprintf("mapping_rules.%d.url", i), rule.url),
			resource.TestCheckResourceAttr(address, fmt.Sprintf("mapping_rules.%d.http_method", i),
				rule.method))
	}
	return func(state *terraform.State) error {
		if err := resource.ComposeAggregateTestCheckFunc(checks...)(state); err != nil {
			return fmt.Errorf("%s: state: %w", step, err)
		}
		return nil
	}
}

// checkStoredRules checks that the mock holds exactly these rules, in this order.
func checkStoredRules(m *mockpermit.Server, step string, want []testRule) resource.TestCheckFunc {
	return func(*terraform.State) error {
		request, err := http.NewRequest(http.MethodGet, m.URL+proxyConfigsPath+"/billing", nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+mockpermit.APIKey)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		var got struct {
			MappingRules []map[string]any `json:"mapping_rules"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			return fmt.Errorf("%s: decoding the stored proxy config %s: %w", step, body, err)
		}
		wantRules := []map[string]any{}
		for _, rule := range want {
			wantRules = append(wantRules, map[string]any{
				"url": rule.url, "http_method": rule.method, "resource": "invoice",
				"headers": map[string]any{},
			})
		}
		if got.MappingRules == nil || !reflect.DeepEqual(got.MappingRules, wantRules) {
			return fmt.Errorf("%s: the mock holds the mapping rules %v, want %v", step,
				got.MappingRules, wantRules)
		}
		return nil
	}
}

// checkUpdate checks that the provider has sent n updates, the last with the name,
// the auth mechanism and the secret, should_delete for each prior rule, and then
// the planned rules.
func checkUpdate(m *mockpermit.Server, step string, n int, prior, planned []testRule,
) resource.TestCheckFunc {
	return func(*terraform.State) error {
		updates := m.Requests(http.MethodPatch, proxyConfigsPath+"/billing")
		if len(updates) != n {
			return fmt.Errorf("%s: the provider sent %d updates, want %d", step, len(updates), n)
		}
		var rules []string
		for _, rule := range prior {
			rules = append(rules, "{"+rule.fields()+`, "should_delete": true}`)
		}
		for _, rule := range planned {
			rules = append(rules, "{"+rule.fields()+"}")
		}
		want := `{"name": "Billing API", "auth_mechanism": "Bearer",
			"secret": "example-bearer-token", "mapping_rules": [` + strings.Join(rules, ", ") + `]}`
		if err := updates[len(updates)-1].CheckJSONBody(want); err != nil {
			return fmt.Errorf("%s: %w", step, err)
		}
		return nil
	}
}

// patchStoredRules sends the mock an update of the billing proxy config with these
// mapping rules, a JSON list, as a change made outside Terraform.
func patchStoredRules(t *testing.T, m *mockpermit.Server, rules string) {
	t.Helper()
	body := `{"auth_mechanism": "Bearer", "secret": "example-bearer-token", ` +
		`"mapping_rules": ` + rules + `}`
	request, err := http.NewRequest(http.MethodPatch, m.URL+proxyConfigsPath+"/billing",
		strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+mockpermit.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		responseBody, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("updating the stored mapping rules: %s; reading the body: %v",
				response.Status, err)
		}
		t.Fatalf("updating the stored mapping rules: %s: %s", response.Status, responseBody)
	}
}
