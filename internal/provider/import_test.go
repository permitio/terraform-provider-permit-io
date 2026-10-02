package provider

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// importCase is how the object of a deleted object case is imported.
type importCase struct {
	// id is the import ID of the object at the case's address.
	id string
	// identifier is the attribute ImportStateVerify finds the imported object by,
	// for a resource type that has no id attribute.
	identifier string
}

// importCases returns how to import the object of each deleted object case, by
// case name. None needs ImportStateVerifyIgnore: Read fills every attribute from
// the API, the proxy config's secret included, as the API returns it unmasked.
func importCases() map[string]importCase {
	return map[string]importCase{
		"resource":           {id: "document"},
		"role":               {id: "admin"},
		"user set":           {id: "reviewers"},
		"resource set":       {id: "drafts"},
		"condition set rule": {id: "reviewers,document:read,drafts"},
		"proxy config":       {id: "billing"},
		"relation":           {id: "file:parent"},
		// A role derivation has no ID of its own in the API, and no id attribute.
		"role derivation": {
			id: "file:editor:folder:manager:parent", identifier: "to_role",
		},
		"tenant":                                  {id: "acme"},
		"user attribute":                          {id: "department"},
		"role assignment":                         {id: "alice:editor:acme"},
		"resource instance":                       {id: "document:handbook"},
		"resource instance role assignment":       {id: "alice:reader:document:handbook:acme"},
		"group resource instance role assignment": {id: "readers:reader:document:handbook:acme"},
	}
}

// TestImportEveryResource creates the object of each deleted object case, which
// has one case for each resource type, imports it with terraform import into an
// empty state, and checks that the imported state has the same attributes as the
// state the create left.
func TestImportEveryResource(t *testing.T) {
	cases := deletedObjectCases()
	imports := importCases()
	if len(imports) != len(cases) {
		t.Errorf("there are %d import cases and %d deleted object cases, want one import "+
			"case for each", len(imports), len(cases))
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			imported, ok := imports[c.name]
			if !ok {
				t.Fatalf("no import case for %s", c.name)
			}
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
						ResourceName:                         c.address,
						ImportState:                          true,
						ImportStateId:                        imported.id,
						ImportStateVerify:                    true,
						ImportStateVerifyIdentifierAttribute: imported.identifier,
					},
				},
			})
		})
	}
}

// TestImportRejectsMalformedIDs imports each resource type with IDs that have too
// few or too many parts, or an empty part, and checks that the import fails with
// an error that gives the format of the type's import ID.
func TestImportRejectsMalformedIDs(t *testing.T) {
	keyFormat := `Expected an import ID in the format "key", got ""`
	cases := map[string]struct {
		ids    []string
		format string
	}{
		"permitio_resource":       {ids: []string{""}, format: keyFormat},
		"permitio_user_set":       {ids: []string{""}, format: keyFormat},
		"permitio_resource_set":   {ids: []string{""}, format: keyFormat},
		"permitio_proxy_config":   {ids: []string{""}, format: keyFormat},
		"permitio_tenant":         {ids: []string{""}, format: keyFormat},
		"permitio_user_attribute": {ids: []string{""}, format: keyFormat},
		"permitio_relation": {
			ids:    []string{"", "file", "file:parent:x", ":parent", "file:"},
			format: `"object_resource:key"`,
		},
		"permitio_role_derivation": {
			ids: []string{
				"file:editor:folder:manager", "file:editor:folder:manager:parent:x",
				"file::folder:manager:parent", "file:editor:folder:manager:",
			},
			format: `"resource:to_role:on_resource:role:linked_by"`,
		},
		"permitio_role_assignment": {
			ids:    []string{"alice:editor", "alice:editor:acme:x", "alice::acme", ":editor:acme"},
			format: `"user:role:tenant"`,
		},
		"permitio_resource_instance_role_assignment": {
			ids: []string{
				"alice:reader:document:acme", "alice:reader:document:handbook:acme:x",
				"alice:reader:document::acme",
			},
			format: `"user:role:resource:resource_instance:tenant"`,
		},
		"permitio_group_resource_instance_role_assignment": {
			ids: []string{
				"readers:reader:document:acme", "readers:reader:document:handbook:acme:x",
				"readers:reader::handbook:acme",
			},
			format: `"group:role:resource:resource_instance:tenant"`,
		},
		// The IDs below are checked against the example of the format that the
		// errors of these resource types give.
		"permitio_role": {
			ids:    []string{"", ":editor", "document:", "document:editor:x"},
			format: `terraform import permitio_role.editor "document:editor"`,
		},
		"permitio_resource_instance": {
			ids:    []string{"document", ":handbook", "document:"},
			format: "Expected format: resource_key:instance_key",
		},
		"permitio_condition_set_rule": {
			ids:    []string{"reviewers,document:read", "reviewers,,drafts", ",document:read,drafts"},
			format: `permitio_condition_set_rule.example "admins,document:read,sensitive-docs"`,
		},
	}
	var served []string
	for _, newResource := range (&PermitProvider{}).Resources(context.Background()) {
		var metadata fwresource.MetadataResponse
		newResource().Metadata(context.Background(),
			fwresource.MetadataRequest{ProviderTypeName: "permitio"}, &metadata)
		served = append(served, metadata.TypeName)
	}
	slices.Sort(served)
	if covered := slices.Sorted(maps.Keys(cases)); len(served) == 0 ||
		!slices.Equal(covered, served) {
		t.Errorf("malformed ID cases cover %q, want one case for each resource type %q",
			covered, served)
	}
	for resourceType, c := range cases {
		for _, id := range c.ids {
			t.Run(fmt.Sprintf("%s/%q", resourceType, id), func(t *testing.T) {
				_, diags := importState(t, resourceType, id)
				if !diags.HasError() {
					t.Fatalf("importing %s with ID %q succeeded, want an error", resourceType, id)
				}
				var details []string
				for _, d := range diags.Errors() {
					details = append(details, d.Summary()+": "+d.Detail())
				}
				detail := strings.Join(details, "\n")
				if !strings.Contains(detail, c.format) {
					t.Errorf("importing %s with ID %q failed with %q, want it to contain %q",
						resourceType, id, detail, c.format)
				}
			})
		}
	}
}

// TestImportByKeyTakesTheWholeID imports each resource type whose import ID is a
// key with a key that contains ":", and checks that the whole ID becomes the key:
// only a composite ID is split.
func TestImportByKeyTakesTheWholeID(t *testing.T) {
	for _, resourceType := range []string{
		"permitio_resource", "permitio_user_set", "permitio_resource_set",
		"permitio_proxy_config", "permitio_tenant", "permitio_user_attribute",
	} {
		t.Run(resourceType, func(t *testing.T) {
			state, diags := importState(t, resourceType, "billing:eu")
			if diags.HasError() {
				t.Fatalf("importing %s with ID billing:eu failed: %v", resourceType, diags)
			}
			var key string
			diags = state.GetAttribute(context.Background(), path.Root("key"), &key)
			if diags.HasError() {
				t.Fatalf("reading the imported key: %v", diags)
			}
			if key != "billing:eu" {
				t.Errorf("importing %s with ID billing:eu set key %q, want billing:eu",
					resourceType, key)
			}
		})
	}
}

// importState runs the ImportState method of resourceType with id, as Terraform
// does before it reads the imported object, and returns the state it leaves for
// Read, and its diagnostics.
func importState(t *testing.T, resourceType, id string) (tfsdk.State, diag.Diagnostics) {
	t.Helper()
	ctx := context.Background()
	for _, newResource := range (&PermitProvider{}).Resources(ctx) {
		r := newResource()
		var metadata fwresource.MetadataResponse
		r.Metadata(ctx, fwresource.MetadataRequest{ProviderTypeName: "permitio"}, &metadata)
		if metadata.TypeName != resourceType {
			continue
		}
		importer, ok := r.(fwresource.ResourceWithImportState)
		if !ok {
			t.Fatalf("%s does not support import", resourceType)
		}
		var schema fwresource.SchemaResponse
		r.Schema(ctx, fwresource.SchemaRequest{}, &schema)
		response := fwresource.ImportStateResponse{State: tfsdk.State{
			Schema: schema.Schema,
			Raw:    tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil),
		}}
		importer.ImportState(ctx, fwresource.ImportStateRequest{ID: id}, &response)
		return response.State, response.Diagnostics
	}
	t.Fatalf("the provider has no resource type %s", resourceType)
	return tfsdk.State{}, nil
}
