package user_attributes_test

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

const userAttributesPath = "/v2/schema/" + mockpermit.ProjectID + "/" +
	mockpermit.EnvironmentID + "/resources/__user/attributes"

// TestUserAttributeCreateUpdateDestroy runs permitio_user_attribute through
// Terraform against the mock Permit API and checks the exact bodies the provider
// sends. The update changes both attributes it can: type and description.
func TestUserAttributeCreateUpdateDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.ResourceAttributes)
	const address = "permitio_user_attribute.department"
	attributeID := mockpermit.ObjectID(1)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: userAttributeConfig("string", "The department the user works in"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", attributeID),
					resource.TestCheckResourceAttr(address, "key", "department"),
					resource.TestCheckResourceAttr(address, "type", "string"),
					resource.TestCheckResourceAttr(address, "description",
						"The department the user works in"),
					resource.TestCheckResourceAttr(address, "resource_key", "__user"),
					resource.TestCheckResourceAttr(address, "resource_id",
						mockpermit.UserResourceID),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					m.CheckRequests(http.MethodPost, userAttributesPath, `{
						"key": "department",
						"type": "string",
						"description": "The department the user works in"
					}`),
				),
			},
			{
				Config: userAttributeConfig("array", "The departments the user works in"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", attributeID),
					resource.TestCheckResourceAttr(address, "type", "array"),
					resource.TestCheckResourceAttr(address, "description",
						"The departments the user works in"),
					m.CheckRequests(http.MethodPatch, userAttributesPath+"/"+attributeID, `{
						"type": "array",
						"description": "The departments the user works in"
					}`),
				),
			},
		},
	})

	m.AssertAllRoutesHit()
}

func userAttributeConfig(attributeType, description string) string {
	return fmt.Sprintf(`
resource "permitio_user_attribute" "department" {
  key         = "department"
  type        = %q
  description = %q
}
`, attributeType, description)
}
