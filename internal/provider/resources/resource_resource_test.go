package resources_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
)

var providerFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"permitio": providerserver.NewProtocol6WithError(provider.New("test")()),
}

const resourcesPath = "/v2/schema/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID +
	"/resources"

// TestResourceCreateUpdateDestroy runs permitio_resource through Terraform against
// the mock Permit API and checks the exact bodies the provider sends. The update
// changes every attribute the resource updates in place: name, description, urn,
// an action's name and description, an attribute's type and description, and it
// adds an action and an attribute.
func TestResourceCreateUpdateDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources)
	const address = "permitio_resource.document"
	id := mockpermit.ObjectID

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
    read  = { name = "Read" }
    write = { name = "Write", description = "Change the text" }
  }
  attributes = {
    owner = { type = "string", description = "Who owns the document" }
    pages = { type = "number" }
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", id(1)),
					resource.TestCheckResourceAttr(address, "key", "document"),
					resource.TestCheckResourceAttr(address, "name", "Document"),
					resource.TestCheckResourceAttr(address, "description", "A text document"),
					resource.TestCheckResourceAttr(address, "urn", "prn:test:document"),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					resource.TestCheckResourceAttr(address, "created_at",
						"2026-01-01 00:00:00 +0000 UTC"),
					resource.TestCheckResourceAttr(address, "actions.%", "2"),
					resource.TestCheckResourceAttr(address, "actions.read.id", id(2)),
					resource.TestCheckResourceAttr(address, "actions.read.name", "Read"),
					resource.TestCheckNoResourceAttr(address, "actions.read.description"),
					resource.TestCheckResourceAttr(address, "actions.write.id", id(3)),
					resource.TestCheckResourceAttr(address, "actions.write.description",
						"Change the text"),
					resource.TestCheckResourceAttr(address, "attributes.%", "2"),
					resource.TestCheckResourceAttr(address, "attributes.owner.type", "string"),
					resource.TestCheckResourceAttr(address, "attributes.owner.description",
						"Who owns the document"),
					resource.TestCheckResourceAttr(address, "attributes.pages.type", "number"),
					resource.TestCheckNoResourceAttr(address, "attributes.pages.description"),
					m.CheckRequests(http.MethodPost, resourcesPath, `{
						"key": "document",
						"name": "Document",
						"description": "A text document",
						"urn": "prn:test:document",
						"actions": {
							"read": {"name": "Read"},
							"write": {"name": "Write", "description": "Change the text"}
						},
						"attributes": {
							"owner": {"type": "string", "description": "Who owns the document"},
							"pages": {"type": "number"}
						}
					}`),
				),
			},
			{
				Config: `
resource "permitio_resource" "document" {
  key         = "document"
  name        = "Shared document"
  description = "A document several people edit"
  urn         = "prn:test:shared-document"
  actions = {
    read   = { name = "View" }
    write  = { name = "Write", description = "Change the text or the title" }
    delete = { name = "Delete" }
  }
  attributes = {
    owner  = { type = "json", description = "Who owns the document, and how to reach them" }
    pages  = { type = "number", description = "How many pages it has" }
    labels = { type = "array" }
  }
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", id(1)),
					resource.TestCheckResourceAttr(address, "name", "Shared document"),
					resource.TestCheckResourceAttr(address, "description",
						"A document several people edit"),
					resource.TestCheckResourceAttr(address, "urn", "prn:test:shared-document"),
					resource.TestCheckResourceAttr(address, "actions.%", "3"),
					resource.TestCheckResourceAttr(address, "actions.read.id", id(2)),
					resource.TestCheckResourceAttr(address, "actions.read.name", "View"),
					resource.TestCheckResourceAttr(address, "actions.write.id", id(3)),
					resource.TestCheckResourceAttr(address, "actions.write.description",
						"Change the text or the title"),
					resource.TestCheckResourceAttr(address, "actions.delete.id", id(6)),
					resource.TestCheckResourceAttr(address, "actions.delete.name", "Delete"),
					resource.TestCheckResourceAttr(address, "attributes.%", "3"),
					resource.TestCheckResourceAttr(address, "attributes.owner.type", "json"),
					resource.TestCheckResourceAttr(address, "attributes.pages.description",
						"How many pages it has"),
					resource.TestCheckResourceAttr(address, "attributes.labels.type", "array"),
					m.CheckRequests(http.MethodPatch, resourcesPath+"/document", `{
						"name": "Shared document",
						"description": "A document several people edit",
						"urn": "prn:test:shared-document",
						"actions": {
							"read": {"name": "View"},
							"write": {
								"name": "Write",
								"description": "Change the text or the title"
							},
							"delete": {"name": "Delete"}
						},
						"attributes": {
							"owner": {
								"type": "json",
								"description": "Who owns the document, and how to reach them"
							},
							"pages": {"type": "number", "description": "How many pages it has"},
							"labels": {"type": "array"}
						}
					}`),
				),
			},
		},
	})

	m.AssertAllRoutesHit()
}

// TestResourceWithoutAttributesCreateUpdateDestroy runs a permitio_resource that
// has no attributes block, the most common shape, through Terraform against the
// mock Permit API. The provider sends "attributes": null for it on create and on
// update, and keeps attributes null in state.
func TestResourceWithoutAttributesCreateUpdateDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources)
	const address = "permitio_resource.folder"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: folderConfig("Folder", `list = { name = "List" }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "name", "Folder"),
					resource.TestCheckResourceAttr(address, "actions.%", "1"),
					resource.TestCheckNoResourceAttr(address, "attributes.%"),
					m.CheckRequests(http.MethodPost, resourcesPath, `{
						"key": "folder",
						"name": "Folder",
						"description": "A folder of documents",
						"urn": "prn:test:folder",
						"actions": {"list": {"name": "List"}},
						"attributes": null
					}`),
				),
			},
			{
				Config: folderConfig("Shared folder", `list = { name = "List" }
    move = { name = "Move" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "name", "Shared folder"),
					resource.TestCheckResourceAttr(address, "actions.%", "2"),
					resource.TestCheckNoResourceAttr(address, "attributes.%"),
					m.CheckRequests(http.MethodPatch, resourcesPath+"/folder", `{
						"name": "Shared folder",
						"description": "A folder of documents",
						"urn": "prn:test:folder",
						"actions": {"list": {"name": "List"}, "move": {"name": "Move"}},
						"attributes": null
					}`),
				),
			},
		},
	})

	m.AssertAllRoutesHit()
}

func folderConfig(name, actions string) string {
	return fmt.Sprintf(`
resource "permitio_resource" "folder" {
  key         = "folder"
  name        = %q
  description = "A folder of documents"
  urn         = "prn:test:folder"
  actions = {
    %s
  }
}
`, name, actions)
}

// TestResourceRemovingAttributesClearsThem checks that removing a resource's
// attributes block sends "attributes": {}, which deletes them in Permit, and
// leaves them null in the state. The API keeps the attributes when a PATCH nulls
// them, which failed the apply with an inconsistent result (PER-16604).
func TestResourceRemovingAttributesClearsThem(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources)
	const address = "permitio_resource.document"
	documentConfig := func(attributes string) string {
		return fmt.Sprintf(`
resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A text document"
  actions = {
    read = { name = "Read" }
  }
  %s
}
`, attributes)
	}
	onlyTheResource := func(*terraform.State) error {
		if keys := m.StoredKeys(); !slices.Equal(keys, []string{"resources/document"}) {
			return fmt.Errorf("the mock holds %q, want the resource without attributes", keys)
		}
		return nil
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: documentConfig(`attributes = {
    owner = { type = "string", description = "Who owns the document" }
    pages = { type = "number" }
  }`),
				Check: resource.TestCheckResourceAttr(address, "attributes.%", "2"),
			},
			{
				Config: documentConfig(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(address, "attributes.%"),
					m.CheckRequests(http.MethodPatch, resourcesPath+"/document", `{
						"name": "Document",
						"description": "A text document",
						"actions": {"read": {"name": "Read"}},
						"attributes": {}
					}`),
					onlyTheResource,
				),
			},
		},
	})
}
