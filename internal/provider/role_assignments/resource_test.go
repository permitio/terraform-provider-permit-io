package role_assignments_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
)

var providerFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"permitio": providerserver.NewProtocol6WithError(provider.New("test")()),
}

const aliceRolesPath = "/v2/facts/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID +
	"/users/alice/roles"

// TestRoleAssignmentCreateImportDestroy runs permitio_role_assignment through
// Terraform against the mock Permit API and checks the exact bodies the provider
// sends to assign and unassign the role. Every attribute of an assignment forces a
// replacement, so the second step changes the role and checks that the provider
// unassigns the old one and assigns the new one. The import reads the assignment
// back as user:role:tenant. The user is added to the mock, since the provider
// cannot create one, and is the only object left after destroy.
func TestRoleAssignmentCreateImportDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Roles, mockpermit.Tenants, mockpermit.RoleAssignments)
	m.AddUser(`{"key": "alice", "email": "alice@example.com"}`)
	const address = "permitio_role_assignment.alice"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			m.CheckStored("users/alice"),
			m.CheckRequests(http.MethodDelete, aliceRolesPath,
				`{"role": "editor", "tenant": "acme"}`,
				`{"role": "viewer", "tenant": "acme"}`),
		),
		Steps: []resource.TestStep{
			{
				Config: assignmentConfig("editor"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// The assignment depends on both roles and the tenant, so the mock
					// makes it after them and the user.
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(5)),
					resource.TestCheckResourceAttr(address, "user", "alice"),
					resource.TestCheckResourceAttr(address, "role", "editor"),
					resource.TestCheckResourceAttr(address, "tenant", "acme"),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					resource.TestCheckResourceAttr(address, "created_at",
						"2026-01-01 00:00:00 +0000 UTC"),
					m.CheckRequests(http.MethodPost, aliceRolesPath,
						`{"role": "editor", "tenant": "acme"}`),
				),
			},
			{
				Config: assignmentConfig("viewer"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address,
							plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(6)),
					resource.TestCheckResourceAttr(address, "role", "viewer"),
					m.CheckRequests(http.MethodDelete, aliceRolesPath,
						`{"role": "editor", "tenant": "acme"}`),
					m.CheckRequests(http.MethodPost, aliceRolesPath,
						`{"role": "editor", "tenant": "acme"}`,
						`{"role": "viewer", "tenant": "acme"}`),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     "alice:viewer:acme",
				ImportStateVerify: true,
			},
		},
	})

	m.AssertRoutesHit(mockpermit.RoleAssignments)
}

// TestRoleAssignmentIgnoresInstanceAssignments assigns the viewer role of documents
// to alice on 150 documents in acme before Terraform assigns her the top-level
// viewer role in acme. The list of her viewer assignments in acme then starts with
// the 150 on documents and has the tenant-level one on page 2 of 100. A refresh
// must read the tenant-level assignment, keep its ID and plan nothing. Once it is
// deleted outside Terraform, the refresh must not take an assignment on a document
// for it: it removes it from state and plans to make it again. Destroy unassigns
// the tenant-level role only and leaves the assignments on documents.
func TestRoleAssignmentIgnoresInstanceAssignments(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceRoles, mockpermit.Roles,
		mockpermit.Tenants, mockpermit.ResourceInstances, mockpermit.RoleAssignments)
	m.AddUser(`{"key": "alice"}`)
	const address = "permitio_role_assignment.alice"
	kept := []string{"users/alice"}
	sameID := statecheck.CompareValue(compare.ValuesSame())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy: func(state *terraform.State) error {
			return m.CheckStored(kept...)(state)
		},
		Steps: []resource.TestStep{
			{Config: viewerRolesConfig},
			{
				PreConfig: func() {
					kept = append(kept,
						m.AddInstanceRoleAssignments("alice", "viewer", "document", "acme", 150)...)
				},
				Config: viewerRolesConfig + tenantViewerConfig,
				ConfigStateChecks: []statecheck.StateCheck{
					sameID.AddStateValue(address, tfjsonpath.New("id")),
				},
			},
			{
				Config: viewerRolesConfig + tenantViewerConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					sameID.AddStateValue(address, tfjsonpath.New("id")),
				},
			},
			{
				PreConfig: func() {
					m.DeleteStored("role_assignments/alice:viewer:acme:")
				},
				Config: viewerRolesConfig + tenantViewerConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

// viewerRolesConfig is the top-level viewer role, a document resource with its own
// viewer role, and the acme tenant.
const viewerRolesConfig = `
resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A text document"
  urn         = "prn:test:document"
  actions = {
    read = { name = "Read" }
  }
}

resource "permitio_role" "document_viewer" {
  key         = "viewer"
  name        = "Document viewer"
  description = "Reads a document"
  resource    = permitio_resource.document.key
  permissions = ["read"]
  extends     = []
}

resource "permitio_role" "viewer" {
  key         = "viewer"
  name        = "Viewer"
  description = "Reads documents"
  permissions = []
  extends     = []
}

resource "permitio_tenant" "acme" {
  key         = "acme"
  name        = "Acme"
  description = "The acme tenant"
}
`

// tenantViewerConfig assigns the top-level viewer role to alice in acme.
const tenantViewerConfig = `
resource "permitio_role_assignment" "alice" {
  user   = "alice"
  role   = permitio_role.viewer.key
  tenant = permitio_tenant.acme.key
}
`

// assignmentConfig returns the editor and viewer roles, the acme tenant, and the
// assignment of role to alice in acme. The assignment depends on both roles, so the
// mock always makes it last and its ID does not depend on the order Terraform picks.
func assignmentConfig(role string) string {
	return fmt.Sprintf(`
resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  description = "Edits documents"
  permissions = []
  extends     = []
}

resource "permitio_role" "viewer" {
  key         = "viewer"
  name        = "Viewer"
  description = "Reads documents"
  permissions = []
  extends     = []
}

resource "permitio_tenant" "acme" {
  key         = "acme"
  name        = "Acme"
  description = "The acme tenant"
}

resource "permitio_role_assignment" "alice" {
  user   = "alice"
  role   = permitio_role.%s.key
  tenant = permitio_tenant.acme.key

  depends_on = [permitio_role.editor, permitio_role.viewer]
}
`, role)
}
