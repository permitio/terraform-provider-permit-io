package provider

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// apiErrorCase says, for one of the deleted object cases, how its diagnostics name
// the object at address and which requests create and update it.
type apiErrorCase struct {
	// object is the object's type and quoted key, as a diagnostic names it.
	object string
	// createPath is the path of the POST that creates the object.
	createPath string
	// update changes the configuration so that the object is updated in place. It
	// is nil for a resource type that is replaced on every change.
	update *configUpdate
}

// configUpdate replaces from with to in a configuration, which makes Terraform
// send a PATCH to path. ${name} in path stands for the object's state attribute
// name.
type configUpdate struct {
	path, from, to string
}

// TestCreateErrorsShowStatusAndMessage answers the create of each resource type's
// object with a 409 and checks that the error names the object, the status and
// the message in the API's body.
func TestCreateErrorsShowStatusAndMessage(t *testing.T) {
	cases := apiErrorCases()
	for _, c := range deletedObjectCases() {
		t.Run(c.name, func(t *testing.T) {
			errCase, ok := cases[c.name]
			if !ok {
				t.Fatalf("no API error case for %s", c.name)
			}
			m := mockpermit.New(t, c.routes...)
			c.setup(m)
			stop := m.FailRequests(http.MethodPost, errCase.createPath, http.StatusConflict, 0)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             m.CheckStored(c.kept...),
				Steps: []resource.TestStep{
					{
						Config: c.config,
						ExpectError: apiErrorPattern("create", errCase.object,
							"409 Conflict: object not found"),
					},
				},
			})
			stop()
		})
	}
}

// TestUpdateErrorsShowStatusAndMessage answers the update of each resource type
// that is updated in place with a 422 and checks that the error names the object,
// the status and the message in the API's body.
func TestUpdateErrorsShowStatusAndMessage(t *testing.T) {
	updated := 0
	for name, errCase := range apiErrorCases() {
		if errCase.update == nil {
			continue
		}
		updated++
		c := deletedObjectCaseNamed(t, name)
		t.Run(c.name, func(t *testing.T) {
			m := mockpermit.New(t, c.routes...)
			c.setup(m)
			update := errCase.update
			if !strings.Contains(c.config, update.from) {
				t.Fatalf("the configuration has no %q to update", update.from)
			}
			var patchPath string
			var stop func()

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             m.CheckStored(c.kept...),
				Steps: []resource.TestStep{
					{
						Config: c.config,
						Check:  expandStatePath(c.address, update.path, &patchPath),
					},
					{
						PreConfig: func() {
							stop = m.FailRequests(http.MethodPatch, patchPath,
								http.StatusUnprocessableEntity, 0)
						},
						Config: strings.Replace(c.config, update.from, update.to, 1),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction(c.address,
									plancheck.ResourceActionUpdate),
							},
						},
						ExpectError: apiErrorPattern("update", errCase.object,
							"422 Unprocessable Entity: object not found"),
					},
				},
			})
			stop()
		})
	}
	if updated == 0 {
		t.Fatal("DID NOT RUN: no case updates an object in place")
	}
}

// TestDataSourceReadErrorsShowStatusAndMessage answers each data source's read
// with a 404 and checks that the error names the object, the status and the
// message in the API's body, and that it is the last diagnostic: a Read that went
// on after the error would add another one.
func TestDataSourceReadErrorsShowStatusAndMessage(t *testing.T) {
	cases := []struct {
		name     string
		routes   mockpermit.Routes
		config   string
		readPath string
		object   string
	}{
		{"resource", mockpermit.Resources, `data "permitio_resource" "ghost" { key = "ghost" }`,
			mockSchemaPath + "/resources/ghost", `resource "ghost"`},
		{"role", mockpermit.Roles, `data "permitio_role" "ghost" { key = "ghost" }`,
			mockSchemaPath + "/roles/ghost", `role "ghost"`},
		{"role of a resource", mockpermit.ResourceRoles, `data "permitio_role" "ghost" {
  key      = "ghost"
  resource = "document"
}`,
			mockSchemaPath + "/resources/document/roles/ghost", `role "document:ghost"`},
		{"condition set", mockpermit.ConditionSets,
			`data "permitio_condition_set" "ghost" { key = "ghost" }`,
			mockSchemaPath + "/condition_sets/ghost", `condition set "ghost"`},
		{"user", mockpermit.Users, `data "permitio_user" "ghost" { key = "ghost" }`,
			mockFactsPath + "/users/ghost", `user "ghost"`},
		{"user attribute", mockpermit.ResourceAttributes,
			`data "permitio_user_attribute" "ghost" { key = "ghost" }`,
			mockSchemaPath + "/resources/__user/attributes/ghost", `user attribute "ghost"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := mockpermit.New(t, c.routes)
			stop := m.FailRequests(http.MethodGet, c.readPath, http.StatusNotFound, 0)
			lastError := regexp.MustCompile(
				apiErrorPattern("read", c.object, "404 Not Found: object not found").String() +
					`\s*\z`)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             m.CheckEmpty,
				Steps: []resource.TestStep{
					{
						Config:      c.config,
						ExpectError: lastError,
					},
				},
			})
			stop()
		})
	}
}

// apiErrorPattern matches the diagnostic for the failed operation on object: its
// summary, then a detail that names the operation on object and gives
// statusAndMessage, the status and message of the API's answer.
func apiErrorPattern(operation, object, statusAndMessage string) *regexp.Regexp {
	return regexp.MustCompile(summaryPattern(operation, object) + `(?s:.*?)` +
		wordsPattern("Unable to "+operation+" "+object+": "+statusAndMessage))
}

// operationVerbs are the forms a diagnostic's summary names each operation with,
// such as "Unable to read" and "Failed reading".
var operationVerbs = map[string]string{
	"create": "create|creating",
	"read":   "read|reading",
	"update": "update|updating",
	"delete": "delete|deleting",
}

// summaryPattern matches the summary line of the diagnostic for the failed
// operation on object, such as "Error: Unable to read role", which ends with the
// object's type, in any case.
func summaryPattern(operation, object string) string {
	objectType, _, _ := strings.Cut(object, ` "`)
	return `(?i:Error:\s+\w+(?:\s+to)?\s+(?:` + operationVerbs[operation] + `)\s+` +
		wordsPattern(objectType) + `\s*\n)`
}

// wordsPattern matches text with any run of spaces in place of each space, so that
// it matches the line breaks Terraform wraps a diagnostic with.
func wordsPattern(text string) string {
	return strings.Join(strings.Fields(regexp.QuoteMeta(text)), `\s+`)
}

// deletedObjectCaseNamed returns the deleted object case with this name.
func deletedObjectCaseNamed(t *testing.T, name string) deletedObjectCase {
	t.Helper()
	for _, c := range deletedObjectCases() {
		if c.name == name {
			return c
		}
	}
	t.Fatalf("no deleted object case is named %s", name)
	return deletedObjectCase{}
}

// apiErrorCases returns the API error case of each deleted object case, by name.
func apiErrorCases() map[string]apiErrorCase {
	return map[string]apiErrorCase{
		"resource": {
			object:     `resource "document"`,
			createPath: mockSchemaPath + "/resources",
			update: &configUpdate{
				path: mockSchemaPath + "/resources/document",
				from: `name    = "Document"`, to: `name    = "Documents"`,
			},
		},
		"role": {
			object:     `role "admin"`,
			createPath: mockSchemaPath + "/roles",
			update: &configUpdate{
				path: mockSchemaPath + "/roles/admin",
				from: `name        = "Admin"`, to: `name        = "Administrator"`,
			},
		},
		"user set": {
			object:     `condition set "reviewers"`,
			createPath: mockSchemaPath + "/condition_sets",
			update: &configUpdate{
				path: mockSchemaPath + "/condition_sets/reviewers",
				from: `name = "Reviewers"`, to: `name = "Code reviewers"`,
			},
		},
		"resource set": {
			object:     `condition set "drafts"`,
			createPath: mockSchemaPath + "/condition_sets",
			update: &configUpdate{
				path: mockSchemaPath + "/condition_sets/drafts",
				from: `name     = "Drafts"`, to: `name     = "Draft documents"`,
			},
		},
		"condition set rule": {
			object:     `condition set rule "reviewers,document:read,drafts"`,
			createPath: mockFactsPath + "/set_rules",
		},
		"proxy config": {
			object:     `proxy config "billing"`,
			createPath: mockFactsPath + "/proxy_configs",
			update: &configUpdate{
				path: mockFactsPath + "/proxy_configs/billing",
				from: `name           = "Billing API"`, to: `name           = "Billing"`,
			},
		},
		"relation": {
			object:     `relation "file/parent"`,
			createPath: mockSchemaPath + "/resources/file/relations",
		},
		"role derivation": {
			object:     `role derivation "folder:manager to file:editor"`,
			createPath: mockSchemaPath + "/resources/file/roles/editor/implicit_grants",
		},
		"tenant": {
			object:     `tenant "acme"`,
			createPath: mockFactsPath + "/tenants",
			update: &configUpdate{
				path: mockFactsPath + "/tenants/acme",
				from: `name = "Acme"`, to: `name = "Acme Corp"`,
			},
		},
		"user attribute": {
			object:     `user attribute "department"`,
			createPath: mockSchemaPath + "/resources/__user/attributes",
			update: &configUpdate{
				path: mockSchemaPath + "/resources/__user/attributes/${id}",
				from: `description = "The user's department"`,
				to:   `description = "The user's team"`,
			},
		},
		"role assignment": {
			object:     `role assignment "alice:editor:acme"`,
			createPath: mockFactsPath + "/users/alice/roles",
		},
		"resource instance": {
			object:     `resource instance "document:handbook"`,
			createPath: mockFactsPath + "/resource_instances",
			update: &configUpdate{
				path: mockFactsPath + "/resource_instances/document:handbook",
				from: "  tenant   = permitio_tenant.acme.key\n}",
				to: "  tenant   = permitio_tenant.acme.key\n" +
					"  attributes = jsonencode({ pages = 10 })\n}",
			},
		},
		"resource instance role assignment": {
			object:     `resource instance role assignment "alice:reader:document:handbook:acme"`,
			createPath: mockFactsPath + "/users/alice/roles",
		},
		"group resource instance role assignment": {
			object: `group resource instance role assignment ` +
				`"readers:reader:document:handbook:acme"`,
			createPath: mockSchemaPath + "/groups/readers/roles",
		},
	}
}
