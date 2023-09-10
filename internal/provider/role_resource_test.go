package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCoffeesDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + `resource "permitio_role" "test" {
							  key         = "writer"
							  name        = "writer"
							  description = "a new writer"
							  permissions = [
								"farm:set-on-fire"
							  ]
							  extends = [
								"admin"
							  ]
							
								}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Verify number of coffees returned
					resource.TestCheckResourceAttr("permitio_role.test", "key", "writer"),
					// Verify the first coffee to ensure all attributes are set
					resource.TestCheckResourceAttr("permitio_role.test", "name", "writer"),
					resource.TestCheckResourceAttr("permitio_role.test", "description", "a new writer"),
					resource.TestCheckResourceAttr("permitio_role.test", "permissions", "[\"farm:set-on-fire\"]"),
					// Verify placeholder id attribute
				),
			},
		},
	})
}
