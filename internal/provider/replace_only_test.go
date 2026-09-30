package provider

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// replaceOnlyCase is a resource type whose every attribute forces replacement, so
// Terraform never calls its Update.
type replaceOnlyCase struct {
	resourceType string
	routes       []mockpermit.Routes
	// setup adds the objects the provider cannot create, such as users.
	setup func(m *mockpermit.Server)
	// kept are the objects setup added, which are left after destroy.
	kept []string
	// config returns the objects the resource under test refers to, and that
	// resource, named "this", with its attributes set from values.
	config func(values map[string]string) string
	// base are the attribute values of the first step.
	base map[string]string
	// changes are the values each later step sets, one attribute at a time.
	changes []attributeChange
}

type attributeChange struct {
	attribute string
	value     string
}

// TestReplaceOnlyResourcesReplaceOnEveryChange changes each attribute of each
// replace-only resource type in turn, and checks that Terraform plans to replace
// the object and that the apply, which fails if it reaches Update, succeeds.
func TestReplaceOnlyResourcesReplaceOnEveryChange(t *testing.T) {
	for _, c := range replaceOnlyCases() {
		t.Run(c.resourceType, func(t *testing.T) {
			m := mockpermit.New(t, c.routes...)
			c.setup(m)
			address := c.resourceType + ".this"
			values := maps.Clone(c.base)
			steps := []resource.TestStep{{Config: c.config(values)}}
			for _, change := range c.changes {
				values = maps.Clone(values)
				values[change.attribute] = change.value
				steps = append(steps, resource.TestStep{
					Config: c.config(values),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(address,
								plancheck.ResourceActionDestroyBeforeCreate),
						},
					},
					Check: resource.TestCheckResourceAttr(address, change.attribute,
						change.value),
				})
			}

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             m.CheckStored(c.kept...),
				Steps:                    steps,
			})
		})
	}
}

// TestReplaceOnlyCasesChangeEveryAttribute checks that each case sets and then
// changes every attribute a configuration can set, and nothing else.
func TestReplaceOnlyCasesChangeEveryAttribute(t *testing.T) {
	for _, c := range replaceOnlyCases() {
		var resp fwresource.SchemaResponse
		newResourceOfType(t, c.resourceType).Schema(context.Background(),
			fwresource.SchemaRequest{}, &resp)
		var configurable []string
		for name, attribute := range resp.Schema.Attributes {
			if attribute.IsRequired() || attribute.IsOptional() {
				configurable = append(configurable, name)
			}
		}
		var changed []string
		for _, change := range c.changes {
			changed = append(changed, change.attribute)
		}
		slices.Sort(configurable)
		slices.Sort(changed)
		base := slices.Sorted(maps.Keys(c.base))
		if len(configurable) == 0 || !slices.Equal(changed, configurable) ||
			!slices.Equal(base, configurable) {
			t.Errorf("%s: the case sets %q and changes %q, want each of %q", c.resourceType,
				base, changed, configurable)
		}
	}
}

// TestReplaceOnlyUpdateReportsAnError calls Update of each replace-only resource
// type, as Terraform never does, and checks that it reports one error instead of
// crashing the provider.
func TestReplaceOnlyUpdateReportsAnError(t *testing.T) {
	for _, c := range replaceOnlyCases() {
		var resp fwresource.UpdateResponse
		newResourceOfType(t, c.resourceType).Update(context.Background(),
			fwresource.UpdateRequest{}, &resp)
		errs := resp.Diagnostics.Errors()
		if len(errs) != 1 || len(resp.Diagnostics) != 1 ||
			!strings.Contains(errs[0].Detail(), "cannot be updated in place; all attributes "+
				"force replacement") {
			t.Errorf("%s: Update reported %v, want one error that it cannot be updated in "+
				"place", c.resourceType, resp.Diagnostics)
		}
	}
}

// newResourceOfType returns the provider's resource of this type.
func newResourceOfType(t *testing.T, resourceType string) fwresource.Resource {
	t.Helper()
	for _, newResource := range (&PermitProvider{}).Resources(context.Background()) {
		r := newResource()
		var metadata fwresource.MetadataResponse
		r.Metadata(context.Background(),
			fwresource.MetadataRequest{ProviderTypeName: "permitio"}, &metadata)
		if metadata.TypeName == resourceType {
			return r
		}
	}
	t.Fatalf("the provider has no resource type %s", resourceType)
	return nil
}

func replaceOnlyCases() []replaceOnlyCase {
	return []replaceOnlyCase{
		{
			resourceType: "permitio_role_assignment",
			routes: []mockpermit.Routes{
				mockpermit.Roles, mockpermit.Tenants, mockpermit.RoleAssignments,
			},
			setup: addAliceAndBob,
			kept:  []string{"users/alice", "users/bob"},
			config: func(v map[string]string) string {
				return tenantsConfig + `
resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  permissions = []
  extends     = []
}

resource "permitio_role" "viewer" {
  key         = "viewer"
  name        = "Viewer"
  permissions = []
  extends     = []
}
` + fmt.Sprintf(`
resource "permitio_role_assignment" "this" {
  user   = %q
  role   = %q
  tenant = %q

  depends_on = [
    permitio_role.editor, permitio_role.viewer, permitio_tenant.acme, permitio_tenant.globex,
  ]
}
`, v["user"], v["role"], v["tenant"])
			},
			base: map[string]string{"user": "alice", "role": "editor", "tenant": "acme"},
			changes: []attributeChange{
				{"user", "bob"}, {"role", "viewer"}, {"tenant", "globex"},
			},
		},
		{
			resourceType: "permitio_resource_instance_role_assignment",
			routes: []mockpermit.Routes{
				mockpermit.Resources, mockpermit.ResourceRoles, mockpermit.Tenants,
				mockpermit.ResourceInstances, mockpermit.RoleAssignments,
			},
			setup: addAliceAndBob,
			kept:  []string{"users/alice", "users/bob"},
			config: func(v map[string]string) string {
				return instancesConfig(v["tenant"]) + fmt.Sprintf(`
resource "permitio_resource_instance_role_assignment" "this" {
  user              = %q
  role              = %q
  resource          = %q
  resource_instance = %q
  tenant            = permitio_tenant.%s.key

  depends_on = [%s]
}
`, v["user"], v["role"], v["resource"], v["resource_instance"], v["tenant"], instancesDependencies)
			},
			base: map[string]string{
				"user": "alice", "role": "reader", "resource": "document",
				"resource_instance": "handbook", "tenant": "acme",
			},
			changes: []attributeChange{
				{"user", "bob"}, {"role", "editor"}, {"resource_instance", "guide"},
				{"resource", "folder"}, {"tenant", "globex"},
			},
		},
		{
			resourceType: "permitio_group_resource_instance_role_assignment",
			routes: []mockpermit.Routes{
				mockpermit.Resources, mockpermit.ResourceRoles, mockpermit.Tenants,
				mockpermit.TenantList, mockpermit.ResourceInstances, mockpermit.GroupRoles,
			},
			setup: func(m *mockpermit.Server) {
				m.AddGroup("developers", "acme")
				m.AddGroup("testers", "acme")
			},
			kept: []string{"groups/developers", "groups/testers"},
			config: func(v map[string]string) string {
				return instancesConfig(v["tenant"]) + fmt.Sprintf(`
resource "permitio_group_resource_instance_role_assignment" "this" {
  group             = %q
  role              = %q
  resource          = %q
  resource_instance = %q
  tenant            = permitio_tenant.%s.key

  depends_on = [%s]
}
`, v["group"], v["role"], v["resource"], v["resource_instance"], v["tenant"], instancesDependencies)
			},
			base: map[string]string{
				"group": "developers", "role": "reader", "resource": "document",
				"resource_instance": "handbook", "tenant": "acme",
			},
			changes: []attributeChange{
				{"group", "testers"}, {"role", "editor"}, {"resource_instance", "guide"},
				{"resource", "folder"}, {"tenant", "globex"},
			},
		},
		{
			resourceType: "permitio_role_derivation",
			routes: []mockpermit.Routes{
				mockpermit.Resources, mockpermit.ResourceRoles, mockpermit.ResourceRelations,
				mockpermit.ImplicitGrants,
			},
			setup:  noSetup,
			config: derivationsConfig,
			base: map[string]string{
				"role": "manager", "on_resource": "folder", "to_role": "editor",
				"resource": "file", "linked_by": "parent",
			},
			changes: []attributeChange{
				{"role", "owner"}, {"to_role", "viewer"}, {"linked_by", "home"},
				{"resource", "note"}, {"on_resource", "drive"},
			},
		},
		{
			resourceType: "permitio_condition_set_rule",
			routes: []mockpermit.Routes{
				mockpermit.Resources, mockpermit.ConditionSets, mockpermit.ConditionSetRules,
			},
			setup: noSetup,
			config: func(v map[string]string) string {
				return conditionSetsConfig + fmt.Sprintf(`
resource "permitio_condition_set_rule" "this" {
  user_set     = %q
  permission   = %q
  resource_set = %q

  depends_on = [
    permitio_user_set.reviewers, permitio_user_set.editors,
    permitio_resource_set.drafts, permitio_resource_set.published,
  ]
}
`, v["user_set"], v["permission"], v["resource_set"])
			},
			base: map[string]string{
				"user_set": "reviewers", "permission": "document:read", "resource_set": "drafts",
			},
			changes: []attributeChange{
				{"user_set", "editors"}, {"permission", "document:write"},
				{"resource_set", "published"},
			},
		},
	}
}

func addAliceAndBob(m *mockpermit.Server) {
	m.AddUser(`{"key": "alice", "email": "alice@example.com"}`)
	m.AddUser(`{"key": "bob", "email": "bob@example.com"}`)
}

const tenantsConfig = `
resource "permitio_tenant" "acme" {
  key  = "acme"
  name = "Acme"
}

resource "permitio_tenant" "globex" {
  key  = "globex"
  name = "Globex"
}
`

// instancesDependencies are the roles and instances of instancesConfig.
const instancesDependencies = `
    permitio_role.document_reader, permitio_role.document_editor,
    permitio_role.folder_reader, permitio_role.folder_editor,
    permitio_resource_instance.document_handbook, permitio_resource_instance.document_guide,
    permitio_resource_instance.folder_handbook, permitio_resource_instance.folder_guide,
  `

// instancesConfig returns the document and folder resources, each with reader and
// editor roles and handbook and guide instances of the tenant, and the acme and
// globex tenants.
func instancesConfig(tenant string) string {
	config := tenantsConfig
	for _, resourceKey := range []string{"document", "folder"} {
		config += fmt.Sprintf(`
resource "permitio_resource" "%[1]s" {
  key     = %[1]q
  name    = %[1]q
  actions = {
    read = { name = "Read" }
  }
}
`, resourceKey)
		for _, roleKey := range []string{"reader", "editor"} {
			config += fmt.Sprintf(`
resource "permitio_role" "%[1]s_%[2]s" {
  key         = %[2]q
  name        = %[2]q
  resource    = permitio_resource.%[1]s.key
  permissions = ["read"]
  extends     = []
}
`, resourceKey, roleKey)
		}
		for _, instanceKey := range []string{"handbook", "guide"} {
			config += fmt.Sprintf(`
resource "permitio_resource_instance" "%[1]s_%[2]s" {
  key      = %[2]q
  resource = permitio_resource.%[1]s.key
  tenant   = permitio_tenant.%[3]s.key
}
`, resourceKey, instanceKey, tenant)
		}
	}
	return config
}

// derivationsConfig returns the folder and drive resources, each with manager and
// owner roles; the file and note resources, each with editor and viewer roles and
// parent and home relations from on_resource; and a role derivation set from v.
func derivationsConfig(v map[string]string) string {
	var config strings.Builder
	var dependencies []string
	for _, subject := range []string{"folder", "drive"} {
		fmt.Fprintf(&config, `
resource "permitio_resource" "%[1]s" {
  key     = %[1]q
  name    = %[1]q
  actions = {
    list = { name = "List" }
  }
}
`, subject)
		for _, roleKey := range []string{"manager", "owner"} {
			fmt.Fprintf(&config, `
resource "permitio_role" "%[1]s_%[2]s" {
  key         = %[2]q
  name        = %[2]q
  resource    = permitio_resource.%[1]s.key
  permissions = ["list"]
  extends     = []
}
`, subject, roleKey)
			dependencies = append(dependencies, "permitio_role."+subject+"_"+roleKey)
		}
	}
	for _, object := range []string{"file", "note"} {
		fmt.Fprintf(&config, `
resource "permitio_resource" "%[1]s" {
  key     = %[1]q
  name    = %[1]q
  actions = {
    read = { name = "Read" }
  }
}
`, object)
		for _, roleKey := range []string{"editor", "viewer"} {
			fmt.Fprintf(&config, `
resource "permitio_role" "%[1]s_%[2]s" {
  key         = %[2]q
  name        = %[2]q
  resource    = permitio_resource.%[1]s.key
  permissions = ["read"]
  extends     = []
}
`, object, roleKey)
			dependencies = append(dependencies, "permitio_role."+object+"_"+roleKey)
		}
		for _, relationKey := range []string{"parent", "home"} {
			fmt.Fprintf(&config, `
resource "permitio_relation" "%[1]s_%[2]s" {
  key              = %[2]q
  name             = %[2]q
  subject_resource = permitio_resource.%[3]s.key
  object_resource  = permitio_resource.%[1]s.key
}
`, object, relationKey, v["on_resource"])
			dependencies = append(dependencies, "permitio_relation."+object+"_"+relationKey)
		}
	}
	fmt.Fprintf(&config, `
resource "permitio_role_derivation" "this" {
  resource    = %q
  to_role     = %q
  on_resource = %q
  role        = %q
  linked_by   = %q

  depends_on = [%s]
}
`, v["resource"], v["to_role"], v["on_resource"], v["role"], v["linked_by"],
		strings.Join(dependencies, ", "))
	return config.String()
}

const conditionSetsConfig = `
resource "permitio_resource" "document" {
  key     = "document"
  name    = "Document"
  actions = {
    read  = { name = "Read" }
    write = { name = "Write" }
  }
}

resource "permitio_user_set" "reviewers" {
  key  = "reviewers"
  name = "Reviewers"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.team" = { equals = "review" } }] }]
  })
}

resource "permitio_user_set" "editors" {
  key  = "editors"
  name = "Editors"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.team" = { equals = "edit" } }] }]
  })
}

resource "permitio_resource_set" "drafts" {
  key      = "drafts"
  name     = "Drafts"
  resource = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [{ allOf = [{ "resource.status" = { equals = "draft" } }] }]
  })
}

resource "permitio_resource_set" "published" {
  key      = "published"
  name     = "Published"
  resource = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [{ allOf = [{ "resource.status" = { equals = "published" } }] }]
  })
}
`
