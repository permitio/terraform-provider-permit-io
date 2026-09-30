package resources

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/datasourcetest"
)

// TestResourceDataSourceConfigDecodes checks that the permitio_resource data
// source's schema decodes into the model its Read uses.
func TestResourceDataSourceConfigDecodes(t *testing.T) {
	datasourcetest.CheckConfigDecodes(t, NewResourceDataSource(), &ResourceModel{})
}

// TestResourceDataSourceAttributeTypeValidators runs the validators of the data
// source's attributes.type. Terraform validates a configuration before the values
// it references are known, so a type taken from a resource created in the same
// apply is unknown then and must pass; a known type that Permit does not have must
// still fail.
func TestResourceDataSourceAttributeTypeValidators(t *testing.T) {
	var resp datasource.SchemaResponse
	NewResourceDataSource().Schema(t.Context(), datasource.SchemaRequest{}, &resp)
	attributes, ok := resp.Schema.Attributes["attributes"].(schema.MapNestedAttribute)
	if !ok {
		t.Fatalf("attributes is a %T, want a map nested attribute",
			resp.Schema.Attributes["attributes"])
	}
	attributeType, ok := attributes.NestedObject.Attributes["type"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("attributes.type is a %T, want a string attribute",
			attributes.NestedObject.Attributes["type"])
	}
	if len(attributeType.Validators) == 0 {
		t.Fatal("DID NOT RUN: attributes.type has no validators")
	}

	for _, tc := range []struct {
		value     types.String
		wantError bool
	}{
		{value: types.StringUnknown(), wantError: false},
		{value: types.StringValue("number"), wantError: false},
		{value: types.StringValue("not-a-type"), wantError: true},
	} {
		request := validator.StringRequest{
			Path:        path.Root("attributes").AtMapKey("pages").AtName("type"),
			ConfigValue: tc.value,
		}
		var failed bool
		for _, v := range attributeType.Validators {
			var response validator.StringResponse
			v.ValidateString(t.Context(), request, &response)
			failed = failed || response.Diagnostics.HasError()
		}
		if failed != tc.wantError {
			t.Errorf("validating attributes.type %s: got an error %t, want %t", tc.value,
				failed, tc.wantError)
		}
	}
}
