package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

const (
	mockSchemaPath = "/v2/schema/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID
	mockFactsPath  = "/v2/facts/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID
)

// deletedObjectCase is one resource type in the tests of objects deleted outside
// Terraform: a configuration whose object at address is that resource type, with
// what it depends on, and where the mock keeps the object.
type deletedObjectCase struct {
	name   string
	routes []mockpermit.Routes
	// setup adds the objects the provider cannot create, such as users.
	setup  func(m *mockpermit.Server)
	config string
	// address is the resource that is deleted outside Terraform. Nothing in the
	// configuration depends on it.
	address string
	// stored is the object at address, written "collection/key" as the mock's
	// StoredKeys lists it.
	stored string
	// goneBeforeDestroy is what the delete test removes from the mock, so that the
	// request that deletes the object at address gets a 404. It is stored alone
	// unless the API answers that request with a 404 only when another object is
	// gone too.
	goneBeforeDestroy []string
	// readPath is the path of the GET request that reads the object at address.
	// ${name} in it stands for the object's state attribute name.
	readPath string
	// readsBefore is how many requests to readPath a refresh sends for other
	// objects before the one for the object at address.
	readsBefore int
	// deletePath is the path of the request that deletes the object at address.
	deletePath string
	// kept are the objects setup added, which are left after destroy unless the
	// delete test removes them.
	kept []string
}

// TestReadRemovesObjectsDeletedOutsideTerraform deletes each resource type's object
// in the mock after it is created, and checks that the next plan creates it again
// instead of failing to read it.
func TestReadRemovesObjectsDeletedOutsideTerraform(t *testing.T) {
	for _, c := range deletedObjectCases() {
		t.Run(c.name, func(t *testing.T) {
			m := mockpermit.New(t, c.routes...)
			c.setup(m)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             m.CheckStored(c.kept...),
				Steps: []resource.TestStep{
					{
						Config: c.config,
						Check:  checkMockHolds(m, c.stored),
					},
					{
						PreConfig: func() { m.DeleteStored(c.stored) },
						Config:    c.config,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(c.address,
									plancheck.ResourceActionCreate),
							},
						},
						Check: checkMockHolds(m, c.stored),
					},
				},
			})
		})
	}
}

// TestDeleteSucceedsWhenObjectIsAlreadyGone deletes each resource type's object in
// the mock after it is created, and checks that destroy, which does not refresh
// first, sends the delete, takes the API's 404 as done, and succeeds. The plans
// skip refreshing too, so that the test does not depend on how Read treats a 404.
func TestDeleteSucceedsWhenObjectIsAlreadyGone(t *testing.T) {
	for _, c := range deletedObjectCases() {
		t.Run(c.name, func(t *testing.T) {
			m := mockpermit.New(t, c.routes...)
			c.setup(m)
			gone := c.goneBeforeDestroy
			if gone == nil {
				gone = []string{c.stored}
			}
			kept := slices.DeleteFunc(slices.Clone(c.kept), func(stored string) bool {
				return slices.Contains(gone, stored)
			})

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				AdditionalCLIOptions: &resource.AdditionalCLIOptions{
					Plan: resource.PlanOptions{NoRefresh: true},
				},
				CheckDestroy: resource.ComposeAggregateTestCheckFunc(
					m.CheckStored(kept...),
					checkRequestCount(m, http.MethodDelete, c.deletePath, 1),
				),
				Steps: []resource.TestStep{
					{
						Config: c.config,
						Check: resource.ComposeAggregateTestCheckFunc(
							checkMockHolds(m, c.stored),
							checkRequestCount(m, http.MethodDelete, c.deletePath, 0),
							func(*terraform.State) error {
								m.DeleteStored(gone...)
								return nil
							},
							checkMockLacks(m, gone...),
						),
					},
				},
			})
		})
	}
}

// serverErrorPattern matches the 500 that FailRequests answers with, as the SDK
// and the provider's own HTTP client report it.
var serverErrorPattern = regexp.MustCompile(`500\s+Internal\s+Server\s+Error|status\s+500`)

// TestReadAndDeleteFailOnOtherErrors answers each resource type's read, then its
// delete, with a 500 that carries the body of a 404, and checks that the provider
// takes neither for an object deleted outside Terraform: the refresh and the
// destroy fail, and the object stays in state, so the plans after them are empty.
func TestReadAndDeleteFailOnOtherErrors(t *testing.T) {
	for _, c := range deletedObjectCases() {
		t.Run(c.name, func(t *testing.T) {
			m := mockpermit.New(t, c.routes...)
			c.setup(m)
			var readPath string
			var stop func()

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             m.CheckStored(c.kept...),
				Steps: []resource.TestStep{
					{
						Config: c.config,
						Check: resource.ComposeAggregateTestCheckFunc(
							checkMockHolds(m, c.stored),
							expandStatePath(c.address, c.readPath, &readPath),
						),
					},
					{
						PreConfig: func() {
							stop = m.FailRequests(http.MethodGet, readPath,
								http.StatusInternalServerError, c.readsBefore)
						},
						RefreshState: true,
						ExpectError:  serverErrorPattern,
					},
					{
						PreConfig: func() { stop() },
						Config:    c.config,
						PlanOnly:  true,
					},
					{
						PreConfig: func() {
							stop = m.FailRequests(http.MethodDelete, c.deletePath,
								http.StatusInternalServerError, 0)
						},
						Config:      c.config,
						Destroy:     true,
						ExpectError: serverErrorPattern,
					},
					{
						PreConfig: func() { stop() },
						Config:    c.config,
						PlanOnly:  true,
					},
				},
			})
		})
	}
}

// TestDeletedObjectCasesCoverEveryResource checks that the tests above have a case
// for each resource type the provider serves, and only for those.
func TestDeletedObjectCasesCoverEveryResource(t *testing.T) {
	var served []string
	for _, newResource := range (&PermitProvider{}).Resources(context.Background()) {
		var metadata fwresource.MetadataResponse
		newResource().Metadata(context.Background(),
			fwresource.MetadataRequest{ProviderTypeName: "permitio"}, &metadata)
		served = append(served, metadata.TypeName)
	}
	var covered []string
	for _, c := range deletedObjectCases() {
		resourceType, _, _ := strings.Cut(c.address, ".")
		covered = append(covered, resourceType)
	}
	slices.Sort(served)
	slices.Sort(covered)
	if len(served) == 0 || !slices.Equal(covered, served) {
		t.Errorf("deleted object cases cover %q, want one case for each resource type %q",
			covered, served)
	}
}

// expandStatePath sets expanded to path with each ${name} in it replaced by the
// state attribute name of the resource at address. It fails when the resource or
// an attribute is not in state.
func expandStatePath(address, path string, expanded *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		state, ok := s.RootModule().Resources[address]
		if !ok || state.Primary == nil {
			return fmt.Errorf("%s is not in state", address)
		}
		var missing []string
		*expanded = os.Expand(path, func(name string) string {
			value, ok := state.Primary.Attributes[name]
			if !ok || value == "" {
				missing = append(missing, name)
			}
			return value
		})
		if len(missing) > 0 {
			return fmt.Errorf("%s has no %q in state to expand %s", address, missing, path)
		}
		return nil
	}
}

// checkMockHolds fails unless the mock holds the object stored.
func checkMockHolds(m *mockpermit.Server, stored string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if keys := m.StoredKeys(); !slices.Contains(keys, stored) {
			return fmt.Errorf("the mock does not hold %s; it holds %q", stored, keys)
		}
		return nil
	}
}

// checkMockLacks fails while the mock holds any of the objects gone.
func checkMockLacks(m *mockpermit.Server, gone ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		for _, stored := range m.StoredKeys() {
			if slices.Contains(gone, stored) {
				return fmt.Errorf("the mock still holds %s", stored)
			}
		}
		return nil
	}
}

// checkRequestCount fails unless the mock has received want requests with this
// method and path.
func checkRequestCount(m *mockpermit.Server, method, path string, want int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := len(m.Requests(method, path)); got != want {
			return fmt.Errorf("%s %s: got %d requests, want %d", method, path, got, want)
		}
		return nil
	}
}

func noSetup(*mockpermit.Server) {}

func addAlice(m *mockpermit.Server) {
	m.AddUser(`{"key": "alice", "email": "alice@example.com"}`)
}

// deletedObjectCases returns one case for each of the provider's resource types.
func deletedObjectCases() []deletedObjectCase {
	return []deletedObjectCase{
		{
			name:       "resource",
			routes:     []mockpermit.Routes{mockpermit.Resources},
			setup:      noSetup,
			config:     documentResourceConfig,
			address:    "permitio_resource.document",
			readPath:   mockSchemaPath + "/resources/document",
			stored:     "resources/document",
			deletePath: mockSchemaPath + "/resources/document",
		},
		{
			name:   "role",
			routes: []mockpermit.Routes{mockpermit.Roles},
			setup:  noSetup,
			config: `
resource "permitio_role" "admin" {
  key         = "admin"
  name        = "Admin"
  permissions = []
  extends     = []
}
`,
			address:    "permitio_role.admin",
			readPath:   mockSchemaPath + "/roles/admin",
			stored:     "roles/admin",
			deletePath: mockSchemaPath + "/roles/admin",
		},
		{
			name:       "user set",
			routes:     []mockpermit.Routes{mockpermit.ConditionSets},
			setup:      noSetup,
			config:     reviewersUserSetConfig,
			address:    "permitio_user_set.reviewers",
			readPath:   mockSchemaPath + "/condition_sets/reviewers",
			stored:     "condition_sets/reviewers",
			deletePath: mockSchemaPath + "/condition_sets/reviewers",
		},
		{
			name:       "resource set",
			routes:     []mockpermit.Routes{mockpermit.Resources, mockpermit.ConditionSets},
			setup:      noSetup,
			config:     documentResourceConfig + draftsResourceSetConfig,
			address:    "permitio_resource_set.drafts",
			readPath:   mockSchemaPath + "/condition_sets/drafts",
			stored:     "condition_sets/drafts",
			deletePath: mockSchemaPath + "/condition_sets/drafts",
		},
		{
			name: "condition set rule",
			routes: []mockpermit.Routes{
				mockpermit.Resources, mockpermit.ConditionSets, mockpermit.ConditionSetRules,
			},
			setup: noSetup,
			config: documentResourceConfig + reviewersUserSetConfig + draftsResourceSetConfig + `
resource "permitio_condition_set_rule" "reviewers_read_drafts" {
  user_set     = permitio_user_set.reviewers.key
  permission   = "${permitio_resource.document.key}:read"
  resource_set = permitio_resource_set.drafts.key
}
`,
			address:  "permitio_condition_set_rule.reviewers_read_drafts",
			readPath: mockFactsPath + "/set_rules",
			stored:   "set_rules/reviewers,document:read,drafts",
			// The API skips unassigning a permission that is not granted, and
			// answers 404 when the user set is gone, which takes its rules with it.
			goneBeforeDestroy: []string{
				"set_rules/reviewers,document:read,drafts", "condition_sets/reviewers",
			},
			deletePath: mockFactsPath + "/set_rules",
		},
		{
			name:   "proxy config",
			routes: []mockpermit.Routes{mockpermit.ProxyConfigs},
			setup:  noSetup,
			config: `
resource "permitio_proxy_config" "billing" {
  key            = "billing"
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
      action      = "read"
    },
  ]
}
`,
			address:    "permitio_proxy_config.billing",
			readPath:   mockFactsPath + "/proxy_configs/billing",
			stored:     "proxy_configs/billing",
			deletePath: mockFactsPath + "/proxy_configs/billing",
		},
		{
			name:    "relation",
			routes:  []mockpermit.Routes{mockpermit.Resources, mockpermit.ResourceRelations},
			setup:   noSetup,
			config:  folderAndFileConfig + parentRelationConfig,
			address: "permitio_relation.parent",
			// Read looks the relation up by the ID of its object resource.
			readPath:   mockSchemaPath + "/resources/${object_resource_id}/relations/parent",
			stored:     "relations/file:parent",
			deletePath: mockSchemaPath + "/resources/file/relations/parent",
		},
		{
			name: "role derivation",
			routes: []mockpermit.Routes{
				mockpermit.Resources, mockpermit.ResourceRoles, mockpermit.ResourceRelations,
				mockpermit.ImplicitGrants,
			},
			setup: noSetup,
			config: folderAndFileConfig + parentRelationConfig + `
resource "permitio_role" "manager" {
  key         = "manager"
  name        = "Manager"
  resource    = permitio_resource.folder.key
  permissions = ["list"]
  extends     = []
}

resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  resource    = permitio_resource.file.key
  permissions = ["read"]
  extends     = []
}

resource "permitio_role_derivation" "manager_edits_files" {
  resource    = permitio_resource.file.key
  to_role     = permitio_role.editor.key
  on_resource = permitio_resource.folder.key
  role        = permitio_role.manager.key
  linked_by   = permitio_relation.parent.key
}
`,
			address: "permitio_role_derivation.manager_edits_files",
			// The editor role's own read, which the derivation depends on, comes first.
			readPath:    mockSchemaPath + "/resources/file/roles/editor",
			readsBefore: 1,
			stored:      "implicit_grants/file:editor:folder:manager:parent",
			deletePath:  mockSchemaPath + "/resources/file/roles/editor/implicit_grants",
		},
		{
			name:       "tenant",
			routes:     []mockpermit.Routes{mockpermit.Tenants},
			setup:      noSetup,
			config:     acmeTenantConfig,
			address:    "permitio_tenant.acme",
			readPath:   mockFactsPath + "/tenants/acme",
			stored:     "tenants/acme",
			deletePath: mockFactsPath + "/tenants/acme",
		},
		{
			name:   "user attribute",
			routes: []mockpermit.Routes{mockpermit.ResourceAttributes},
			setup:  noSetup,
			config: `
resource "permitio_user_attribute" "department" {
  key         = "department"
  type        = "string"
  description = "The user's department"
}
`,
			address:    "permitio_user_attribute.department",
			readPath:   mockSchemaPath + "/resources/__user/attributes/department",
			stored:     "resource_attributes/__user:department",
			deletePath: mockSchemaPath + "/resources/__user/attributes/department",
		},
		{
			name: "role assignment",
			routes: []mockpermit.Routes{
				mockpermit.Roles, mockpermit.Tenants, mockpermit.RoleAssignments,
			},
			setup: addAlice,
			config: acmeTenantConfig + `
resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  permissions = []
  extends     = []
}

resource "permitio_role_assignment" "alice" {
  user   = "alice"
  role   = permitio_role.editor.key
  tenant = permitio_tenant.acme.key
}
`,
			address:    "permitio_role_assignment.alice",
			readPath:   mockFactsPath + "/role_assignments",
			stored:     "role_assignments/alice:editor:acme:",
			deletePath: mockFactsPath + "/users/alice/roles",
			kept:       []string{"users/alice"},
		},
		{
			name: "resource instance",
			routes: []mockpermit.Routes{
				mockpermit.Resources, mockpermit.Tenants, mockpermit.ResourceInstances,
			},
			setup:      noSetup,
			config:     documentResourceConfig + acmeTenantConfig + handbookInstanceConfig,
			address:    "permitio_resource_instance.handbook",
			readPath:   mockFactsPath + "/resource_instances/document:handbook",
			stored:     "resource_instances/document:handbook",
			deletePath: mockFactsPath + "/resource_instances/document:handbook",
		},
		{
			name: "resource instance role assignment",
			routes: []mockpermit.Routes{
				mockpermit.Resources, mockpermit.ResourceRoles, mockpermit.Tenants,
				mockpermit.ResourceInstances, mockpermit.RoleAssignments,
			},
			setup: addAlice,
			config: documentResourceConfig + acmeTenantConfig + handbookInstanceConfig +
				documentReaderRoleConfig + `
resource "permitio_resource_instance_role_assignment" "alice" {
  user              = "alice"
  role              = permitio_role.reader.key
  resource          = permitio_resource.document.key
  resource_instance = permitio_resource_instance.handbook.key
  tenant            = permitio_tenant.acme.key
}
`,
			address:    "permitio_resource_instance_role_assignment.alice",
			readPath:   mockFactsPath + "/role_assignments",
			stored:     "role_assignments/alice:reader:acme:document:handbook",
			deletePath: mockFactsPath + "/users/alice/roles",
			kept:       []string{"users/alice"},
		},
		{
			name: "group resource instance role assignment",
			routes: []mockpermit.Routes{
				mockpermit.Resources, mockpermit.ResourceRoles, mockpermit.Tenants,
				mockpermit.TenantList, mockpermit.ResourceInstances, mockpermit.GroupRoles,
			},
			setup: func(m *mockpermit.Server) { m.AddGroup("readers", "acme") },
			config: documentResourceConfig + acmeTenantConfig + handbookInstanceConfig +
				documentReaderRoleConfig + `
resource "permitio_group_resource_instance_role_assignment" "readers" {
  group             = "readers"
  role              = permitio_role.reader.key
  resource          = permitio_resource.document.key
  resource_instance = permitio_resource_instance.handbook.key
  tenant            = permitio_tenant.acme.key
}
`,
			address:  "permitio_group_resource_instance_role_assignment.readers",
			readPath: mockSchemaPath + "/groups/readers/roles",
			stored:   "group_roles/readers:reader:document:handbook:acme",
			// What the API answers to removing a role the group does not have is
			// unconfirmed. It answers 404 when the group is gone, which takes its
			// roles with it.
			goneBeforeDestroy: []string{
				"group_roles/readers:reader:document:handbook:acme", "groups/readers",
			},
			deletePath: mockSchemaPath + "/groups/readers/roles",
			kept:       []string{"groups/readers"},
		},
	}
}

const documentResourceConfig = `
resource "permitio_resource" "document" {
  key     = "document"
  name    = "Document"
  actions = {
    read = { name = "Read" }
  }
}
`

const reviewersUserSetConfig = `
resource "permitio_user_set" "reviewers" {
  key  = "reviewers"
  name = "Reviewers"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.team" = { equals = "review" } }] }]
  })
}
`

const draftsResourceSetConfig = `
resource "permitio_resource_set" "drafts" {
  key      = "drafts"
  name     = "Drafts"
  resource = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [{ allOf = [{ "resource.status" = { equals = "draft" } }] }]
  })
}
`

const folderAndFileConfig = `
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
`

const parentRelationConfig = `
resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent folder"
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.file.key
}
`

const acmeTenantConfig = `
resource "permitio_tenant" "acme" {
  key  = "acme"
  name = "Acme"
}
`

const handbookInstanceConfig = `
resource "permitio_resource_instance" "handbook" {
  key      = "handbook"
  resource = permitio_resource.document.key
  tenant   = permitio_tenant.acme.key
}
`

const documentReaderRoleConfig = `
resource "permitio_role" "reader" {
  key         = "reader"
  name        = "Reader"
  resource    = permitio_resource.document.key
  permissions = ["read"]
  extends     = []
}
`
