package provider

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccTenantAttributesRemoved checks against the Permit API that removing the
// attributes from a tenant's configuration clears them in Permit, and leaves them
// null in the state (PER-16603).
func TestAccTenantAttributesRemoved(t *testing.T) {
	key := acctest.RandomWithPrefix(testAccKeyPrefix)
	const address = "permitio_tenant.test"
	config := func(attributes string) string {
		return providerConfig + fmt.Sprintf(`
			resource "permitio_tenant" "test" {
				key  = %q
				name = "Attributes test"
				%s
			}`, key, attributes)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroyed(key),
		Steps: []resource.TestStep{
			{
				Config: config(`attributes = jsonencode({ tier = "gold", seats = 25 })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "attributes",
						`{"seats":25,"tier":"gold"}`),
					testAccCheckTenantAttributes(key,
						map[string]any{"seats": float64(25), "tier": "gold"}),
				),
			},
			{
				Config: config(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(address, "attributes"),
					testAccCheckTenantAttributes(key, nil),
				),
			},
		},
	})
}

// testAccCheckTenantAttributes reads the tenant with this key through the SDK and
// fails unless Permit holds the attributes want, or none when want is empty.
func testAccCheckTenantAttributes(key string, want map[string]any) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client, err := testAccPermitClient()
		if err != nil {
			return fmt.Errorf("building the client to read tenant %s: %w", key, err)
		}
		tenant, err := client.Api.Tenants.Get(context.Background(), key)
		if err != nil {
			return fmt.Errorf("reading tenant %s: %w", key, err)
		}
		if len(want) == 0 && len(tenant.Attributes) == 0 {
			return nil
		}
		if !reflect.DeepEqual(tenant.Attributes, want) {
			return fmt.Errorf("tenant %s has the attributes %v in Permit, want %v", key,
				tenant.Attributes, want)
		}
		return nil
	}
}

// testAccCheckTenantDestroyed fails unless Permit answers 404 for the tenant with
// this key.
func testAccCheckTenantDestroyed(key string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client, err := testAccPermitClient()
		if err != nil {
			return fmt.Errorf("building the client for the destroy check: %w", err)
		}
		_, err = client.Api.Tenants.Get(context.Background(), key)
		exists, err := testAccExistsFromErr(err)
		if err != nil {
			return fmt.Errorf("checking that tenant %s was destroyed: %w", key, err)
		}
		if exists {
			return fmt.Errorf("tenant %s still exists in Permit after destroy", key)
		}
		return nil
	}
}
