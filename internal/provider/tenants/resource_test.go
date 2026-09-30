package tenants_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
)

var providerFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"permitio": providerserver.NewProtocol6WithError(provider.New("test")()),
}

const tenantsPath = "/v2/facts/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID +
	"/tenants"

// TestTenantCreateUpdateDestroy runs permitio_tenant through Terraform against the
// mock Permit API and checks the exact bodies the provider sends. The update
// changes every attribute the tenant updates in place: name, description and
// attributes. The new attributes keep every key of the old ones, and jsonencode
// writes them the way the provider reads them back.
func TestTenantCreateUpdateDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Tenants)
	const address = "permitio_tenant.test"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: tenantConfig("Acme", "First tenant", `{ tier = "gold" }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(1)),
					resource.TestCheckResourceAttr(address, "key", "acme"),
					resource.TestCheckResourceAttr(address, "name", "Acme"),
					resource.TestCheckResourceAttr(address, "description", "First tenant"),
					resource.TestCheckResourceAttr(address, "attributes", `{"tier":"gold"}`),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					resource.TestCheckResourceAttr(address, "created_at",
						"2026-01-01 00:00:00 +0000 UTC"),
					resource.TestCheckResourceAttr(address, "last_action_at",
						"2026-01-01 00:00:00 +0000 UTC"),
					m.CheckRequests(http.MethodPost, tenantsPath, `{
						"key": "acme",
						"name": "Acme",
						"description": "First tenant",
						"attributes": {"tier": "gold"}
					}`),
				),
			},
			{
				Config: tenantConfig("Acme Renamed", "The first tenant, renamed",
					`{ seats = 25, tier = "platinum" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(1)),
					resource.TestCheckResourceAttr(address, "name", "Acme Renamed"),
					resource.TestCheckResourceAttr(address, "description",
						"The first tenant, renamed"),
					resource.TestCheckResourceAttr(address, "attributes",
						`{"seats":25,"tier":"platinum"}`),
					m.CheckRequests(http.MethodPost, tenantsPath, `{
						"key": "acme",
						"name": "Acme",
						"description": "First tenant",
						"attributes": {"tier": "gold"}
					}`),
					m.CheckRequests(http.MethodPatch, tenantsPath+"/acme", `{
						"name": "Acme Renamed",
						"description": "The first tenant, renamed",
						"attributes": {"seats": 25, "tier": "platinum"}
					}`),
				),
			},
		},
	})

	m.AssertAllRoutesHit()
}

func tenantConfig(name, description, attributes string) string {
	return fmt.Sprintf(`
resource "permitio_tenant" "test" {
  key         = "acme"
  name        = %q
  description = %q
  attributes  = jsonencode(%s)
}
`, name, description, attributes)
}
