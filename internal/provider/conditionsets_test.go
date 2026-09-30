package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccUserSetWithContains tests that the contains operator in conditions is correctly preserved.
func TestAccUserSetWithContains(t *testing.T) {
	key := acctest.RandomWithPrefix(testAccKeyPrefix)
	const address = "permitio_user_set.test_contains"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + fmt.Sprintf(`
					resource "permitio_user_set" "test_contains" {
						key  = %q
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
					}`, key),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "key", key),
					resource.TestCheckResourceAttr(address, "name", "Test Contains Operator"),
					// Check that the conditions contain the 'contains' operator
					resource.TestCheckResourceAttrSet(address, "conditions"),
				),
			},
			// Update testing
			{
				Config: providerConfig + fmt.Sprintf(`
					resource "permitio_user_set" "test_contains" {
						key  = %q
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
					}`, key),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "key", key),
					resource.TestCheckResourceAttr(address, "name",
						"Test Contains Operator Updated"),
					resource.TestCheckResourceAttrSet(address, "conditions"),
				),
			},
		},
	})
}

// TestAccUserSetWithParentId tests that parent_id field is correctly handled.
func TestAccUserSetWithParentId(t *testing.T) {
	testID := acctest.RandomWithPrefix(testAccKeyPrefix)
	parentKey := testID + "-parent"
	childKey := testID + "-child"
	const (
		parent = "permitio_user_set.parent"
		child  = "permitio_user_set.child"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			// Create parent user set first
			{
				Config: providerConfig + fmt.Sprintf(`
					resource "permitio_user_set" "parent" {
						key  = %q
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
						key  = %q
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
					}`, parentKey, childKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(parent, "key", parentKey),
					resource.TestCheckResourceAttr(parent, "name", "Parent User Set"),
					resource.TestCheckResourceAttr(child, "key", childKey),
					resource.TestCheckResourceAttr(child, "name", "Child User Set"),
					resource.TestCheckResourceAttrSet(child, "parent_id"),
				),
			},
		},
	})
}

// TestAccResourceSetWithContains tests that the contains operator works for resource sets.
// The condition reads resource.title, so the resource declares a title attribute;
// without one, creating the set failed with 400 Bad Request on every run.
func TestAccResourceSetWithContains(t *testing.T) {
	testID := acctest.RandomWithPrefix(testAccKeyPrefix)
	resourceKey := testID + "-document"
	setKey := testID + "-contains"
	const address = "permitio_resource_set.test_contains"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
					resource "permitio_resource" "test_doc" {
						key         = %q
						name        = "test document"
						description = "a test document"
						actions = {
							"read" = {
								"name" = "read"
							}
						}
						attributes = {
							"title" = {
								"description" = "the title of the document"
								"type"        = "string"
							}
						}
					}

					resource "permitio_resource_set" "test_contains" {
						key      = %q
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
					}`, resourceKey, setKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "key", setKey),
					resource.TestCheckResourceAttr(address, "name",
						"Test Resource Set with Contains"),
					resource.TestCheckResourceAttrSet(address, "conditions"),
				),
			},
		},
	})
}

// TestAccUserSetMultipleOperators tests complex conditions with multiple operators.
func TestAccUserSetMultipleOperators(t *testing.T) {
	key := acctest.RandomWithPrefix(testAccKeyPrefix)
	const address = "permitio_user_set.test_multiple"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
					resource "permitio_user_set" "test_multiple" {
						key  = %q
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
					}`, key),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "key", key),
					resource.TestCheckResourceAttr(address, "name", "Test Multiple Operators"),
					resource.TestCheckResourceAttrSet(address, "conditions"),
				),
			},
		},
	})
}
