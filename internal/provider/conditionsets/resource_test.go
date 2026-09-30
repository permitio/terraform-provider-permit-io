package conditionsets_test

import (
	"fmt"
	"net/http"
	"strings"
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

const conditionSetsPath = "/v2/schema/" + mockpermit.ProjectID + "/" +
	mockpermit.EnvironmentID + "/condition_sets"

// parentIDToken stands for the parent set's ID in a body that
// checkRequestsNamingParent checks, since the test cannot know the ID up front.
const parentIDToken = "<parent ID>"

// employeesConfig is the user set that the other user sets get as their parent,
// and interns, a user set created with it as its parent.
const employeesConfig = `
resource "permitio_user_set" "employees" {
  key         = "employees"
  name        = "Employees"
  description = "Users employed by the company"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.employed" = { equals = true } }] }]
  })
}

resource "permitio_user_set" "interns" {
  key         = "interns"
  name        = "Interns"
  description = "Employees on an internship"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.role" = { equals = "intern" } }] }]
  })
  parent_id = permitio_user_set.employees.id
}
`

// TestUserSetCreateUpdateDestroy runs permitio_user_set through Terraform against
// the mock Permit API and checks the exact bodies the provider sends. It creates
// sets with and without a parent_id. The update changes every attribute a user set
// updates in place: name, description, conditions, and parent_id, which it sets
// to another user set.
func TestUserSetCreateUpdateDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.ConditionSets)
	const (
		address = "permitio_user_set.engineers"
		child   = "permitio_user_set.interns"
		parent  = "permitio_user_set.employees"
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: employeesConfig + `
resource "permitio_user_set" "engineers" {
  key         = "engineers"
  name        = "Engineers"
  description = "Users in engineering"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.department" = { equals = "engineering" } }] }]
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(address, "id"),
					resource.TestCheckResourceAttr(address, "key", "engineers"),
					resource.TestCheckResourceAttr(address, "name", "Engineers"),
					resource.TestCheckResourceAttr(address, "description", "Users in engineering"),
					resource.TestCheckResourceAttr(address, "conditions",
						`{"allOf":[{"allOf":[{"subject.department":{"equals":"engineering"}}]}]}`),
					resource.TestCheckNoResourceAttr(address, "resource"),
					resource.TestCheckNoResourceAttr(address, "parent_id"),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					resource.TestCheckResourceAttr(parent, "conditions",
						`{"allOf":[{"allOf":[{"subject.employed":{"equals":true}}]}]}`),
					resource.TestCheckResourceAttrPair(child, "parent_id", parent, "id"),
					checkRequestsNamingParent(m, http.MethodPost, conditionSetsPath, parent,
						`{"key": "employees", "name": "Employees", "type": "userset",
						  "description": "Users employed by the company",
						  "conditions": {"allOf": [{"allOf": [
						    {"subject.employed": {"equals": true}}]}]}}`,
						`{"key": "interns", "name": "Interns", "type": "userset",
						  "description": "Employees on an internship",
						  "conditions": {"allOf": [{"allOf": [
						    {"subject.role": {"equals": "intern"}}]}]},
						  "parent_id": "`+parentIDToken+`"}`,
						`{"key": "engineers", "name": "Engineers", "type": "userset",
						  "description": "Users in engineering",
						  "conditions": {"allOf": [{"allOf": [
						    {"subject.department": {"equals": "engineering"}}]}]}}`,
					),
				),
			},
			{
				Config: employeesConfig + `
resource "permitio_user_set" "engineers" {
  key         = "engineers"
  name        = "Senior engineers"
  description = "Senior users in engineering"
  conditions = jsonencode({
    allOf = [{ allOf = [
      { "subject.department" = { equals = "engineering" } },
      { "subject.level" = { equals = "senior" } },
    ] }]
  })
  parent_id = permitio_user_set.employees.id
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction(parent, plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction(child, plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "name", "Senior engineers"),
					resource.TestCheckResourceAttr(address, "description",
						"Senior users in engineering"),
					resource.TestCheckResourceAttr(address, "conditions",
						`{"allOf":[{"allOf":[{"subject.department":{"equals":"engineering"}},`+
							`{"subject.level":{"equals":"senior"}}]}]}`),
					resource.TestCheckResourceAttrPair(address, "parent_id", parent, "id"),
					resource.TestCheckResourceAttrPair(child, "parent_id", parent, "id"),
					checkRequestsNamingParent(m, http.MethodPatch, conditionSetsPath+"/engineers",
						parent,
						`{"name": "Senior engineers", "description": "Senior users in engineering",
						  "conditions": {"allOf": [{"allOf": [
						    {"subject.department": {"equals": "engineering"}},
						    {"subject.level": {"equals": "senior"}}]}]},
						  "parent_id": "`+parentIDToken+`"}`),
					m.CheckRequests(http.MethodPatch, conditionSetsPath+"/employees"),
					m.CheckRequests(http.MethodPatch, conditionSetsPath+"/interns"),
				),
			},
		},
	})

	m.AssertAllRoutesHit()
}

// documentConfig is the resource that the resource sets are on, the
// internal_documents set that the other resource sets get as their parent, and
// internal_reports, a resource set created with it as its parent.
const documentConfig = `
resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A text document"
  urn         = "prn:test:document"
  actions = {
    read = { name = "Read" }
  }
}

resource "permitio_resource_set" "internal_documents" {
  key         = "internal_documents"
  name        = "Internal documents"
  description = "Documents only employees may see"
  resource    = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [{ allOf = [{ "resource.classification" = { equals = "internal" } }] }]
  })
}

resource "permitio_resource_set" "internal_reports" {
  key         = "internal_reports"
  name        = "Internal reports"
  description = "Reports only employees may see"
  resource    = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [{ allOf = [{ "resource.type" = { equals = "report" } }] }]
  })
  parent_id = permitio_resource_set.internal_documents.id
}
`

// TestResourceSetCreateUpdateDestroy runs permitio_resource_set through Terraform
// against the mock Permit API and checks the exact bodies the provider sends. It
// creates sets with and without a parent_id. The update changes every
// attribute a resource set updates in place: name, description, conditions, and
// parent_id, which it sets to another resource set on the same resource. The
// resource itself does not change.
func TestResourceSetCreateUpdateDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ConditionSets)
	const (
		address = "permitio_resource_set.drafts"
		child   = "permitio_resource_set.internal_reports"
		parent  = "permitio_resource_set.internal_documents"
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: documentConfig + `
resource "permitio_resource_set" "drafts" {
  key         = "drafts"
  name        = "Drafts"
  description = "Documents not yet published"
  resource    = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [{ allOf = [{ "resource.status" = { equals = "draft" } }] }]
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(address, "id"),
					resource.TestCheckResourceAttr(address, "key", "drafts"),
					resource.TestCheckResourceAttr(address, "name", "Drafts"),
					resource.TestCheckResourceAttr(address, "description",
						"Documents not yet published"),
					resource.TestCheckResourceAttr(address, "resource", "document"),
					resource.TestCheckResourceAttr(address, "conditions",
						`{"allOf":[{"allOf":[{"resource.status":{"equals":"draft"}}]}]}`),
					resource.TestCheckNoResourceAttr(address, "parent_id"),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					resource.TestCheckResourceAttr(child, "resource", "document"),
					resource.TestCheckResourceAttrPair(child, "parent_id", parent, "id"),
					checkRequestsNamingParent(m, http.MethodPost, conditionSetsPath, parent,
						`{"key": "internal_documents", "name": "Internal documents",
						  "type": "resourceset", "resource_id": "document",
						  "description": "Documents only employees may see",
						  "conditions": {"allOf": [{"allOf": [
						    {"resource.classification": {"equals": "internal"}}]}]}}`,
						`{"key": "internal_reports", "name": "Internal reports",
						  "type": "resourceset", "resource_id": "document",
						  "description": "Reports only employees may see",
						  "conditions": {"allOf": [{"allOf": [
						    {"resource.type": {"equals": "report"}}]}]},
						  "parent_id": "`+parentIDToken+`"}`,
						`{"key": "drafts", "name": "Drafts", "type": "resourceset",
						  "resource_id": "document", "description": "Documents not yet published",
						  "conditions": {"allOf": [{"allOf": [
						    {"resource.status": {"equals": "draft"}}]}]}}`,
					),
				),
			},
			{
				Config: documentConfig + `
resource "permitio_resource_set" "drafts" {
  key         = "drafts"
  name        = "Internal drafts"
  description = "Internal documents not yet published"
  resource    = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [{ anyOf = [
      { "resource.status" = { equals = "draft" } },
      { "resource.status" = { equals = "review" } },
    ] }]
  })
  parent_id = permitio_resource_set.internal_documents.id
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction(parent, plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction(child, plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("permitio_resource.document",
							plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "name", "Internal drafts"),
					resource.TestCheckResourceAttr(address, "description",
						"Internal documents not yet published"),
					resource.TestCheckResourceAttr(address, "resource", "document"),
					resource.TestCheckResourceAttr(address, "conditions",
						`{"allOf":[{"anyOf":[{"resource.status":{"equals":"draft"}},`+
							`{"resource.status":{"equals":"review"}}]}]}`),
					resource.TestCheckResourceAttrPair(address, "parent_id", parent, "id"),
					resource.TestCheckResourceAttrPair(child, "parent_id", parent, "id"),
					checkRequestsNamingParent(m, http.MethodPatch, conditionSetsPath+"/drafts",
						parent,
						`{"name": "Internal drafts",
						  "description": "Internal documents not yet published",
						  "conditions": {"allOf": [{"anyOf": [
						    {"resource.status": {"equals": "draft"}},
						    {"resource.status": {"equals": "review"}}]}]},
						  "parent_id": "`+parentIDToken+`"}`),
					m.CheckRequests(http.MethodPatch, conditionSetsPath+"/internal_documents"),
					m.CheckRequests(http.MethodPatch, conditionSetsPath+"/internal_reports"),
				),
			},
		},
	})

	m.AssertRoutesHit(mockpermit.ConditionSets)
}

// checkRequestsNamingParent works like CheckRequests, after replacing
// parentIDToken in each body in want with the ID that the condition set at the
// state address parent has.
func checkRequestsNamingParent(m *mockpermit.Server, method, path, parent string,
	want ...string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		set, ok := state.RootModule().Resources[parent]
		if !ok || set.Primary.ID == "" {
			return fmt.Errorf("%s has no ID in state", parent)
		}
		bodies := make([]string, len(want))
		for i, body := range want {
			bodies[i] = strings.ReplaceAll(body, parentIDToken, set.Primary.ID)
		}
		return m.CheckRequests(method, path, bodies...)(state)
	}
}
