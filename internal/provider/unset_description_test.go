package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// descriptionSetOutside is the description TestUnsetDescriptionIsLeftOut gives
// objects in the mock, the way an edit in the Permit UI would.
const descriptionSetOutside = "Set in the Permit UI"

// TestUnsetDescriptionIsLeftOut checks that a resource, a role, a resource role, a
// tenant and a relation configured without a description, and a resource without
// a urn, leave them out of the requests that create and update them instead of
// sending "". The objects then get no description, and a description set outside
// Terraform survives an update that does not mention it (PER-16604).
func TestUnsetDescriptionIsLeftOut(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.Roles, mockpermit.ResourceRoles,
		mockpermit.Tenants, mockpermit.ResourceRelations)
	const (
		resourcesPath = mockSchemaPath + "/resources"
		documentPath  = resourcesPath + "/document"
		rolesPath     = mockSchemaPath + "/roles"
		tenantsPath   = mockFactsPath + "/tenants"
	)
	// updated are the objects updated in place, by address, with where the mock
	// keeps each.
	updated := map[string]string{
		"permitio_resource.document": "resources/document",
		"permitio_role.viewer":       "roles/viewer",
		"permitio_role.reader":       "resource_roles/document:reader",
		"permitio_tenant.acme":       "tenants/acme",
	}
	created := []resource.TestCheckFunc{
		resource.TestCheckNoResourceAttr("permitio_resource.folder", "description"),
		resource.TestCheckNoResourceAttr("permitio_resource.document", "urn"),
		resource.TestCheckNoResourceAttr("permitio_relation.parent", "description"),
		m.CheckRequests(http.MethodPost, resourcesPath,
			`{"key": "folder", "name": "Folder", "actions": {"list": {"name": "List"}},
			  "attributes": null}`,
			`{"key": "document", "name": "Document", "actions": {"read": {"name": "Read"}},
			  "attributes": null}`),
		m.CheckRequests(http.MethodPost, rolesPath,
			`{"key": "viewer", "name": "Viewer", "permissions": [], "extends": []}`),
		m.CheckRequests(http.MethodPost, documentPath+"/roles",
			`{"key": "reader", "name": "Reader", "permissions": ["read"], "extends": []}`),
		m.CheckRequests(http.MethodPost, tenantsPath, `{"key": "acme", "name": "Acme"}`),
		m.CheckRequests(http.MethodPost, documentPath+"/relations",
			`{"key": "parent", "name": "Parent folder", "subject_resource": "folder"}`),
	}
	renamed := []resource.TestCheckFunc{
		resource.TestCheckNoResourceAttr("permitio_resource.document", "urn"),
		m.CheckRequests(http.MethodPatch, documentPath,
			`{"name": "Document renamed", "actions": {"read": {"name": "Read"}},
			  "attributes": null}`),
		m.CheckRequests(http.MethodPatch, rolesPath+"/viewer",
			`{"name": "Viewer renamed", "extends": []}`),
		m.CheckRequests(http.MethodPatch, documentPath+"/roles/reader",
			`{"name": "Reader renamed", "extends": []}`),
		m.CheckRequests(http.MethodPatch, tenantsPath+"/acme",
			`{"name": "Acme renamed", "attributes": {}}`),
	}
	var updateActions []plancheck.PlanCheck
	for address, stored := range updated {
		created = append(created, resource.TestCheckNoResourceAttr(address, "description"))
		renamed = append(renamed,
			resource.TestCheckResourceAttr(address, "description", descriptionSetOutside),
			m.CheckStoredJSON(stored, "description", `"`+descriptionSetOutside+`"`))
		updateActions = append(updateActions,
			plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate))
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: unsetDescriptionConfig(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(created...),
			},
			{
				PreConfig: func() {
					for _, stored := range updated {
						m.SetStored(stored, map[string]any{"description": descriptionSetOutside})
					}
				},
				Config: unsetDescriptionConfig(" renamed"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             updateActions,
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(renamed...),
			},
		},
	})
}

// unsetDescriptionConfig returns a resource, a role, a resource role, a tenant and
// a relation without a description, and resources without a urn. suffix ends the
// names of the objects that TestUnsetDescriptionIsLeftOut updates in place.
func unsetDescriptionConfig(suffix string) string {
	return fmt.Sprintf(`
resource "permitio_resource" "folder" {
  key  = "folder"
  name = "Folder"
  actions = {
    list = { name = "List" }
  }
}

resource "permitio_resource" "document" {
  key  = "document"
  name = "Document%[1]s"
  actions = {
    read = { name = "Read" }
  }
}

resource "permitio_role" "viewer" {
  key  = "viewer"
  name = "Viewer%[1]s"
}

resource "permitio_role" "reader" {
  key         = "reader"
  name        = "Reader%[1]s"
  resource    = permitio_resource.document.key
  permissions = ["read"]
}

resource "permitio_tenant" "acme" {
  key  = "acme"
  name = "Acme%[1]s"
}

resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent folder"
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.document.key
}
`, suffix)
}
