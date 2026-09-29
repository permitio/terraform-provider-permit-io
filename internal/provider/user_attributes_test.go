package provider

import (
	"fmt"
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
			description := "acceptance test for type " + attributeType
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: providerConfig + fmt.Sprintf(`
							resource "permitio_user_attribute" "test_%[1]s" {
								key         = "tf_acc_type_%[1]s"
								type        = "%[1]s"
								description = "%[2]s"
							}`, attributeType, description),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(resourceName, "type", attributeType),
							resource.TestCheckResourceAttr(resourceName, "description", description),
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
	const (
		sourceName = "permitio_user_attribute.source"
		dataName   = "data.permitio_user_attribute.by_key"
	)

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
					resource.TestCheckResourceAttr(dataName, "key", "tf_acc_ds_test"),
					resource.TestCheckResourceAttr(dataName, "type", "number"),
					resource.TestCheckResourceAttr(dataName, "description", "data source acceptance test"),
					resource.TestCheckResourceAttrSet(dataName, "environment_id"),
					resource.TestCheckResourceAttrPair(dataName, "id", sourceName, "id"),
					resource.TestCheckResourceAttrPair(dataName, "resource_id", sourceName, "resource_id"),
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
