package role_derivations_test

import (
	"fmt"
	"net/http"
	"regexp"
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
	implicitGrantsPath = "/v2/schema/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID +
		"/resources/file/roles/editor/implicit_grants"
	managerBody = `{"role": "manager", "on_resource": "folder", "linked_by_relation": "parent"}`
	ownerBody   = `{"role": "owner", "on_resource": "folder", "linked_by_relation": "parent"}`
)

// TestRoleDerivationCreateDestroy runs permitio_role_derivation through Terraform
// against the mock Permit API and checks the exact bodies the provider sends to
// create and delete it. Two derivations grant the same file role, from the
// manager and the owner of the file's folder, so each one must read back its own
// grant. Every attribute of a derivation forces a replacement, so the second step
// changes the role they grant instead and checks that they are left alone.
func TestRoleDerivationCreateDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceRoles,
		mockpermit.ResourceRelations, mockpermit.ImplicitGrants)
	const (
		manager = "permitio_role_derivation.manager_edits_files"
		owner   = "permitio_role_derivation.owner_edits_files"
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			m.CheckEmpty,
			m.CheckRequests(http.MethodDelete, implicitGrantsPath, managerBody, ownerBody),
		),
		Steps: []resource.TestStep{
			{
				Config: derivationConfig("Edits a file", `["read", "write"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(manager, "resource", "file"),
					resource.TestCheckResourceAttr(manager, "to_role", "editor"),
					resource.TestCheckResourceAttr(manager, "on_resource", "folder"),
					resource.TestCheckResourceAttr(manager, "role", "manager"),
					resource.TestCheckResourceAttr(manager, "linked_by", "parent"),
					resource.TestCheckResourceAttr(owner, "resource", "file"),
					resource.TestCheckResourceAttr(owner, "to_role", "editor"),
					resource.TestCheckResourceAttr(owner, "on_resource", "folder"),
					resource.TestCheckResourceAttr(owner, "role", "owner"),
					resource.TestCheckResourceAttr(owner, "linked_by", "parent"),
					m.CheckRequests(http.MethodPost, implicitGrantsPath, managerBody, ownerBody),
				),
			},
			{
				Config: derivationConfig("Edits and shares a file", `["write", "share"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("permitio_role.editor",
							plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("permitio_resource.file",
							plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction(manager, plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction(owner, plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(manager, "role", "manager"),
					resource.TestCheckResourceAttr(owner, "role", "owner"),
					m.CheckRequests(http.MethodPost, implicitGrantsPath, managerBody, ownerBody),
					m.CheckRequests(http.MethodDelete, implicitGrantsPath),
				),
			},
		},
	})

	m.AssertRoutesHit(mockpermit.ImplicitGrants)
}

// TestRoleDerivationCreateNotFoundHint checks that when the API answers a
// derivation's create with a 404, here because role is not a role on on_resource,
// the error names the keys to check.
func TestRoleDerivationCreateNotFoundHint(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceRoles,
		mockpermit.ResourceRelations, mockpermit.ImplicitGrants)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: missingRoleConfig,
				ExpectError: regexp.MustCompile(`Check\s+that\s+to_role\s+"editor"\s+is\s+a\s+` +
					`role\s+on\s+resource\s+"file",\s+role\s+"manager"\s+is\s+a\s+role\s+on\s+` +
					`on_resource\s+"folder"`),
			},
		},
	})

	if got := len(m.Requests(http.MethodPost, implicitGrantsPath)); got != 1 {
		t.Errorf("POST %s: got %d requests, want 1", implicitGrantsPath, got)
	}
}

// missingRoleConfig is a derivation from a manager role that the folder resource
// does not have.
const missingRoleConfig = `
resource "permitio_resource" "folder" {
  key     = "folder"
  name    = "Folder"
  actions = {
    list = { name = "List" }
  }
}

resource "permitio_resource" "file" {
  key     = "file"
  name    = "File"
  actions = {
    read = { name = "Read" }
  }
}

resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  resource    = permitio_resource.file.key
  permissions = ["read"]
  extends     = []
}

resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent folder"
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.file.key
}

resource "permitio_role_derivation" "manager_edits_files" {
  resource    = permitio_resource.file.key
  to_role     = permitio_role.editor.key
  on_resource = permitio_resource.folder.key
  role        = "manager"
  linked_by   = permitio_relation.parent.key
}
`

// derivationConfig returns folders with their manager and owner roles, files in
// folders with their editor role, and the two derivations that make a folder's
// manager and owner editors of its files. The editor role gets editorDescription
// and editorPermissions, from the file's read, write and share actions.
func derivationConfig(editorDescription, editorPermissions string) string {
	return fmt.Sprintf(`
resource "permitio_resource" "folder" {
  key         = "folder"
  name        = "Folder"
  description = "A folder of files"
  urn         = "prn:test:folder"
  actions = {
    list = { name = "List" }
  }
}

resource "permitio_role" "manager" {
  key         = "manager"
  name        = "Manager"
  description = "Manages a folder"
  resource    = permitio_resource.folder.key
  permissions = ["list"]
  extends     = []
}

resource "permitio_role" "owner" {
  key         = "owner"
  name        = "Owner"
  description = "Owns a folder"
  resource    = permitio_resource.folder.key
  permissions = ["list"]
  extends     = []
}

resource "permitio_resource" "file" {
  key         = "file"
  name        = "File"
  description = "A file in a folder"
  urn         = "prn:test:file"
  actions = {
    read  = { name = "Read" }
    write = { name = "Write" }
    share = { name = "Share" }
  }
}

resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  description = %q
  resource    = permitio_resource.file.key
  permissions = %s
  extends     = []
}

resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent folder"
  description      = "The folder that holds the file"
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.file.key
}

resource "permitio_role_derivation" "manager_edits_files" {
  resource    = permitio_resource.file.key
  to_role     = permitio_role.editor.key
  on_resource = permitio_resource.folder.key
  role        = permitio_role.manager.key
  linked_by   = permitio_relation.parent.key
}

resource "permitio_role_derivation" "owner_edits_files" {
  resource    = permitio_resource.file.key
  to_role     = permitio_role.editor.key
  on_resource = permitio_resource.folder.key
  role        = permitio_role.owner.key
  linked_by   = permitio_relation.parent.key
}
`, editorDescription, editorPermissions)
}
