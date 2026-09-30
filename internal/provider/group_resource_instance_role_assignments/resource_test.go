package group_resource_instance_role_assignments_test

import (
	"fmt"
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
	developerRolesPath = "/v2/schema/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID +
		"/groups/developers/roles"
	editorBody = `{"role": "editor", "resource": "workspace", "resource_instance": "ws-1",
		"tenant": "acme"}`
	viewerBody = `{"role": "viewer", "resource": "workspace", "resource_instance": "ws-1",
		"tenant": "acme"}`
)

// TestGroupResourceInstanceRoleAssignmentCreateImportDestroy runs
// permitio_group_resource_instance_role_assignment through Terraform against the
// mock Permit API and checks the exact bodies the provider sends to assign the role
// to the group and remove it. Every attribute of an assignment forces a replacement,
// so the second step changes the role and checks that the provider removes the old
// one and assigns the new one. The import reads the assignment back as
// group:role:resource:resource_instance:tenant. The provider reads the project and
// environment IDs from the tenant list. The group is added to the mock, since the
// provider cannot create one, and is the only object left after destroy.
func TestGroupResourceInstanceRoleAssignmentCreateImportDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceRoles, mockpermit.Tenants,
		mockpermit.TenantList, mockpermit.ResourceInstances, mockpermit.GroupRoles)
	m.AddGroup("developers", "acme")
	const address = "permitio_group_resource_instance_role_assignment.developers"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			m.CheckStored("groups/developers"),
			m.CheckRequests(http.MethodDelete, developerRolesPath, editorBody, viewerBody),
		),
		Steps: []resource.TestStep{
			{
				Config: groupAssignmentConfig("editor"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(address, "id"),
					resource.TestCheckResourceAttr(address, "group", "developers"),
					resource.TestCheckResourceAttr(address, "role", "editor"),
					resource.TestCheckResourceAttr(address, "resource", "workspace"),
					resource.TestCheckResourceAttr(address, "resource_instance", "ws-1"),
					resource.TestCheckResourceAttr(address, "tenant", "acme"),
					m.CheckRequests(http.MethodPost, developerRolesPath, editorBody),
				),
			},
			{
				Config: groupAssignmentConfig("viewer"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address,
							plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "role", "viewer"),
					m.CheckRequests(http.MethodDelete, developerRolesPath, editorBody),
					m.CheckRequests(http.MethodPost, developerRolesPath, editorBody, viewerBody),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     "developers:viewer:workspace:ws-1:acme",
				ImportStateVerify: true,
				// The import leaves id unset, a known bug (PER-16457), so the check
				// matches the imported assignment by group and does not compare ids.
				ImportStateVerifyIdentifierAttribute: "group",
				ImportStateVerifyIgnore:              []string{"id"},
			},
		},
	})

	m.AssertRoutesHit(mockpermit.GroupRoles, mockpermit.TenantList)
}

// groupAssignmentConfig returns a workspace resource with editor and viewer roles,
// the acme tenant, the ws-1 workspace of acme, and the assignment of role on ws-1 to
// the developers group.
func groupAssignmentConfig(role string) string {
	return fmt.Sprintf(`
resource "permitio_resource" "workspace" {
  key         = "workspace"
  name        = "Workspace"
  description = "A shared workspace"
  urn         = "prn:test:workspace"
  actions = {
    read = { name = "Read" }
  }
}

resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  description = "Edits a workspace"
  resource    = permitio_resource.workspace.key
  permissions = ["read"]
  extends     = []
}

resource "permitio_role" "viewer" {
  key         = "viewer"
  name        = "Viewer"
  description = "Views a workspace"
  resource    = permitio_resource.workspace.key
  permissions = ["read"]
  extends     = []
}

resource "permitio_tenant" "acme" {
  key         = "acme"
  name        = "Acme"
  description = "The acme tenant"
}

resource "permitio_resource_instance" "ws_1" {
  key      = "ws-1"
  resource = permitio_resource.workspace.key
  tenant   = permitio_tenant.acme.key
}

resource "permitio_group_resource_instance_role_assignment" "developers" {
  group             = "developers"
  role              = permitio_role.%s.key
  resource          = permitio_resource.workspace.key
  resource_instance = permitio_resource_instance.ws_1.key
  tenant            = permitio_tenant.acme.key
}
`, role)
}
