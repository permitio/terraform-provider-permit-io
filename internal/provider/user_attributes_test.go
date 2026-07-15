package provider

import (
	"regexp"
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
			{
				Config: providerConfig +
					`resource "permitio_user_attribute" "test" {
						key         = "test"
						type        = "number"
						description = "an updated test"
					}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("permitio_user_attribute.test", "key", "test"),
					resource.TestCheckResourceAttr("permitio_user_attribute.test", "type", "number"),
					resource.TestCheckResourceAttr("permitio_user_attribute.test", "description", "an updated test"),
				),
			},
			{
				// Pins the API behavior for empty descriptions: "" round-trips
				// as "" (not null) on both update and read.
				Config: providerConfig +
					`resource "permitio_user_attribute" "test" {
						key         = "test"
						type        = "number"
						description = ""
					}`,
				Check: resource.TestCheckResourceAttr("permitio_user_attribute.test", "description", ""),
			},
		},
	})
}

func TestUserAttributeAllTypes(t *testing.T) {
	for _, attributeType := range []string{"bool", "number", "string", "time", "array", "json"} {
		t.Run(attributeType, func(t *testing.T) {
			resourceName := "permitio_user_attribute.test_" + attributeType
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: providerConfig + `
							resource "permitio_user_attribute" "test_` + attributeType + `" {
								key         = "tf_acc_type_` + attributeType + `"
								type        = "` + attributeType + `"
								description = "acceptance test for type ` + attributeType + `"
							}`,
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(resourceName, "type", attributeType),
							resource.TestCheckResourceAttr(resourceName, "description", "acceptance test for type "+attributeType),
							resource.TestCheckResourceAttrSet(resourceName, "id"),
							resource.TestCheckResourceAttrSet(resourceName, "resource_id"),
						),
					},
				},
			})
		})
	}
}

func TestUserAttributeDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig +
					`resource "permitio_user_attribute" "source" {
						key         = "tf_acc_ds_test"
						type        = "number"
						description = "data source acceptance test"
					}

					data "permitio_user_attribute" "by_key" {
						key = permitio_user_attribute.source.key
					}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.permitio_user_attribute.by_key", "key", "tf_acc_ds_test"),
					resource.TestCheckResourceAttr("data.permitio_user_attribute.by_key", "type", "number"),
					resource.TestCheckResourceAttr("data.permitio_user_attribute.by_key", "description", "data source acceptance test"),
					resource.TestCheckResourceAttrSet("data.permitio_user_attribute.by_key", "environment_id"),
					resource.TestCheckResourceAttrPair(
						"data.permitio_user_attribute.by_key", "id",
						"permitio_user_attribute.source", "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.permitio_user_attribute.by_key", "resource_id",
						"permitio_user_attribute.source", "resource_id",
					),
				),
			},
		},
	})
}

func TestUserAttributeDataSourceNotFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig +
					`data "permitio_user_attribute" "missing" {
						key = "tf_acc_does_not_exist"
					}`,
				ExpectError: regexp.MustCompile("Unable to read user attribute"),
			},
		},
	})
}
