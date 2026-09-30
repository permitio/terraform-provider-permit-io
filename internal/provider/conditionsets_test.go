package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccConditionSetName returns a condition set name for one test run, where runID is
// the run's value from acctest.RandomWithPrefix. Condition set names are unique within
// an environment, like keys, so a fixed name conflicts with any set of that name
// already there, such as one left behind by an earlier run.
func testAccConditionSetName(label, runID string) string {
	return label + " " + runID
}

// TestConditionSetNameIsUniquePerRun checks that two runs never ask for the same
// condition set name, and that two sets in one run do not either.
func TestConditionSetNameIsUniquePerRun(t *testing.T) {
	first := testAccConditionSetName("Parent User Set", "tfacc-1")
	if second := testAccConditionSetName("Parent User Set", "tfacc-2"); first == second {
		t.Errorf("runs tfacc-1 and tfacc-2 both get the name %q", first)
	}
	if !strings.Contains(first, "tfacc-1") {
		t.Errorf("name %q for run tfacc-1 does not contain the run ID", first)
	}
	if child := testAccConditionSetName("Child User Set", "tfacc-1"); first == child {
		t.Errorf("labels Parent User Set and Child User Set both get the name %q", first)
	}
}

// TestAccUserSetWithContains tests that the contains operator in conditions is correctly preserved.
func TestAccUserSetWithContains(t *testing.T) {
	key := acctest.RandomWithPrefix(testAccKeyPrefix)
	name := testAccConditionSetName("Test Contains Operator", key)
	updatedName := testAccConditionSetName("Test Contains Operator Updated", key)
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
						name = %q
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
					}`, key, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "key", key),
					resource.TestCheckResourceAttr(address, "name", name),
					// Check that the conditions contain the 'contains' operator
					resource.TestCheckResourceAttrSet(address, "conditions"),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     key,
				ImportStateVerify: true,
			},
			// Update testing
			{
				Config: providerConfig + fmt.Sprintf(`
					resource "permitio_user_set" "test_contains" {
						key  = %q
						name = %q
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
					}`, key, updatedName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "key", key),
					resource.TestCheckResourceAttr(address, "name", updatedName),
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
	parentName := testAccConditionSetName("Parent User Set", testID)
	childName := testAccConditionSetName("Child User Set", testID)
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
						name = %q
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
						name = %q
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
					}`, parentKey, parentName, childKey, childName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(parent, "key", parentKey),
					resource.TestCheckResourceAttr(parent, "name", parentName),
					resource.TestCheckResourceAttr(child, "key", childKey),
					resource.TestCheckResourceAttr(child, "name", childName),
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
	setName := testAccConditionSetName("Test Resource Set with Contains", testID)
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
						name     = %q
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
					}`, resourceKey, setKey, setName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "key", setKey),
					resource.TestCheckResourceAttr(address, "name", setName),
					resource.TestCheckResourceAttrSet(address, "conditions"),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     setKey,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccUserSetMultipleOperators tests complex conditions with multiple operators.
func TestAccUserSetMultipleOperators(t *testing.T) {
	key := acctest.RandomWithPrefix(testAccKeyPrefix)
	name := testAccConditionSetName("Test Multiple Operators", key)
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
						name = %q
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
					}`, key, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "key", key),
					resource.TestCheckResourceAttr(address, "name", name),
					resource.TestCheckResourceAttrSet(address, "conditions"),
				),
			},
		},
	})
}
