package user_attributes_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// TestUserAttributeWithoutDescription runs permitio_user_attribute, whose
// description is optional, through the ways a configuration can leave it out. A
// new attribute without one gets an empty description. Removing a description
// that was set plans no change and keeps it, also when the type changes in the
// same apply; setting it to "" clears it.
func TestUserAttributeWithoutDescription(t *testing.T) {
	m := mockpermit.New(t, mockpermit.ResourceAttributes)
	const (
		address     = "permitio_user_attribute.team"
		description = "The team the user is on"
	)
	attributeID := mockpermit.ObjectID(1)
	attributePath := userAttributesPath + "/" + attributeID
	withoutDescription := func(attributeType string) string {
		return fmt.Sprintf(`
resource "permitio_user_attribute" "team" {
  key  = "team"
  type = %q
}
`, attributeType)
	}
	withDescription := func(attributeType, text string) string {
		return fmt.Sprintf(`
resource "permitio_user_attribute" "team" {
  key         = "team"
  type        = %q
  description = %q
}
`, attributeType, text)
	}
	setDescription := `{"type": "string", "description": "The team the user is on"}`
	changeType := `{"type": "number", "description": "The team the user is on"}`
	clearDescription := `{"type": "number", "description": ""}`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: withoutDescription("string"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", attributeID),
					resource.TestCheckResourceAttr(address, "key", "team"),
					resource.TestCheckResourceAttr(address, "type", "string"),
					resource.TestCheckResourceAttr(address, "description", ""),
					m.CheckRequests(http.MethodPost, userAttributesPath, `{
						"key": "team",
						"type": "string",
						"description": ""
					}`),
				),
			},
			{
				Config: withDescription("string", description),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "description", description),
					m.CheckRequests(http.MethodPatch, attributePath, setDescription),
				),
			},
			{
				Config: withoutDescription("string"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "description", description),
					m.CheckRequests(http.MethodPatch, attributePath, setDescription),
				),
			},
			{
				Config: withoutDescription("number"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "type", "number"),
					resource.TestCheckResourceAttr(address, "description", description),
					m.CheckRequests(http.MethodPatch, attributePath, setDescription,
						changeType),
				),
			},
			{
				Config: withDescription("number", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "description", ""),
					m.CheckRequests(http.MethodPatch, attributePath, setDescription,
						changeType, clearDescription),
				),
			},
		},
	})
}
