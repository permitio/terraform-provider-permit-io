package roles_test

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
)

var providerFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"permitio": providerserver.NewProtocol6WithError(provider.New("test")()),
}

const (
	schemaPath        = "/v2/schema/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID
	rolesPath         = schemaPath + "/roles"
	resourceRolesPath = schemaPath + "/resources/document/roles"
)

// The document resource the roles grant permissions on. The second version adds
// the delete action that the editor role is then given.
const (
	documentConfig = `
resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A text document"
  urn         = "prn:test:document"
  actions = {
    read  = { name = "Read" }
    write = { name = "Write" }
  }
}
`
	documentWithDeleteConfig = `
resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A text document"
  urn         = "prn:test:document"
  actions = {
    read   = { name = "Read" }
    write  = { name = "Write" }
    delete = { name = "Delete" }
  }
}
`
)

// The viewer and reviewer roles, which no step changes: the editor role extends
// one of them, then the other. The resource version scopes them to the document.
const (
	viewerAndReviewerConfig = `
resource "permitio_role" "viewer" {
  key         = "viewer"
  name        = "Viewer"
  description = "Reads documents"
  permissions = ["${permitio_resource.document.key}:read"]
  extends     = []
}

resource "permitio_role" "reviewer" {
  key         = "reviewer"
  name        = "Reviewer"
  description = "Reviews documents"
  permissions = ["${permitio_resource.document.key}:read"]
  extends     = []
}
`
	resourceViewerAndReviewerConfig = `
resource "permitio_role" "viewer" {
  key         = "viewer"
  name        = "Viewer"
  description = "Reads documents"
  resource    = permitio_resource.document.key
  permissions = ["read"]
  extends     = []
}

resource "permitio_role" "reviewer" {
  key         = "reviewer"
  name        = "Reviewer"
  description = "Reviews documents"
  resource    = permitio_resource.document.key
  permissions = ["read"]
  extends     = []
}
`
)

// TestRoleCreateUpdateImportDestroy runs a top-level permitio_role through
// Terraform against the mock Permit API. The update changes the name and
// description, swaps the role it extends, and removes one permission and adds
// another; the import reads the role back by key.
func TestRoleCreateUpdateImportDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.Roles)
	const address = "permitio_role.editor"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: documentConfig + viewerAndReviewerConfig + `
resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  description = "Edits documents"
  permissions = [
    "${permitio_resource.document.key}:read",
    "${permitio_resource.document.key}:write",
  ]
  extends = [permitio_role.viewer.key]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(address, "id"),
					resource.TestCheckResourceAttr(address, "key", "editor"),
					resource.TestCheckResourceAttr(address, "name", "Editor"),
					resource.TestCheckResourceAttr(address, "description", "Edits documents"),
					resource.TestCheckTypeSetElemAttr(address, "permissions.*", "document:read"),
					resource.TestCheckTypeSetElemAttr(address, "permissions.*", "document:write"),
					resource.TestCheckResourceAttr(address, "permissions.#", "2"),
					resource.TestCheckResourceAttr(address, "extends.#", "1"),
					resource.TestCheckTypeSetElemAttr(address, "extends.*", "viewer"),
					resource.TestCheckNoResourceAttr(address, "resource"),
					resource.TestCheckNoResourceAttr(address, "resource_id"),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					m.CheckRequests(http.MethodPost, rolesPath,
						`{"key": "viewer", "name": "Viewer", "description": "Reads documents",
						  "permissions": ["document:read"], "extends": []}`,
						`{"key": "reviewer", "name": "Reviewer", "description": "Reviews documents",
						  "permissions": ["document:read"], "extends": []}`,
						`{"key": "editor", "name": "Editor", "description": "Edits documents",
						  "permissions": ["document:read", "document:write"],
						  "extends": ["viewer"]}`,
					),
				),
			},
			{
				Config: documentWithDeleteConfig + viewerAndReviewerConfig + `
resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Chief editor"
  description = "Edits and deletes documents"
  permissions = [
    "${permitio_resource.document.key}:write",
    "${permitio_resource.document.key}:delete",
  ]
  extends = [permitio_role.reviewer.key]
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("permitio_role.viewer",
							plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "name", "Chief editor"),
					resource.TestCheckResourceAttr(address, "description",
						"Edits and deletes documents"),
					resource.TestCheckResourceAttr(address, "permissions.#", "2"),
					resource.TestCheckTypeSetElemAttr(address, "permissions.*", "document:write"),
					resource.TestCheckTypeSetElemAttr(address, "permissions.*", "document:delete"),
					resource.TestCheckResourceAttr(address, "extends.#", "1"),
					resource.TestCheckTypeSetElemAttr(address, "extends.*", "reviewer"),
					m.CheckRequests(http.MethodPatch, rolesPath+"/editor",
						`{"name": "Chief editor", "description": "Edits and deletes documents",
						  "extends": ["reviewer"]}`),
					m.CheckRequests(http.MethodDelete, rolesPath+"/editor/permissions",
						`{"permissions": ["document:read"]}`),
					m.CheckRequests(http.MethodPost, rolesPath+"/editor/permissions",
						`{"permissions": ["document:delete"]}`),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     "editor",
				ImportStateVerify: true,
			},
		},
	})

	m.AssertAllRoutesHit()
}

// TestResourceRoleCreateUpdateImportDestroy runs a permitio_role scoped to a
// resource through Terraform against the mock Permit API. The update changes the
// name and description, swaps the role it extends, and removes one permission and
// adds another; the import reads the role back as resource:role.
func TestResourceRoleCreateUpdateImportDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceRoles)
	const address = "permitio_role.editor"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: documentConfig + resourceViewerAndReviewerConfig + `
resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  description = "Edits documents"
  resource    = permitio_resource.document.key
  permissions = ["read", "write"]
  extends     = [permitio_role.viewer.key]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(address, "id"),
					resource.TestCheckResourceAttr(address, "key", "editor"),
					resource.TestCheckResourceAttr(address, "name", "Editor"),
					resource.TestCheckResourceAttr(address, "description", "Edits documents"),
					resource.TestCheckResourceAttr(address, "resource", "document"),
					resource.TestCheckResourceAttrPair(address, "resource_id",
						"permitio_resource.document", "id"),
					resource.TestCheckResourceAttr(address, "permissions.#", "2"),
					resource.TestCheckTypeSetElemAttr(address, "permissions.*", "read"),
					resource.TestCheckTypeSetElemAttr(address, "permissions.*", "write"),
					resource.TestCheckResourceAttr(address, "extends.#", "1"),
					resource.TestCheckTypeSetElemAttr(address, "extends.*", "viewer"),
					m.CheckRequests(http.MethodPost, resourceRolesPath,
						`{"key": "viewer", "name": "Viewer", "description": "Reads documents",
						  "permissions": ["read"], "extends": []}`,
						`{"key": "reviewer", "name": "Reviewer", "description": "Reviews documents",
						  "permissions": ["read"], "extends": []}`,
						`{"key": "editor", "name": "Editor", "description": "Edits documents",
						  "permissions": ["read", "write"], "extends": ["viewer"]}`,
					),
				),
			},
			{
				Config: documentWithDeleteConfig + resourceViewerAndReviewerConfig + `
resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Chief editor"
  description = "Edits and deletes documents"
  resource    = permitio_resource.document.key
  permissions = ["write", "delete"]
  extends     = [permitio_role.reviewer.key]
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("permitio_role.viewer",
							plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "name", "Chief editor"),
					resource.TestCheckResourceAttr(address, "description",
						"Edits and deletes documents"),
					resource.TestCheckResourceAttr(address, "permissions.#", "2"),
					resource.TestCheckTypeSetElemAttr(address, "permissions.*", "write"),
					resource.TestCheckTypeSetElemAttr(address, "permissions.*", "delete"),
					resource.TestCheckResourceAttr(address, "extends.#", "1"),
					resource.TestCheckTypeSetElemAttr(address, "extends.*", "reviewer"),
					m.CheckRequests(http.MethodPatch, resourceRolesPath+"/editor",
						`{"name": "Chief editor", "description": "Edits and deletes documents",
						  "extends": ["reviewer"]}`),
					m.CheckRequests(http.MethodDelete, resourceRolesPath+"/editor/permissions",
						`{"permissions": ["read"]}`),
					m.CheckRequests(http.MethodPost, resourceRolesPath+"/editor/permissions",
						`{"permissions": ["delete"]}`),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     "document:editor",
				ImportStateVerify: true,
			},
		},
	})

	m.AssertAllRoutesHit()
}
