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
		},
	})
}

func TestUserAttributeAllTypes(t *testing.T) {
	config := providerConfig
	checks := make([]resource.TestCheckFunc, 0)

	for _, attributeType := range []string{"bool", "number", "string", "time", "array", "json"} {
		config += `
			resource "permitio_user_attribute" "test_` + attributeType + `" {
				key         = "tf_acc_type_` + attributeType + `"
				type        = "` + attributeType + `"
				description = "acceptance test for type ` + attributeType + `"
			}`
		checks = append(checks,
			resource.TestCheckResourceAttr("permitio_user_attribute.test_"+attributeType, "type", attributeType),
			resource.TestCheckResourceAttr("permitio_user_attribute.test_"+attributeType, "resource_key", "__user"),
			resource.TestCheckResourceAttrSet("permitio_user_attribute.test_"+attributeType, "id"),
		)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.ComposeAggregateTestCheckFunc(checks...),
			},
		},
	})
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
					resource.TestCheckResourceAttr("data.permitio_user_attribute.by_key", "resource_key", "__user"),
					resource.TestCheckResourceAttrSet("data.permitio_user_attribute.by_key", "id"),
					resource.TestCheckResourceAttrSet("data.permitio_user_attribute.by_key", "environment_id"),
					resource.TestCheckResourceAttrPair(
						"data.permitio_user_attribute.by_key", "id",
						"permitio_user_attribute.source", "id",
					),
				),
			},
		},
	})
}
