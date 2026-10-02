package roles_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// TestRoleDataSourceRead reads a top-level role and a role of a resource that the
// mock Permit API serves through the permitio_role data source, by key and, for
// the resource role, resource, and checks every attribute it exports.
func TestRoleDataSourceRead(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.Roles, mockpermit.ResourceRoles)
	const (
		editor = "data.permitio_role.editor"
		reader = "data.permitio_role.reader"
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: documentConfig + `
resource "permitio_role" "viewer" {
  key         = "viewer"
  name        = "Viewer"
  description = "Reads documents"
  permissions = ["${permitio_resource.document.key}:read"]
  extends     = []
}

resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  description = "Edits documents"
  permissions = ["${permitio_resource.document.key}:write"]
  extends     = [permitio_role.viewer.key]
}

resource "permitio_role" "reader" {
  key         = "reader"
  name        = "Reader"
  description = "Reads a document"
  resource    = permitio_resource.document.key
  permissions = ["read"]
  extends     = []
}

data "permitio_role" "editor" {
  key = permitio_role.editor.key
}

data "permitio_role" "reader" {
  key      = permitio_role.reader.key
  resource = permitio_resource.document.key
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(editor, "id", "permitio_role.editor", "id"),
					resource.TestCheckResourceAttr(editor, "key", "editor"),
					resource.TestCheckResourceAttr(editor, "name", "Editor"),
					resource.TestCheckResourceAttr(editor, "description", "Edits documents"),
					resource.TestCheckResourceAttr(editor, "permissions.#", "1"),
					resource.TestCheckTypeSetElemAttr(editor, "permissions.*", "document:write"),
					resource.TestCheckResourceAttr(editor, "extends.#", "1"),
					resource.TestCheckTypeSetElemAttr(editor, "extends.*", "viewer"),
					resource.TestCheckNoResourceAttr(editor, "resource"),
					resource.TestCheckNoResourceAttr(editor, "resource_id"),
					resource.TestCheckResourceAttr(editor, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(editor, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(editor, "environment_id",
						mockpermit.EnvironmentID),
					resource.TestCheckResourceAttr(editor, "created_at",
						"2026-01-01 00:00:00 +0000 UTC"),
					resource.TestCheckResourceAttr(editor, "updated_at",
						"2026-01-01 00:00:00 +0000 UTC"),

					resource.TestCheckResourceAttrPair(reader, "id", "permitio_role.reader", "id"),
					resource.TestCheckResourceAttr(reader, "key", "reader"),
					resource.TestCheckResourceAttr(reader, "name", "Reader"),
					resource.TestCheckResourceAttr(reader, "description", "Reads a document"),
					resource.TestCheckResourceAttr(reader, "permissions.#", "1"),
					resource.TestCheckTypeSetElemAttr(reader, "permissions.*", "read"),
					resource.TestCheckResourceAttr(reader, "extends.#", "0"),
					resource.TestCheckResourceAttr(reader, "resource", "document"),
					resource.TestCheckResourceAttrPair(reader, "resource_id",
						"permitio_resource.document", "id"),
					resource.TestCheckResourceAttr(reader, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(reader, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(reader, "environment_id",
						mockpermit.EnvironmentID),
					resource.TestCheckResourceAttr(reader, "created_at",
						"2026-01-01 00:00:00 +0000 UTC"),
					resource.TestCheckResourceAttr(reader, "updated_at",
						"2026-01-01 00:00:00 +0000 UTC"),
				),
			},
		},
	})
}
