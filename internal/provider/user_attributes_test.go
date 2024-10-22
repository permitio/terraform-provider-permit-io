package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestUserAttributes(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig +
					`resource "permitio_user_attribute" "test" {
						key         = "test"
						type        = "string"
						description = "a new test"
					}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("permitio_user_attribute.test", "key", "test"),
					resource.TestCheckResourceAttr("permitio_user_attribute.test", "type", "string"),
					resource.TestCheckResourceAttr("permitio_user_attribute.test", "description", "a new test"),
				),
			},
		},
	})
}
