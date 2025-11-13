package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestUserSetWithContains tests that the contains operator in conditions is correctly preserved
func TestUserSetWithContains(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig +
					`resource "permitio_user_set" "test_contains" {
						key  = "test-contains-user-set"
						name = "Test Contains Operator"
						conditions = jsonencode({
							"allOf" : [
								{
									"allOf" : [
										{
											"subject.email" : {
												"contains" : "@test.com"
											}
										}
									]
								}
							]
						})
					}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("permitio_user_set.test_contains", "key", "test-contains-user-set"),
					resource.TestCheckResourceAttr("permitio_user_set.test_contains", "name", "Test Contains Operator"),
					// Check that the conditions contain the 'contains' operator
					resource.TestCheckResourceAttrSet("permitio_user_set.test_contains", "conditions"),
				),
			},
			// Update testing
			{
				Config: providerConfig +
					`resource "permitio_user_set" "test_contains" {
						key  = "test-contains-user-set"
						name = "Test Contains Operator Updated"
						conditions = jsonencode({
							"allOf" : [
								{
									"allOf" : [
										{
											"subject.email" : {
												"contains" : "@updated.com"
											}
										}
									]
								}
							]
						})
					}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("permitio_user_set.test_contains", "key", "test-contains-user-set"),
					resource.TestCheckResourceAttr("permitio_user_set.test_contains", "name", "Test Contains Operator Updated"),
					resource.TestCheckResourceAttrSet("permitio_user_set.test_contains", "conditions"),
				),
			},
		},
	})
}

// TestUserSetWithParentId tests that parent_id field is correctly handled
func TestUserSetWithParentId(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create parent user set first
			{
				Config: providerConfig +
					`resource "permitio_user_set" "parent" {
						key  = "parent-user-set"
						name = "Parent User Set"
						conditions = jsonencode({
							"allOf" : [
								{
									"allOf" : [
										{
											"subject.email" : {
												"equals" : "admin@test.com"
											}
										}
									]
								}
							]
						})
					}

					resource "permitio_user_set" "child" {
						key  = "child-user-set"
						name = "Child User Set"
						parent_id = permitio_user_set.parent.id
						conditions = jsonencode({
							"allOf" : [
								{
									"allOf" : [
										{
											"subject.email" : {
												"contains" : "@child.com"
											}
										}
									]
								}
							]
						})
						depends_on = [permitio_user_set.parent]
					}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("permitio_user_set.parent", "key", "parent-user-set"),
					resource.TestCheckResourceAttr("permitio_user_set.parent", "name", "Parent User Set"),
					resource.TestCheckResourceAttr("permitio_user_set.child", "key", "child-user-set"),
					resource.TestCheckResourceAttr("permitio_user_set.child", "name", "Child User Set"),
					resource.TestCheckResourceAttrSet("permitio_user_set.child", "parent_id"),
				),
			},
		},
	})
}

// TestResourceSetWithContains tests that the contains operator works for resource sets
func TestResourceSetWithContains(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig +
					`resource "permitio_resource" "test_doc" {
						key         = "test-document"
						name        = "test document"
						description = "a test document"
						actions = {
							"read" = {
								"name" = "read"
							}
						}
					}

					resource "permitio_resource_set" "test_contains" {
						key      = "test-contains-resource-set"
						name     = "Test Resource Set with Contains"
						resource = permitio_resource.test_doc.key
						conditions = jsonencode({
							"allOf" : [
								{
									"allOf" : [
										{
											"resource.title" : {
												"contains" : "secret"
											}
										}
									]
								}
							]
						})
						depends_on = [permitio_resource.test_doc]
					}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("permitio_resource_set.test_contains", "key", "test-contains-resource-set"),
					resource.TestCheckResourceAttr("permitio_resource_set.test_contains", "name", "Test Resource Set with Contains"),
					resource.TestCheckResourceAttrSet("permitio_resource_set.test_contains", "conditions"),
				),
			},
		},
	})
}

// TestUserSetMultipleOperators tests complex conditions with multiple operators
func TestUserSetMultipleOperators(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig +
					`resource "permitio_user_set" "test_multiple" {
						key  = "test-multiple-operators"
						name = "Test Multiple Operators"
						conditions = jsonencode({
							"allOf" : [
								{
									"subject.email" : {
										"contains" : "@company.com"
									}
								},
								{
									"subject.key" : {
										"equals" : "engineering_user"
									}
								}
							]
						})
					}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("permitio_user_set.test_multiple", "key", "test-multiple-operators"),
					resource.TestCheckResourceAttr("permitio_user_set.test_multiple", "name", "Test Multiple Operators"),
					resource.TestCheckResourceAttrSet("permitio_user_set.test_multiple", "conditions"),
				),
			},
		},
	})
}
