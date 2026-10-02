package provider

import (
	"maps"
	"net/http"
	"regexp"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// permitID has the form of the IDs Permit gives its objects.
const permitID = "4f6c1e2a-7b3d-4c5e-9f8a-1b2c3d4e5f60"

// keyOnlyArguments are, by resource type, the arguments that take the key of a
// resource, role or relation and reject an ID. Developers choose those keys. The
// other arguments of these types, such as user, tenant, group, resource_instance
// and the instance's own key, accept a UUID, because those keys can be UUIDs.
var keyOnlyArguments = map[string][]string{
	"permitio_relation":          {"subject_resource", "object_resource"},
	"permitio_role_derivation":   {"resource", "role", "on_resource", "to_role", "linked_by"},
	"permitio_resource_instance": {"resource"},
	"permitio_role_assignment":   {"role"},

	"permitio_resource_instance_role_assignment":       {"role", "resource"},
	"permitio_group_resource_instance_role_assignment": {"role", "resource"},
}

// TestKeyOnlyArgumentsRejectIDs validates, through the provider's protocol server
// as Terraform does, a configuration of each resource type in keyOnlyArguments
// for each string argument it has, with that argument set to an ID and every
// other one null. It checks that exactly the key-only arguments reject the ID, and
// that no other argument does.
func TestKeyOnlyArgumentsRejectIDs(t *testing.T) {
	server, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatalf("starting the provider server: %v", err)
	}
	schemas, err := server.GetProviderSchema(t.Context(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("reading the provider schema: %v", err)
	}

	rejected, accepted := 0, 0
	for _, typeName := range slices.Sorted(maps.Keys(keyOnlyArguments)) {
		schema, ok := schemas.ResourceSchemas[typeName]
		if !ok {
			t.Errorf("the provider serves no %s", typeName)
			continue
		}
		if len(schema.Block.BlockTypes) > 0 {
			t.Fatalf("%s has blocks, which this test does not fill in", typeName)
		}
		for _, attribute := range schema.Block.Attributes {
			configurable := attribute.Required || attribute.Optional
			if !configurable || !attribute.ValueType().Is(tftypes.String) {
				continue
			}
			diagnostics := validateWithID(t, server, typeName, schema, attribute.Name)
			keyOnly := slices.Contains(keyOnlyArguments[typeName], attribute.Name)
			switch {
			case keyOnly && !hasError(diagnostics, "Expected a key, got an ID"):
				t.Errorf("%s.%s accepts an ID; want the error \"Expected a key, got an "+
					"ID\", got %q", typeName, attribute.Name, summaries(diagnostics))
			case !keyOnly && hasError(diagnostics, "Expected a key, got an ID"):
				t.Errorf("%s.%s rejects a UUID, which can be a key there: %q",
					typeName, attribute.Name, summaries(diagnostics))
			case keyOnly:
				rejected++
			default:
				accepted++
			}
		}
	}

	wantRejected := 0
	for _, arguments := range keyOnlyArguments {
		wantRejected += len(arguments)
	}
	if rejected != wantRejected {
		t.Errorf("%d key-only arguments rejected an ID, want %d", rejected, wantRejected)
	}
	// user, tenant, group, resource_instance, and the relation's and the instance's
	// own key, name, description and attributes.
	const minAccepted = 14
	if accepted < minAccepted {
		t.Errorf("DID NOT RUN: %d other arguments accepted a UUID, want at least %d",
			accepted, minAccepted)
	}
}

// validateWithID asks server to validate a configuration of typeName that sets
// argument to permitID and leaves every other attribute null.
func validateWithID(t *testing.T, server tfprotov6.ProviderServer, typeName string,
	schema *tfprotov6.Schema, argument string,
) []*tfprotov6.Diagnostic {
	t.Helper()
	values := map[string]tftypes.Value{}
	for _, attribute := range schema.Block.Attributes {
		values[attribute.Name] = tftypes.NewValue(attribute.ValueType(), nil)
	}
	values[argument] = tftypes.NewValue(tftypes.String, permitID)
	config, err := tfprotov6.NewDynamicValue(schema.ValueType(),
		tftypes.NewValue(schema.ValueType(), values))
	if err != nil {
		t.Fatalf("encoding a %s configuration: %v", typeName, err)
	}
	response, err := server.ValidateResourceConfig(t.Context(),
		&tfprotov6.ValidateResourceConfigRequest{TypeName: typeName, Config: &config})
	if err != nil {
		t.Fatalf("validating a %s configuration: %v", typeName, err)
	}
	return response.Diagnostics
}

// hasError reports whether diagnostics has an error whose summary is summary.
func hasError(diagnostics []*tfprotov6.Diagnostic, summary string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == tfprotov6.DiagnosticSeverityError &&
			diagnostic.Summary == summary {
			return true
		}
	}
	return false
}

// summaries returns the summary and detail of each of diagnostics.
func summaries(diagnostics []*tfprotov6.Diagnostic) []string {
	var texts []string
	for _, diagnostic := range diagnostics {
		texts = append(texts, diagnostic.Summary+": "+diagnostic.Detail)
	}
	return texts
}

// TestKeyOnlyArgumentsFailThePlan plans, with Terraform and the mock Permit API,
// one configuration of each resource type that has key-only arguments, with an ID
// in one of them, and a resource instance without a tenant. Each plan must fail
// with the error that names the argument, before the provider sends a request:
// the mock serves no Permit object, so a request fails the test.
func TestKeyOnlyArgumentsFailThePlan(t *testing.T) {
	idError := func(argument string) *regexp.Regexp {
		return regexp.MustCompile(`Expected a key, got an ID[\s\S]*` + argument +
			`\s+is\s+"` + permitID + `"`)
	}
	for _, tt := range []struct {
		name        string
		config      string
		expectError *regexp.Regexp
	}{
		{
			name: "permitio_relation",
			config: `
resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent folder"
  subject_resource = "folder"
  object_resource  = "` + permitID + `"
}
`,
			expectError: idError("object_resource"),
		},
		{
			name: "permitio_role_derivation",
			config: `
resource "permitio_role_derivation" "managers_edit_files" {
  resource    = "` + permitID + `"
  to_role     = "editor"
  on_resource = "folder"
  role        = "manager"
  linked_by   = "parent"
}
`,
			expectError: idError("resource"),
		},
		{
			name: "permitio_resource_instance",
			config: `
resource "permitio_resource_instance" "handbook" {
  key      = "handbook"
  resource = "` + permitID + `"
  tenant   = "acme"
}
`,
			expectError: idError("resource"),
		},
		{
			name: "permitio_role_assignment",
			config: `
resource "permitio_role_assignment" "alice_editor" {
  user   = "alice"
  role   = "` + permitID + `"
  tenant = "acme"
}
`,
			expectError: idError("role"),
		},
		{
			name: "permitio_resource_instance_role_assignment",
			config: `
resource "permitio_resource_instance_role_assignment" "alice_reads_handbook" {
  user              = "alice"
  role              = "reader"
  resource          = "` + permitID + `"
  resource_instance = "handbook"
  tenant            = "acme"
}
`,
			expectError: idError("resource"),
		},
		{
			name: "permitio_group_resource_instance_role_assignment",
			config: `
resource "permitio_group_resource_instance_role_assignment" "readers_read_handbook" {
  group             = "readers"
  role              = "` + permitID + `"
  resource          = "document"
  resource_instance = "handbook"
  tenant            = "acme"
}
`,
			expectError: idError("role"),
		},
		{
			name: "permitio_resource_instance without tenant",
			config: `
resource "permitio_resource_instance" "handbook" {
  key      = "handbook"
  resource = "document"
}
`,
			expectError: regexp.MustCompile(`The argument "tenant" is required`),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mockpermit.New(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: tt.config, PlanOnly: true, ExpectError: tt.expectError},
				},
			})
		})
	}
}

// TestKeyOnlyArgumentWithIDUnknownAtPlan applies a relation that names its
// subject resource by the id of a resource the same apply creates. The ID is
// unknown at plan, so the plan passes and the apply creates both resources; then
// Terraform validates the relation with the ID known, and the apply fails before
// the provider creates the relation.
func TestKeyOnlyArgumentWithIDUnknownAtPlan(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceRelations)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: `
resource "permitio_resource" "folder" {
  key     = "folder"
  name    = "Folder"
  actions = { list = { name = "List" } }
}

resource "permitio_resource" "file" {
  key     = "file"
  name    = "File"
  actions = { read = { name = "Read" } }
}

resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent folder"
  subject_resource = permitio_resource.folder.id
  object_resource  = permitio_resource.file.key
}
`,
				ExpectError: regexp.MustCompile(`Expected a key, got an ID[\s\S]*` +
					`subject_resource\s+is\s+"[0-9a-f-]{36}"`),
			},
		},
	})

	if got := len(m.Requests(http.MethodPost, mockSchemaPath+"/resources")); got != 2 {
		t.Errorf("the apply created %d resources, want 2: the plan must pass while the "+
			"ID is unknown", got)
	}
	relations := mockSchemaPath + "/resources/file/relations"
	if got := len(m.Requests(http.MethodPost, relations)); got != 0 {
		t.Errorf("the apply sent %d relation creates, want none", got)
	}
}
