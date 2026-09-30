package conditionsets_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// TestConditionSetDataSourceRead reads a user set and a resource set that the mock
// Permit API serves through the permitio_condition_set data source, by key only,
// and checks every attribute it exports. The user set has no description, resource
// or parent; the resource set has all three. A third read passes the inputs older
// configurations had to set, with values that differ from the user set's, and
// checks that they are accepted and that the values read from Permit replace them.
func TestConditionSetDataSourceRead(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ConditionSets)
	const (
		userSet     = "data.permitio_condition_set.contractors"
		resourceSet = "data.permitio_condition_set.internal_reports"
		legacy      = "data.permitio_condition_set.legacy"
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: documentConfig + `
resource "permitio_user_set" "contractors" {
  key  = "contractors"
  name = "Contractors"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.contractor" = { equals = true } }] }]
  })
}

data "permitio_condition_set" "contractors" {
  key = permitio_user_set.contractors.key
}

data "permitio_condition_set" "internal_reports" {
  key = permitio_resource_set.internal_reports.key
}

data "permitio_condition_set" "legacy" {
  key         = permitio_user_set.contractors.key
  name        = "Wrong"
  description = "Wrong"
  type        = "resourceset"
  resource    = "document"
  conditions  = "{}"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(userSet, "id",
						"permitio_user_set.contractors", "id"),
					resource.TestCheckResourceAttr(userSet, "key", "contractors"),
					resource.TestCheckResourceAttr(userSet, "name", "Contractors"),
					resource.TestCheckNoResourceAttr(userSet, "description"),
					resource.TestCheckResourceAttr(userSet, "type", "userset"),
					resource.TestCheckNoResourceAttr(userSet, "resource"),
					resource.TestCheckResourceAttr(userSet, "conditions",
						`{"allOf":[{"allOf":[{"subject.contractor":{"equals":true}}]}]}`),
					resource.TestCheckNoResourceAttr(userSet, "parent_id"),
					resource.TestCheckResourceAttr(userSet, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(userSet, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(userSet, "environment_id",
						mockpermit.EnvironmentID),

					resource.TestCheckResourceAttrPair(resourceSet, "id",
						"permitio_resource_set.internal_reports", "id"),
					resource.TestCheckResourceAttr(resourceSet, "key", "internal_reports"),
					resource.TestCheckResourceAttr(resourceSet, "name", "Internal reports"),
					resource.TestCheckResourceAttr(resourceSet, "description",
						"Reports only employees may see"),
					resource.TestCheckResourceAttr(resourceSet, "type", "resourceset"),
					resource.TestCheckResourceAttr(resourceSet, "resource", "document"),
					resource.TestCheckResourceAttr(resourceSet, "conditions",
						`{"allOf":[{"allOf":[{"resource.type":{"equals":"report"}}]}]}`),
					resource.TestCheckResourceAttrPair(resourceSet, "parent_id",
						"permitio_resource_set.internal_documents", "id"),
					resource.TestCheckResourceAttr(resourceSet, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(resourceSet, "project_id",
						mockpermit.ProjectID),
					resource.TestCheckResourceAttr(resourceSet, "environment_id",
						mockpermit.EnvironmentID),

					resource.TestCheckResourceAttrPair(legacy, "id",
						"permitio_user_set.contractors", "id"),
					resource.TestCheckResourceAttr(legacy, "name", "Contractors"),
					resource.TestCheckNoResourceAttr(legacy, "description"),
					resource.TestCheckResourceAttr(legacy, "type", "userset"),
					resource.TestCheckNoResourceAttr(legacy, "resource"),
					resource.TestCheckResourceAttr(legacy, "conditions",
						`{"allOf":[{"allOf":[{"subject.contractor":{"equals":true}}]}]}`),
				),
			},
		},
	})
}
