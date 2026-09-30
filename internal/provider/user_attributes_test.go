package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/user_attributes"
)

// testAccUserAttributeKey returns a random user attribute key such as
// "tfacc_1234567890". It keeps the underscores that the attribute tests have always
// sent to the API instead of acctest's hyphen: the API spec allows both, but only
// underscore keys have been seen to work.
func testAccUserAttributeKey() string {
	return strings.ReplaceAll(acctest.RandomWithPrefix(testAccKeyPrefix), "-", "_")
}

// userAttributeNotFoundError matches the data source's error for a key the API
// answers with 404, and nothing else: Terraform wraps long lines, so the words
// may be split by any whitespace.
var userAttributeNotFoundError = regexp.MustCompile(
	`(?s)Unable to read user attribute.*404\s+Not\s+Found`)

func TestAccUserAttributes(t *testing.T) {
	key := testAccUserAttributeKey()
	const address = "permitio_user_attribute.test"
	config := func(attributeType, description string) string {
		return providerConfig + fmt.Sprintf(`
			resource "permitio_user_attribute" "test" {
				key         = %q
				type        = %q
				description = %q
			}`, key, attributeType, description)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("string", "a new test"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "key", key),
					resource.TestCheckResourceAttr(address, "type", "string"),
					resource.TestCheckResourceAttr(address, "description", "a new test"),
				),
			},
			{
				Config: config("number", "an updated test"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "key", key),
					resource.TestCheckResourceAttr(address, "type", "number"),
					resource.TestCheckResourceAttr(address, "description", "an updated test"),
				),
			},
			{
				// Pins the API behavior for empty descriptions: "" round-trips
				// as "" (not null) on both update and read.
				Config: config("number", ""),
				Check:  resource.TestCheckResourceAttr(address, "description", ""),
			},
			{
				// An attribute deleted outside Terraform is created again by the next
				// apply instead of failing the plan.
				PreConfig: func() { testAccDeleteUserAttribute(t, key) },
				Config:    config("number", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "key", key),
					resource.TestCheckResourceAttr(address, "type", "number"),
				),
			},
		},
	})
}

// testAccDeleteUserAttribute deletes a user attribute through the SDK, the way a
// change outside Terraform would.
func testAccDeleteUserAttribute(t *testing.T, key string) {
	t.Helper()
	client, err := testAccPermitClient()
	if err != nil {
		t.Fatalf("building the client to delete user attribute %s: %v", key, err)
	}
	err = client.Api.ResourceAttributes.Delete(context.Background(), user_attributes.UserKey, key)
	if err != nil {
		t.Fatalf("deleting user attribute %s outside Terraform: %v", key, err)
	}
}

func TestAccUserAttributeAllTypes(t *testing.T) {
	for _, attributeType := range []string{"bool", "number", "string", "time", "array", "json"} {
		t.Run(attributeType, func(t *testing.T) {
			resourceName := "permitio_user_attribute.test_" + attributeType
			description := "acceptance test for type " + attributeType
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             testAccCheckDestroy,
				Steps: []resource.TestStep{
					{
						Config: providerConfig + fmt.Sprintf(`
							resource "permitio_user_attribute" "test_%[1]s" {
								key         = %[2]q
								type        = "%[1]s"
								description = "%[3]s"
							}`, attributeType, testAccUserAttributeKey(), description),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(resourceName, "type", attributeType),
							resource.TestCheckResourceAttr(
								resourceName, "description", description),
							resource.TestCheckResourceAttrSet(resourceName, "id"),
							resource.TestCheckResourceAttrSet(resourceName, "resource_id"),
						),
					},
				},
			})
		})
	}
}

func TestAccUserAttributeDataSource(t *testing.T) {
	const (
		sourceName = "permitio_user_attribute.source"
		dataName   = "data.permitio_user_attribute.by_key"
	)
	key := testAccUserAttributeKey()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
					resource "permitio_user_attribute" "source" {
						key         = %q
						type        = "number"
						description = "data source acceptance test"
					}

					data "permitio_user_attribute" "by_key" {
						key = permitio_user_attribute.source.key
					}`, key),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataName, "key", key),
					resource.TestCheckResourceAttr(dataName, "type", "number"),
					resource.TestCheckResourceAttr(dataName, "description",
						"data source acceptance test"),
					resource.TestCheckResourceAttrSet(dataName, "environment_id"),
					resource.TestCheckResourceAttrPair(dataName, "id", sourceName, "id"),
					resource.TestCheckResourceAttrPair(
						dataName, "resource_id", sourceName, "resource_id"),
				),
			},
		},
	})
}

// TestUserAttributeNotFoundErrorPattern checks the pattern against Terraform's output
// for the data source, captured from a fake API: a 404 must match, and an auth
// failure, which the old pattern also matched, must not.
func TestUserAttributeNotFoundErrorPattern(t *testing.T) {
	const notFound = `Error running pre-apply plan: exit status 1

Error: Unable to read user attribute

  with data.permitio_user_attribute.missing,
  on terraform_plugin_test.tf line 14, in data "permitio_user_attribute" "missing":
  14: 		data "permitio_user_attribute" "missing" {

Unable to read user attribute with key tfacc_1234567890123456789: 404 Not
Found`
	const unauthorized = `Error running pre-apply plan: exit status 1

Error: Unable to read user attribute

  with data.permitio_user_attribute.missing,
  on terraform_plugin_test.tf line 14, in data "permitio_user_attribute" "missing":
  14: 		data "permitio_user_attribute" "missing" {

Unable to read user attribute with key tfacc_1234567890123456789: ErrorCode:
ContextError, ErrorType: general_error, Message: The context is missing or
invalid - The access for this object is not authorized using the provided API
key, make sure you have the right permissions with the right API key`

	if !userAttributeNotFoundError.MatchString(notFound) {
		t.Errorf("pattern %q does not match the 404 error", userAttributeNotFoundError)
	}
	if userAttributeNotFoundError.MatchString(unauthorized) {
		t.Errorf("pattern %q matches the auth error", userAttributeNotFoundError)
	}
}

func TestAccUserAttributeDataSourceNotFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
					data "permitio_user_attribute" "missing" {
						key = %q
					}`, testAccUserAttributeKey()),
				ExpectError: userAttributeNotFoundError,
			},
		},
	})
}
