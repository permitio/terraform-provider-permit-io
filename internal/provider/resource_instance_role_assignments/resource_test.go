package resource_instance_role_assignments_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

const (
	factsPath           = "/v2/facts/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID
	aliceRolesPath      = factsPath + "/users/alice/roles"
	roleAssignmentsPath = factsPath + "/role_assignments"
)

const (
	readerBody = `{"role": "reader", "tenant": "acme", "resource_instance": "document:handbook"}`
	editorBody = `{"role": "editor", "tenant": "acme", "resource_instance": "document:handbook"}`
)

// TestResourceInstanceRoleAssignmentCreateImportDestroy runs
// permitio_resource_instance_role_assignment through Terraform against the mock
// Permit API and checks the exact bodies the provider sends to assign and unassign
// the role on the instance. Every attribute of an assignment forces a replacement,
// so the second step changes the role and checks that the provider unassigns the
// old one and assigns the new one. The import reads the assignment back as
// user:role:resource:resource_instance:tenant. The user is added to the mock, since
// the provider cannot create one, and is the only object left after destroy.
func TestResourceInstanceRoleAssignmentCreateImportDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceRoles, mockpermit.Tenants,
		mockpermit.ResourceInstances, mockpermit.RoleAssignments)
	m.AddUser(`{"key": "alice", "email": "alice@example.com"}`)
	const address = "permitio_resource_instance_role_assignment.alice"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			m.CheckStored("users/alice"),
			m.CheckRequests(http.MethodDelete, aliceRolesPath,
				readerBody,
				editorBody),
		),
		Steps: []resource.TestStep{
			{
				Config: instanceAssignmentConfig("reader"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// The assignment depends on everything else, so the mock makes it
					// last: after the user, the resource and its action, the two roles,
					// the tenant and the instance.
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(8)),
					resource.TestCheckResourceAttr(address, "user", "alice"),
					resource.TestCheckResourceAttr(address, "role", "reader"),
					resource.TestCheckResourceAttr(address, "resource", "document"),
					resource.TestCheckResourceAttr(address, "resource_instance", "handbook"),
					resource.TestCheckResourceAttr(address, "tenant", "acme"),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					resource.TestCheckResourceAttr(address, "created_at",
						"2026-01-01 00:00:00 +0000 UTC"),
					m.CheckRequests(http.MethodPost, aliceRolesPath,
						readerBody),
				),
			},
			{
				Config: instanceAssignmentConfig("editor"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address,
							plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(9)),
					resource.TestCheckResourceAttr(address, "role", "editor"),
					resource.TestCheckResourceAttr(address, "resource_instance", "handbook"),
					m.CheckRequests(http.MethodDelete, aliceRolesPath,
						readerBody),
					m.CheckRequests(http.MethodPost, aliceRolesPath,
						readerBody,
						editorBody),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     "alice:editor:document:handbook:acme",
				ImportStateVerify: true,
			},
		},
	})

	m.AssertRoutesHit(mockpermit.RoleAssignments)
}

// TestResourceInstanceRoleAssignmentReadsEveryPage makes the assignment after 250
// others of the reader role to alice in acme, which puts it on the third page of
// 100. A refresh must find it there and plan nothing. Once it is deleted outside
// Terraform, the refresh reads up to the empty page after the last, removes it from
// state and plans to make it again.
func TestResourceInstanceRoleAssignmentReadsEveryPage(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceRoles, mockpermit.Tenants,
		mockpermit.ResourceInstances, mockpermit.RoleAssignments)
	m.AddUser(`{"key": "alice"}`)
	const address = "permitio_resource_instance_role_assignment.alice"
	kept := []string{"users/alice"}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy: func(state *terraform.State) error {
			return m.CheckStored(kept...)(state)
		},
		Steps: []resource.TestStep{
			{Config: handbookConfig},
			{
				PreConfig: func() {
					kept = append(kept,
						m.AddInstanceRoleAssignments("alice", "reader", "document", "acme", 250)...)
				},
				Config: instanceAssignmentConfig("reader"),
			},
			{
				Config: instanceAssignmentConfig("reader"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: checkListedPages(m, 3),
			},
			{
				PreConfig: func() {
					m.DeleteStored("role_assignments/alice:reader:acme:document:handbook")
				},
				Config: instanceAssignmentConfig("reader"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionCreate),
					},
				},
				Check: checkListedPages(m, 4),
			},
		},
	})
}

// checkListedPages returns a check that passes when the provider has listed alice's
// assignments, 100 a page, up to page last and never past it.
func checkListedPages(m *mockpermit.Server, last int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		listed := map[string]bool{}
		for _, request := range m.Requests(http.MethodGet, roleAssignmentsPath) {
			query, err := url.ParseQuery(request.RawQuery)
			if err != nil {
				return fmt.Errorf("GET %s?%s: %w", roleAssignmentsPath, request.RawQuery, err)
			}
			if query.Get("per_page") != "100" {
				return fmt.Errorf("GET %s?%s: per_page is not 100", roleAssignmentsPath,
					request.RawQuery)
			}
			listed[query.Get("page")] = true
		}
		if !listed[strconv.Itoa(last)] || listed[strconv.Itoa(last+1)] {
			return fmt.Errorf("listed pages %v, want pages up to %d", listed, last)
		}
		return nil
	}
}

// instanceAssignmentConfig returns handbookConfig and the assignment of role on the
// handbook to alice. The assignment depends on both roles, so the mock always makes
// it last and its ID does not depend on the order Terraform picks.
func instanceAssignmentConfig(role string) string {
	return handbookConfig + fmt.Sprintf(`
resource "permitio_resource_instance_role_assignment" "alice" {
  user              = "alice"
  role              = permitio_role.%s.key
  resource          = permitio_resource.document.key
  resource_instance = permitio_resource_instance.handbook.key
  tenant            = permitio_tenant.acme.key

  depends_on = [permitio_role.reader, permitio_role.editor]
}
`, role)
}

// handbookConfig is a document resource with reader and editor roles, the acme
// tenant and the handbook document of acme.
const handbookConfig = `
resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A text document"
  urn         = "prn:test:document"
  actions = {
    read = { name = "Read" }
  }
}

resource "permitio_role" "reader" {
  key         = "reader"
  name        = "Reader"
  description = "Reads a document"
  resource    = permitio_resource.document.key
  permissions = ["read"]
  extends     = []
}

resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  description = "Edits a document"
  resource    = permitio_resource.document.key
  permissions = ["read"]
  extends     = []
}

resource "permitio_tenant" "acme" {
  key         = "acme"
  name        = "Acme"
  description = "The acme tenant"
}

resource "permitio_resource_instance" "handbook" {
  key      = "handbook"
  resource = permitio_resource.document.key
  tenant   = permitio_tenant.acme.key
}
`
