package group_resource_instance_role_assignments_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/httpclient"
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
// group:role:resource:resource_instance:tenant, with the group key as its id. The
// provider sends its group role requests to the project and environment of its API
// key's scope, without listing tenants, which the mock does not serve here, and
// through its own HTTP client, which names the provider in the User-Agent. The
// group is added to the mock, since the provider cannot create one, and is the only
// object left after destroy.
func TestGroupResourceInstanceRoleAssignmentCreateImportDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceRoles, mockpermit.Tenants,
		mockpermit.ResourceInstances, mockpermit.GroupRoles)
	m.AddGroup("developers", "acme")
	const address = "permitio_group_resource_instance_role_assignment.developers"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			m.CheckStored("groups/developers"),
			m.CheckRequests(http.MethodDelete, developerRolesPath, editorBody, viewerBody),
			checkUserAgent(m, developerRolesPath),
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
			},
		},
	})

	m.AssertRoutesHit(mockpermit.GroupRoles)
}

// checkUserAgent returns a check that passes when every POST, GET and DELETE
// request to urlPath, and at least one of each, has the provider's User-Agent.
func checkUserAgent(m *mockpermit.Server, urlPath string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		want := httpclient.UserAgent("test")
		for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
			requests := m.Requests(method, urlPath)
			if len(requests) == 0 {
				return fmt.Errorf("no %s %s request was sent", method, urlPath)
			}
			for _, request := range requests {
				if got := request.Header.Get("User-Agent"); got != want {
					return fmt.Errorf("%s %s: User-Agent = %q, want %q", method, urlPath, got, want)
				}
			}
		}
		return nil
	}
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
