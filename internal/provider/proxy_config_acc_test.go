package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccMappingRule is a proxy config mapping rule as the acceptance test writes it
// and as it expects Permit to store it.
type testAccMappingRule struct {
	url, method, resource, action string
}

// TestAccProxyConfigMappingRules checks against the Permit API that an update of a
// proxy config's mapping rules, which removes every prior rule and then adds the
// planned ones in one request, leaves Permit with exactly the planned rules in the
// planned order: after a reorder with a changed http_method, after a rule is removed,
// another inserted in the middle and a kept rule's action changed, after all but one
// are removed, and after all are removed. The provider's Update and Read return the
// rules in the planned and state order, so only this check, not the apply or the empty
// plan after each step, catches Permit holding them in another order.
func TestAccProxyConfigMappingRules(t *testing.T) {
	testID := acctest.RandomWithPrefix(testAccKeyPrefix)
	resourceKey := testID + "-invoice"
	proxyKey := testID + "-billing"
	const (
		address        = "permitio_proxy_config.billing"
		invoices       = "https://billing.example.com/v1/invoices"
		invoiceArchive = "https://billing.example.com/v1/invoices/archive"
	)
	a := testAccMappingRule{invoices, "get", resourceKey, "read"}
	b := testAccMappingRule{invoices, "post", resourceKey, "write"}
	bPut := testAccMappingRule{invoices, "put", resourceKey, "write"}
	c := testAccMappingRule{invoiceArchive, "get", resourceKey, "read"}
	cWrite := testAccMappingRule{invoiceArchive, "get", resourceKey, "write"}
	d := testAccMappingRule{invoiceArchive, "delete", resourceKey, "write"}

	config := func(rules ...testAccMappingRule) string {
		var ruleBlocks strings.Builder
		for _, rule := range rules {
			fmt.Fprintf(&ruleBlocks, `
				{
					url         = %q
					http_method = %q
					resource    = permitio_resource.invoice.key
					action      = %q
				},`, rule.url, rule.method, rule.action)
		}
		return providerConfig + fmt.Sprintf(`
			resource "permitio_resource" "invoice" {
				key  = %q
				name = "Invoice"
				actions = {
					read  = { name = "Read" }
					write = { name = "Write" }
				}
			}
			resource "permitio_proxy_config" "billing" {
				key            = %q
				name           = "Billing API"
				auth_mechanism = "Bearer"
				auth_secret = {
					bearer = "tfacc-not-a-real-token"
				}
				mapping_rules = [%s
				]
			}`, resourceKey, proxyKey, ruleBlocks.String())
	}
	updateStep := func(rules ...testAccMappingRule) resource.TestStep {
		return resource.TestStep{
			Config: config(rules...),
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
				},
			},
			Check: testAccCheckProxyConfigRules(proxyKey, rules),
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(a, b, c),
				Check:  testAccCheckProxyConfigRules(proxyKey, []testAccMappingRule{a, b, c}),
			},
			updateStep(c, a, bPut),
			updateStep(cWrite, d, bPut),
			updateStep(d),
			updateStep(),
		},
	})
}

// testAccCheckProxyConfigRules reads the proxy config with this key through the SDK
// and fails unless Permit holds exactly the rules want, in that order.
func testAccCheckProxyConfigRules(key string, want []testAccMappingRule) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client, err := testAccPermitClient()
		if err != nil {
			return fmt.Errorf("building the client to read proxy config %s: %w", key, err)
		}
		proxyConfig, err := client.Api.ProxyConfigs.Get(context.Background(), key)
		if err != nil {
			return fmt.Errorf("reading proxy config %s: %w", key, err)
		}
		got := make([]testAccMappingRule, 0, len(proxyConfig.MappingRules))
		for _, rule := range proxyConfig.MappingRules {
			action := ""
			if rule.Action != nil {
				action = *rule.Action
			}
			got = append(got, testAccMappingRule{
				rule.Url, string(rule.HttpMethod), rule.Resource, action,
			})
		}
		if !slices.Equal(got, want) {
			return fmt.Errorf("proxy config %s has the mapping rules %+v in Permit, want %+v",
				key, got, want)
		}
		return nil
	}
}
