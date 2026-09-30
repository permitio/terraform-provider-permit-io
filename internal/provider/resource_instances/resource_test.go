package resource_instances_test

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

const resourceInstancesPath = "/v2/facts/" + mockpermit.ProjectID + "/" +
	mockpermit.EnvironmentID + "/resource_instances"

// TestResourceInstanceCreateUpdateImportDestroy runs permitio_resource_instance
// through Terraform against the mock Permit API and checks the exact bodies the
// provider sends. Its attributes are the only attribute it updates in place; the
// new ones keep every key of the old ones, and jsonencode writes them the way the
// provider reads them back. The import reads the instance back as resource:key.
func TestResourceInstanceCreateUpdateImportDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.Tenants,
		mockpermit.ResourceInstances)
	const address = "permitio_resource_instance.handbook"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: instanceConfig(`{ classification = "internal", pages = 12 }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					// The instance depends on the resource and the tenant, so the mock
					// creates it last, after them and the resource's action.
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(4)),
					resource.TestCheckResourceAttr(address, "key", "handbook"),
					resource.TestCheckResourceAttr(address, "resource", "document"),
					resource.TestCheckResourceAttrPair(address, "resource_id",
						"permitio_resource.document", "id"),
					resource.TestCheckResourceAttr(address, "tenant", "acme"),
					resource.TestCheckResourceAttr(address, "attributes",
						`{"classification":"internal","pages":12}`),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					resource.TestCheckResourceAttr(address, "created_at",
						"2026-01-01 00:00:00 +0000 UTC"),
					m.CheckRequests(http.MethodPost, resourceInstancesPath, `{
						"key": "handbook",
						"resource": "document",
						"tenant": "acme",
						"attributes": {"classification": "internal", "pages": 12}
					}`),
				),
			},
			{
				Config: instanceConfig(
					`{ classification = "public", pages = 14, reviewed = true }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("permitio_resource.document",
							plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("permitio_tenant.acme",
							plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "attributes",
						`{"classification":"public","pages":14,"reviewed":true}`),
					resource.TestCheckResourceAttr(address, "tenant", "acme"),
					m.CheckRequests(http.MethodPatch, resourceInstancesPath+"/document:handbook", `{
						"attributes": {"classification": "public", "pages": 14, "reviewed": true}
					}`),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     "document:handbook",
				ImportStateVerify: true,
			},
		},
	})

	m.AssertRoutesHit(mockpermit.ResourceInstances)
}

// instanceConfig returns a document resource, the acme tenant, and the handbook
// document of acme with the attributes in the HCL object attributes.
func instanceConfig(attributes string) string {
	return fmt.Sprintf(`
resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A text document"
  urn         = "prn:test:document"
  actions = {
    read = { name = "Read" }
  }
}

resource "permitio_tenant" "acme" {
  key         = "acme"
  name        = "Acme"
  description = "The acme tenant"
  attributes  = jsonencode({ tier = "gold" })
}

resource "permitio_resource_instance" "handbook" {
  key        = "handbook"
  resource   = permitio_resource.document.key
  tenant     = permitio_tenant.acme.key
  attributes = jsonencode(%s)
}
`, attributes)
}
