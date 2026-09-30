package conditionsetrules_test

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
)

var providerFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"permitio": providerserver.NewProtocol6WithError(provider.New("test")()),
}

const (
	setRulesPath = "/v2/facts/" + mockpermit.ProjectID + "/" + mockpermit.EnvironmentID +
		"/set_rules"
	ruleBody = `{"user_set": "reviewers", "permission": "document:read",
		"resource_set": "drafts", "is_role": false, "is_resource": false}`
)

// TestConditionSetRuleCreateImportDestroy runs permitio_condition_set_rule through
// Terraform against the mock Permit API and checks the exact bodies the provider
// sends to assign and unassign the permission. Every attribute of a rule forces a
// replacement, so there is no in-place update. Reading the rule back filters the
// rules by the action's key, which the import also does.
func TestConditionSetRuleCreateImportDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ConditionSets,
		mockpermit.ConditionSetRules)
	const address = "permitio_condition_set_rule.reviewers_read_drafts"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			m.CheckEmpty,
			m.CheckRequests(http.MethodDelete, setRulesPath, ruleBody),
		),
		Steps: []resource.TestStep{
			{
				Config: ruleConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					// The rule depends on every other object, so the mock creates it
					// last, after the resource, its two actions and the two sets.
					resource.TestCheckResourceAttr(address, "id", mockpermit.ObjectID(6)),
					resource.TestCheckResourceAttr(address, "user_set", "reviewers"),
					resource.TestCheckResourceAttr(address, "permission", "document:read"),
					resource.TestCheckResourceAttr(address, "resource_set", "drafts"),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					m.CheckRequests(http.MethodPost, setRulesPath, ruleBody),
					m.CheckRequests(http.MethodDelete, setRulesPath),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     "reviewers,document:read,drafts",
				ImportStateVerify: true,
			},
		},
	})

	m.AssertRoutesHit(mockpermit.ConditionSetRules)
}

// ruleConfig is a document resource, a user set of reviewers, a resource set of
// draft documents, and the rule that lets reviewers read drafts.
const ruleConfig = `
resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A text document"
  urn         = "prn:test:document"
  actions = {
    read  = { name = "Read" }
    write = { name = "Write" }
  }
}

resource "permitio_user_set" "reviewers" {
  key         = "reviewers"
  name        = "Reviewers"
  description = "Users who review documents"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.team" = { equals = "review" } }] }]
  })
}

resource "permitio_resource_set" "drafts" {
  key         = "drafts"
  name        = "Drafts"
  description = "Documents not yet published"
  resource    = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [{ allOf = [{ "resource.status" = { equals = "draft" } }] }]
  })
}

resource "permitio_condition_set_rule" "reviewers_read_drafts" {
  user_set     = permitio_user_set.reviewers.key
  permission   = "${permitio_resource.document.key}:read"
  resource_set = permitio_resource_set.drafts.key
}
`
