package conditionsets_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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

// TestUserSetConditionsJSON checks that conditions are compared as JSON: heredoc
// conditions, with other whitespace than the API answers with, give an empty plan
// after a create and after an update. The numbers in them, 18.0 and an integer
// beyond 2^53 that a float64 would round to 9007199254740992, reach the API as
// written (PER-16603).
func TestUserSetConditionsJSON(t *testing.T) {
	m := mockpermit.New(t, mockpermit.ConditionSets)
	const (
		address = "permitio_user_set.reviewers"
		stored  = "condition_sets/reviewers"
	)
	const heredoc = `conditions = <<-EOT
    {
      "allOf": [
        { "allOf": [
          { "subject.team": { "equals": "review" } },
          { "subject.level": { "equals": 18.0 } },
          { "subject.id": { "equals": 9007199254740993 } }
        ] }
      ]
    }
  EOT`
	const heredocValue = "{\n  \"allOf\": [\n    { \"allOf\": [\n" +
		"      { \"subject.team\": { \"equals\": \"review\" } },\n" +
		"      { \"subject.level\": { \"equals\": 18.0 } },\n" +
		"      { \"subject.id\": { \"equals\": 9007199254740993 } }\n" +
		"    ] }\n  ]\n}\n"
	const (
		sentConditions = `{"allOf": [{"allOf": [{"subject.team": {"equals": "review"}},
			{"subject.level": {"equals": 18.0}},
			{"subject.id": {"equals": 9007199254740993}}]}]}`
		storedConditions = `{"allOf":[{"allOf":[{"subject.team":{"equals":"review"}},` +
			`{"subject.level":{"equals":18.0}},{"subject.id":{"equals":9007199254740993}}]}]}`
		createBody = `{"key": "reviewers", "name": "Reviewers", "type": "userset",
			"description": "Users who review", "conditions": ` + sentConditions + `}`
		renamedBody = `{"name": "Document reviewers", "description": "Users who review",
			"conditions": ` + sentConditions + `}`
	)
	config := func(name, conditions string) string {
		return fmt.Sprintf(`
resource "permitio_user_set" "reviewers" {
  key         = "reviewers"
  name        = %q
  description = "Users who review"
  %s
}
`, name, conditions)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: config("Reviewers", heredoc),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "conditions", heredocValue),
					m.CheckRequests(http.MethodPost, conditionSetsPath, createBody),
					m.CheckStoredJSON(stored, "conditions", storedConditions),
				),
			},
			{
				Config: config("Document reviewers", heredoc),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "conditions", heredocValue),
					m.CheckRequests(http.MethodPatch, conditionSetsPath+"/reviewers",
						renamedBody),
					m.CheckStoredJSON(stored, "conditions", storedConditions),
				),
			},
		},
	})
}

// TestUserSetDescription checks that a user set's description of "" reaches the
// API and the state as "", on create and on update, and stays "" when the API
// answers with null for it, and that removing a description clears it: the
// provider sends "", which the Go SDK can send where it cannot send null, and
// keeps the description null in the state (PER-16604).
func TestUserSetDescription(t *testing.T) {
	m := mockpermit.New(t, mockpermit.ConditionSets)
	const (
		reviewers  = "permitio_user_set.reviewers"
		staff      = "permitio_user_set.staff"
		conditions = `{"allOf": [{"allOf": [{"subject.employed": {"equals": true}}]}]}`
	)
	config := func(reviewersDescription, staffDescription string) string {
		return userSetDescriptionConfig("reviewers", reviewersDescription) +
			userSetDescriptionConfig("staff", staffDescription)
	}
	body := func(fields string) string {
		return `{` + fields + `, "conditions": ` + conditions + `}`
	}
	var (
		createReviewers = body(`"key": "reviewers", "name": "reviewers", "type": "userset",
			"description": ""`)
		createStaff    = body(`"key": "staff", "name": "staff", "type": "userset"`)
		describe       = body(`"name": "reviewers", "description": "Users who review"`)
		emptyStaff     = body(`"name": "staff", "description": ""`)
		clearReviewers = body(`"name": "reviewers", "description": ""`)
		reviewersPath  = conditionSetsPath + "/reviewers"
		staffPath      = conditionSetsPath + "/staff"
		bothUpdated    = []plancheck.PlanCheck{
			plancheck.ExpectResourceAction(reviewers, plancheck.ResourceActionUpdate),
			plancheck.ExpectResourceAction(staff, plancheck.ResourceActionUpdate),
		}
		emptyPlan = []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: config(`""`, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: emptyPlan,
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(reviewers, "description", ""),
					resource.TestCheckNoResourceAttr(staff, "description"),
					m.CheckRequests(http.MethodPost, conditionSetsPath, createReviewers,
						createStaff),
					m.CheckStoredJSON("condition_sets/reviewers", "description", `""`),
				),
			},
			{
				// An API that stores "" as no description answers with null for it.
				PreConfig: func() {
					m.SetStored("condition_sets/reviewers", map[string]any{"description": nil})
				},
				Config: config(`""`, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: emptyPlan,
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(reviewers, "description", ""),
					m.CheckStoredJSON("condition_sets/reviewers", "description", "null"),
				),
			},
			{
				Config: config(`"Users who review"`, `""`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             bothUpdated,
					PostApplyPostRefresh: emptyPlan,
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(reviewers, "description", "Users who review"),
					resource.TestCheckResourceAttr(staff, "description", ""),
					m.CheckRequests(http.MethodPatch, reviewersPath, describe),
					m.CheckRequests(http.MethodPatch, staffPath, emptyStaff),
				),
			},
			{
				Config: config("", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             bothUpdated,
					PostApplyPostRefresh: emptyPlan,
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(reviewers, "description"),
					resource.TestCheckNoResourceAttr(staff, "description"),
					m.CheckRequests(http.MethodPatch, reviewersPath, describe, clearReviewers),
					m.CheckRequests(http.MethodPatch, staffPath, emptyStaff, emptyStaff),
					m.CheckStoredJSON("condition_sets/reviewers", "description", `""`),
				),
			},
		},
	})
}

// userSetDescriptionConfig returns a user set named key with the description
// argument set to description, an HCL expression, or without it when description
// is empty.
func userSetDescriptionConfig(key, description string) string {
	argument := ""
	if description != "" {
		argument = "description = " + description
	}
	return fmt.Sprintf(`
resource "permitio_user_set" %[1]q {
  key  = %[1]q
  name = %[1]q
  %[2]s
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.employed" = { equals = true } }] }]
  })
}
`, key, argument)
}

// TestUserSetParentCannotBeRemoved checks that a plan that removes parent_id from a
// set that has a parent fails, and sends nothing. The API detaches a set only when
// an update sends a null parent_id, which the Go SDK cannot send, so an apply would
// have kept the parent while the state said there was none (PER-16604). It also
// checks the remedy the error names: a tainted set is replaced without a parent.
func TestUserSetParentCannotBeRemoved(t *testing.T) {
	m := mockpermit.New(t, mockpermit.ConditionSets)
	const (
		child  = "permitio_user_set.interns"
		parent = "permitio_user_set.employees"
	)
	withoutParent := strings.Replace(employeesConfig,
		"parent_id = permitio_user_set.employees.id", "", 1)
	if withoutParent == employeesConfig {
		t.Fatal("DID NOT RUN: employeesConfig has no parent_id to remove")
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: employeesConfig,
				Check:  resource.TestCheckResourceAttrPair(child, "parent_id", parent, "id"),
			},
			{
				Config: withoutParent,
				ExpectError: regexp.MustCompile(`(?s)Cannot remove the parent of a condition ` +
					`set.*terraform\s+taint`),
			},
			{
				Config: employeesConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(child, "parent_id", parent, "id"),
					m.CheckRequests(http.MethodPatch, conditionSetsPath+"/interns"),
				),
			},
			{
				Config: withoutParent,
				Taint:  []string{child},
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(child, plancheck.ResourceActionReplace),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(child, "parent_id"),
					m.CheckRequests(http.MethodPatch, conditionSetsPath+"/interns"),
				),
			},
		},
	})
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

// TestResourceSetNamingItsResourceByIDOrKey names a resource set's resource by
// ID, then by key, then by ID again, and then names another resource. The API
// returns the resource's key, so the set must keep an ID it was given, or the plan
// after the apply is not empty. A change between the ID and the key of the set's
// resource must update the set in place, as a replacement would delete the rules
// on it, while naming another resource must replace the set, as the API cannot
// move it.
func TestResourceSetNamingItsResourceByIDOrKey(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ConditionSets)
	const address = "permitio_resource_set.drafts"
	step := func(resourceName, attribute string,
		action plancheck.ResourceActionType,
	) resource.TestStep {
		return resource.TestStep{
			Config: fmt.Sprintf(`
resource "permitio_resource" "document" {
  key  = "document"
  name = "Document"
  actions = {
    read = { name = "Read" }
  }
}

resource "permitio_resource" "folder" {
  key  = "folder"
  name = "Folder"
  actions = {
    list = { name = "List" }
  }
}

resource "permitio_resource_set" "drafts" {
  key      = "drafts"
  name     = "Drafts"
  resource = permitio_resource.%s.%s
  conditions = jsonencode({
    allOf = [{ allOf = [{ "resource.status" = { equals = "draft" } }] }]
  })
}
`, resourceName, attribute),
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(address, action),
				},
			},
			Check: resource.TestCheckResourceAttrPair(address, "resource",
				"permitio_resource."+resourceName, attribute),
		}
	}

	// The plan reads the set once to refresh it and once more to compare its
	// resource with the new one, which the fault fails.
	var stopFault func()
	readFails := step("document", "key", plancheck.ResourceActionDestroyBeforeCreate)
	readFails.PreConfig = func() {
		stopFault = m.FailRequests(http.MethodGet, conditionSetsPath+"/drafts",
			http.StatusInternalServerError, 1)
	}
	readFails.ExpectError = regexp.MustCompile(`Unable to read resource set`)
	unchanged := step("folder", "key", plancheck.ResourceActionNoop)
	unchanged.PreConfig = func() { stopFault() }

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			step("document", "id", plancheck.ResourceActionCreate),
			step("document", "key", plancheck.ResourceActionUpdate),
			step("document", "id", plancheck.ResourceActionUpdate),
			step("folder", "id", plancheck.ResourceActionDestroyBeforeCreate),
			step("folder", "key", plancheck.ResourceActionUpdate),
			readFails,
			unchanged,
		},
	})
}

// TestUserSetRejectsResource checks that a user set takes no resource: the API
// drops a user set's resource, so setting one never had an effect.
func TestUserSetRejectsResource(t *testing.T) {
	mockpermit.New(t, mockpermit.ConditionSets)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "permitio_user_set" "reviewers" {
  key      = "reviewers"
  name     = "Reviewers"
  resource = "document"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.team" = { equals = "review" } }] }]
  })
}
`,
				ExpectError: regexp.MustCompile(`An argument named "resource" is not expected here`),
			},
		},
	})
}

// TestUserSetStateWithResourceUpgrades gives the provider the state of a user set
// that an earlier release saved with a resource, as Terraform does before it uses
// a saved state, and checks that the provider drops the resource and keeps every
// other value. Every earlier release saved user sets at schema version 0.
func TestUserSetStateWithResourceUpgrades(t *testing.T) {
	server, err := providerFactories["permitio"]()
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := server.GetProviderSchema(t.Context(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	userSet, ok := schemas.ResourceSchemas["permitio_user_set"]
	if !ok {
		t.Fatal("the provider has no permitio_user_set schema")
	}
	kept := map[string]string{
		"id":              mockpermit.ObjectID(1),
		"organization_id": mockpermit.OrganizationID,
		"project_id":      mockpermit.ProjectID,
		"environment_id":  mockpermit.EnvironmentID,
		"key":             "reviewers",
		"name":            "Reviewers",
		"description":     "Users who review documents",
		"conditions":      `{"allOf":[{"allOf":[{"subject.team":{"equals":"review"}}]}]}`,
	}
	saved := map[string]any{"resource": "document", "parent_id": nil}
	want := map[string]tftypes.Value{"parent_id": tftypes.NewValue(tftypes.String, nil)}
	for name, value := range kept {
		saved[name] = value
		want[name] = tftypes.NewValue(tftypes.String, value)
	}
	savedJSON, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := server.UpgradeResourceState(t.Context(), &tfprotov6.UpgradeResourceStateRequest{
		TypeName: "permitio_user_set",
		Version:  0,
		RawState: &tfprotov6.RawState{JSON: savedJSON},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		t.Errorf("upgrading the saved state: %s: %s", d.Summary, d.Detail)
	}
	if t.Failed() {
		t.FailNow()
	}
	upgraded, err := resp.UpgradedState.Unmarshal(userSet.ValueType())
	if err != nil {
		t.Fatalf("decoding the upgraded state: %v", err)
	}
	var got map[string]tftypes.Value
	if err := upgraded.As(&got); err != nil {
		t.Fatalf("reading the upgraded state as an object: %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("the upgraded state has attributes %q, want %q",
			slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
	}
	for name, value := range want {
		if !got[name].Equal(value) {
			t.Errorf("the upgraded state has %s = %s, want %s", name, got[name], value)
		}
	}
}
