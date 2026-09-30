package user_attributes_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// TestUserAttributeDataSourceRead reads a user attribute the mock Permit API
// serves through the permitio_user_attribute data source and checks every
// attribute it exports.
func TestUserAttributeDataSourceRead(t *testing.T) {
	m := mockpermit.New(t, mockpermit.ResourceAttributes)
	const address = "data.permitio_user_attribute.team"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: `
resource "permitio_user_attribute" "team" {
  key         = "team"
  type        = "string"
  description = "The team the user is on"
}

data "permitio_user_attribute" "team" {
  key = permitio_user_attribute.team.key
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(1)),
					resource.TestCheckResourceAttr(address, "key", "team"),
					resource.TestCheckResourceAttr(address, "type", "string"),
					resource.TestCheckResourceAttr(address, "description",
						"The team the user is on"),
					resource.TestCheckResourceAttr(address, "resource_id",
						mockpermit.UserResourceID),
					resource.TestCheckResourceAttr(address, "resource_key", "__user"),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					resource.TestCheckResourceAttr(address, "created_at",
						"2026-01-01 00:00:00 +0000 UTC"),
					resource.TestCheckResourceAttr(address, "updated_at",
						"2026-01-01 00:00:00 +0000 UTC"),
				),
			},
		},
	})
}
