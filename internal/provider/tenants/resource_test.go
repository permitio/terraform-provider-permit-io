package tenants_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
)

var providerFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"permitio": providerserver.NewProtocol6WithError(provider.New("test")()),
}

const tenantsPath = "/v2/facts/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID +
	"/tenants"

// TestTenantCreateUpdateDestroy runs permitio_tenant through Terraform against the
// mock Permit API and checks the exact bodies the provider sends.
func TestTenantCreateUpdateDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Tenants)
	const address = "permitio_tenant.test"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy: func(*terraform.State) error {
			if keys := m.StoredKeys(); len(keys) > 0 {
				return fmt.Errorf("objects left in the mock after destroy: %q", keys)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: tenantConfig("Acme"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id",
						"00000000-0000-4000-8000-000000000001"),
					resource.TestCheckResourceAttr(address, "key", "acme"),
					resource.TestCheckResourceAttr(address, "name", "Acme"),
					resource.TestCheckResourceAttr(address, "description", "First tenant"),
					resource.TestCheckResourceAttr(address, "attributes", `{"tier":"gold"}`),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					checkOnlyRequest(m, http.MethodPost, tenantsPath, `{
						"key": "acme",
						"name": "Acme",
						"description": "First tenant",
						"attributes": {"tier": "gold"}
					}`),
				),
			},
			{
				Config: tenantConfig("Acme Renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "name", "Acme Renamed"),
					checkOnlyRequest(m, http.MethodPatch, tenantsPath+"/acme", `{
						"name": "Acme Renamed",
						"description": "First tenant",
						"attributes": {"tier": "gold"}
					}`),
				),
			},
		},
	})

	m.AssertAllRoutesHit()
}

func tenantConfig(name string) string {
	return fmt.Sprintf(`
resource "permitio_tenant" "test" {
  key         = "acme"
  name        = %q
  description = "First tenant"
  attributes  = jsonencode({ tier = "gold" })
}
`, name)
}

// checkOnlyRequest checks that the provider has sent exactly one request with this
// method and path so far, and that its body is the JSON in want.
func checkOnlyRequest(m *mockpermit.Server, method, path, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		requests := m.Requests(method, path)
		if len(requests) != 1 {
			return fmt.Errorf("%s %s: got %d requests, want 1", method, path, len(requests))
		}
		return requests[0].CheckJSONBody(want)
	}
}
