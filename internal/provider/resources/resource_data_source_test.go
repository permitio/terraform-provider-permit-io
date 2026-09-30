package resources_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// TestResourceDataSourceRead reads a resource the mock Permit API serves through
// the permitio_resource data source and checks every attribute it exports. The
// data source requires name and actions as inputs, so the configuration repeats
// the resource's own.
func TestResourceDataSourceRead(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources)
	const (
		address = "data.permitio_resource.document"
		created = "permitio_resource.document"
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: `
resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A text document"
  urn         = "prn:test:document"
  actions = {
    read = { name = "Read", description = "Read a document" }
  }
  attributes = {
    pages = { type = "number", description = "How many pages" }
  }
}

data "permitio_resource" "document" {
  key  = permitio_resource.document.key
  name = "Document"
  actions = {
    read = { name = "Read", description = "Read a document" }
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(1)),
					resource.TestCheckResourceAttr(address, "key", "document"),
					resource.TestCheckResourceAttr(address, "name", "Document"),
					resource.TestCheckResourceAttr(address, "description", "A text document"),
					resource.TestCheckResourceAttr(address, "urn", "prn:test:document"),
					resource.TestCheckResourceAttr(address, "actions.%", "1"),
					resource.TestCheckResourceAttrPair(address, "actions.read.id",
						created, "actions.read.id"),
					resource.TestCheckResourceAttr(address, "actions.read.name", "Read"),
					resource.TestCheckResourceAttr(address, "actions.read.description",
						"Read a document"),
					resource.TestCheckResourceAttr(address, "attributes.%", "1"),
					resource.TestCheckResourceAttr(address, "attributes.pages.type", "number"),
					resource.TestCheckResourceAttr(address, "attributes.pages.description",
						"How many pages"),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					resource.TestCheckResourceAttr(address, "created_at",
						"2026-01-01 00:00:00 +0000 UTC"),
					resource.TestCheckResourceAttr(address, "updated_at",
						"2026-01-01 00:00:00 +0000 UTC"),
				),
			},
		},
	})
}
