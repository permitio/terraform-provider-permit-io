package proxy_configs_test

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
)

var providerFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"permitio": providerserver.NewProtocol6WithError(provider.New("test")()),
}

const (
	proxyConfigsPath = "/v2/facts/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID +
		"/proxy_configs"
	// The mapping rules the config starts with, as the provider sends them. The
	// first has every optional field, the second none.
	invoiceRules = `
		{"url": "https://billing.example.com/v1/invoices", "http_method": "get",
		 "resource": "invoice", "action": "read", "priority": 1,
		 "headers": {"x-tenant": "required"}},
		{"url": "https://billing.example.com/v1/invoices", "http_method": "post",
		 "resource": "invoice"}`
)

// TestProxyConfigCreateUpdateDestroy runs permitio_proxy_config through Terraform
// against the mock Permit API and checks the exact bodies the provider sends. The
// update changes every attribute a proxy config updates in place: name, the auth
// mechanism with its secret, from Bearer to Basic, and the mapping rules, by adding
// one. Each plan must mark the secret sensitive, so that Terraform hides it.
func TestProxyConfigCreateUpdateDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.ProxyConfigs)
	const address = "permitio_proxy_config.billing"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: `
resource "permitio_proxy_config" "billing" {
  key            = "billing"
  name           = "Billing API"
  auth_mechanism = "Bearer"
  auth_secret = {
    bearer = "example-bearer-token"
  }
  mapping_rules = [
    {
      url         = "https://billing.example.com/v1/invoices"
      http_method = "get"
      resource    = "invoice"
      action      = "read"
      priority    = 1
      headers     = { "x-tenant" = "required" }
    },
    {
      url         = "https://billing.example.com/v1/invoices"
      http_method = "post"
      resource    = "invoice"
    },
  ]
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectSensitiveValue(address, tfjsonpath.New("auth_secret")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(1)),
					resource.TestCheckResourceAttr(address, "key", "billing"),
					resource.TestCheckResourceAttr(address, "name", "Billing API"),
					resource.TestCheckResourceAttr(address, "auth_mechanism", "Bearer"),
					resource.TestCheckResourceAttr(address, "auth_secret.bearer",
						"example-bearer-token"),
					resource.TestCheckNoResourceAttr(address, "auth_secret.basic"),
					resource.TestCheckResourceAttr(address, "mapping_rules.#", "2"),
					resource.TestCheckResourceAttr(address, "mapping_rules.0.url",
						"https://billing.example.com/v1/invoices"),
					resource.TestCheckResourceAttr(address, "mapping_rules.0.http_method", "get"),
					resource.TestCheckResourceAttr(address, "mapping_rules.0.resource", "invoice"),
					resource.TestCheckResourceAttr(address, "mapping_rules.0.action", "read"),
					resource.TestCheckResourceAttr(address, "mapping_rules.0.priority", "1"),
					resource.TestCheckResourceAttr(address, "mapping_rules.0.headers.x-tenant",
						"required"),
					resource.TestCheckResourceAttr(address, "mapping_rules.1.http_method", "post"),
					resource.TestCheckNoResourceAttr(address, "mapping_rules.1.action"),
					resource.TestCheckNoResourceAttr(address, "mapping_rules.1.priority"),
					resource.TestCheckNoResourceAttr(address, "mapping_rules.1.headers.%"),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					m.CheckRequests(http.MethodPost, proxyConfigsPath, `{
						"key": "billing",
						"name": "Billing API",
						"auth_mechanism": "Bearer",
						"secret": "example-bearer-token",
						"mapping_rules": [`+invoiceRules+`]
					}`),
				),
			},
			{
				Config: `
resource "permitio_proxy_config" "billing" {
  key            = "billing"
  name           = "Billing and refunds API"
  auth_mechanism = "Basic"
  auth_secret = {
    basic = "example-user:example-password"
  }
  mapping_rules = [
    {
      url         = "https://billing.example.com/v1/invoices"
      http_method = "get"
      resource    = "invoice"
      action      = "read"
      priority    = 1
      headers     = { "x-tenant" = "required" }
    },
    {
      url         = "https://billing.example.com/v1/invoices"
      http_method = "post"
      resource    = "invoice"
    },
    {
      url         = "https://billing.example.com/v1/refunds"
      http_method = "post"
      resource    = "refund"
      action      = "create"
      priority    = 2
    },
  ]
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
						plancheck.ExpectSensitiveValue(address, tfjsonpath.New("auth_secret")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(1)),
					resource.TestCheckResourceAttr(address, "name", "Billing and refunds API"),
					resource.TestCheckResourceAttr(address, "auth_mechanism", "Basic"),
					resource.TestCheckResourceAttr(address, "auth_secret.basic",
						"example-user:example-password"),
					resource.TestCheckNoResourceAttr(address, "auth_secret.bearer"),
					resource.TestCheckResourceAttr(address, "mapping_rules.#", "3"),
					resource.TestCheckResourceAttr(address, "mapping_rules.0.headers.x-tenant",
						"required"),
					resource.TestCheckNoResourceAttr(address, "mapping_rules.1.action"),
					resource.TestCheckResourceAttr(address, "mapping_rules.2.url",
						"https://billing.example.com/v1/refunds"),
					resource.TestCheckResourceAttr(address, "mapping_rules.2.resource", "refund"),
					resource.TestCheckResourceAttr(address, "mapping_rules.2.priority", "2"),
					resource.TestCheckNoResourceAttr(address, "mapping_rules.2.headers.%"),
					m.CheckRequests(http.MethodPatch, proxyConfigsPath+"/billing", `{
						"name": "Billing and refunds API",
						"auth_mechanism": "Basic",
						"secret": "example-user:example-password",
						"mapping_rules": [`+invoiceRules+`,
							{"url": "https://billing.example.com/v1/refunds",
							 "http_method": "post", "resource": "refund", "action": "create",
							 "priority": 2}]
					}`),
				),
			},
		},
	})

	m.AssertAllRoutesHit()
}
