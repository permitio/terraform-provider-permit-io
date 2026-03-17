package provider

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestResourceWithoutAttributes verifies that creating a resource without
// the optional attributes field does not cause "inconsistent result after apply".
func TestResourceWithoutAttributes(t *testing.T) {
	testID := fmt.Sprintf("test-%d-%d", time.Now().Unix(), rand.Intn(10000))
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`resource "permitio_resource" "no_attrs" {
					key  = "noattrs-%s"
					name = "noattrs-%s"
					description = "resource without attributes"
					actions = {
						"read" = {
							"name" = "Read"
						}
					}
				}`, testID, testID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("permitio_resource.no_attrs", "key", fmt.Sprintf("noattrs-%s", testID)),
					resource.TestCheckResourceAttr("permitio_resource.no_attrs", "name", fmt.Sprintf("noattrs-%s", testID)),
				),
			},
		},
	})
}

// TestResourceScopedRole verifies that creating a resource-scoped role works
// and that the extends field is properly typed even when empty.
func TestResourceScopedRole(t *testing.T) {
	testID := fmt.Sprintf("test-%d-%d", time.Now().Unix(), rand.Intn(10000))
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
				resource "permitio_resource" "workspace" {
					key  = "workspace-%s"
					name = "workspace-%s"
					actions = {
						"read" = {
							"name" = "Read"
						}
						"write" = {
							"name" = "Write"
						}
					}
				}

				resource "permitio_role" "workspace_viewer" {
					key         = "viewer-%s"
					name        = "Viewer"
					description = "Viewer access to workspace"
					permissions = ["read"]
					resource    = permitio_resource.workspace.key
					depends_on  = [permitio_resource.workspace]
				}`, testID, testID, testID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("permitio_role.workspace_viewer", "key", fmt.Sprintf("viewer-%s", testID)),
					resource.TestCheckResourceAttr("permitio_role.workspace_viewer", "name", "Viewer"),
					resource.TestCheckResourceAttr("permitio_role.workspace_viewer", "resource", fmt.Sprintf("workspace-%s", testID)),
				),
			},
		},
	})
}
