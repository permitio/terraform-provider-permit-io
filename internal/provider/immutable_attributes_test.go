package provider

import (
	"fmt"
	"maps"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// immutableAttributesCase is a resource type with attributes the Permit API cannot
// change in place, while it updates the others: a key names the object in the
// update URL, a PATCH cannot move a resource set to another resource, and the API
// has no update for a relation.
type immutableAttributesCase struct {
	resourceType string
	routes       []mockpermit.Routes
	// config returns the objects the resource under test refers to, and that
	// resource, named "this", with its attributes set from values.
	config func(values map[string]string) string
	// base are the attribute values of the first step.
	base map[string]string
	// changes are the values each later step sets, one immutable attribute at a
	// time.
	changes []attributeChange
}

// TestImmutableAttributesForceReplacement changes each attribute the Permit API
// cannot change in place, one at a time, and checks that Terraform plans to
// replace the object and that the apply against the mock gives it the new value.
func TestImmutableAttributesForceReplacement(t *testing.T) {
	for _, c := range immutableAttributesCases() {
		t.Run(c.resourceType, func(t *testing.T) {
			m := mockpermit.New(t, c.routes...)
			address := c.resourceType + ".this"
			values := maps.Clone(c.base)
			steps := []resource.TestStep{{Config: c.config(values)}}
			for _, change := range c.changes {
				values = maps.Clone(values)
				values[change.attribute] = change.value
				steps = append(steps, resource.TestStep{
					Config: c.config(values),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(address,
								plancheck.ResourceActionDestroyBeforeCreate),
						},
					},
					Check: resource.TestCheckResourceAttr(address, change.attribute,
						change.value),
				})
			}

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             m.CheckEmpty,
				Steps:                    steps,
			})
		})
	}
}

func immutableAttributesCases() []immutableAttributesCase {
	return []immutableAttributesCase{
		{
			resourceType: "permitio_resource",
			routes:       []mockpermit.Routes{mockpermit.Resources},
			config: func(v map[string]string) string {
				return fmt.Sprintf(`
resource "permitio_resource" "this" {
  key  = %q
  name = "Document"
  actions = {
    read = { name = "Read" }
  }
}
`, v["key"])
			},
			base:    map[string]string{"key": "document"},
			changes: []attributeChange{{"key", "file"}},
		},
		{
			resourceType: "permitio_user_set",
			routes:       []mockpermit.Routes{mockpermit.ConditionSets},
			config: func(v map[string]string) string {
				return fmt.Sprintf(`
resource "permitio_user_set" "this" {
  key  = %q
  name = "Reviewers"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.team" = { equals = "review" } }] }]
  })
}
`, v["key"])
			},
			base:    map[string]string{"key": "reviewers"},
			changes: []attributeChange{{"key", "editors"}},
		},
		{
			resourceType: "permitio_resource_set",
			routes:       []mockpermit.Routes{mockpermit.Resources, mockpermit.ConditionSets},
			config: func(v map[string]string) string {
				return documentAndFolderConfig + fmt.Sprintf(`
resource "permitio_resource_set" "this" {
  key      = %q
  name     = "Drafts"
  resource = permitio_resource.%s.key
  conditions = jsonencode({
    allOf = [{ allOf = [{ "resource.status" = { equals = "draft" } }] }]
  })
}
`, v["key"], v["resource"])
			},
			base: map[string]string{"key": "drafts", "resource": "document"},
			changes: []attributeChange{
				{"key", "unpublished"}, {"resource", "folder"},
			},
		},
		{
			resourceType: "permitio_proxy_config",
			routes:       []mockpermit.Routes{mockpermit.ProxyConfigs},
			config: func(v map[string]string) string {
				return fmt.Sprintf(`
resource "permitio_proxy_config" "this" {
  key            = %q
  name           = "Billing API"
  auth_mechanism = "Bearer"
  auth_secret = {
    bearer = "example-bearer-token"
  }
  mapping_rules = [
    {
      url         = "https://billing.example.com/v1/invoices"
      http_method = "get"
      resource    = "invoice"
    },
  ]
}
`, v["key"])
			},
			base:    map[string]string{"key": "billing"},
			changes: []attributeChange{{"key", "payments"}},
		},
		{
			resourceType: "permitio_user_attribute",
			routes:       []mockpermit.Routes{mockpermit.ResourceAttributes},
			config: func(v map[string]string) string {
				return fmt.Sprintf(`
resource "permitio_user_attribute" "this" {
  key  = %q
  type = "string"
}
`, v["key"])
			},
			base:    map[string]string{"key": "department"},
			changes: []attributeChange{{"key", "team"}},
		},
		{
			resourceType: "permitio_relation",
			routes:       []mockpermit.Routes{mockpermit.Resources, mockpermit.ResourceRelations},
			config: func(v map[string]string) string {
				return documentAndFolderConfig + fmt.Sprintf(`
resource "permitio_relation" "this" {
  key              = "parent"
  name             = %q
  description      = %q
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.document.key
}
`, v["name"], v["description"])
			},
			base: map[string]string{
				"name": "Parent folder", "description": "The folder that holds the document",
			},
			changes: []attributeChange{
				{"name", "Containing folder"}, {"description", "The folder the document is in"},
			},
		},
	}
}

// documentAndFolderConfig is two resources that resource sets and relations refer
// to.
const documentAndFolderConfig = `
resource "permitio_resource" "document" {
  key  = "document"
  name = "Document"
  actions = {
    read = { name = "Read" }
  }
}

resource "permitio_resource" "folder" {
  key  = "folder"
  name = "Folder"
  actions = {
    list = { name = "List" }
  }
}
`
