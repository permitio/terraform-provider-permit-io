package common

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestKeyNotID checks that KeyNotID rejects a value shaped like a Permit ID, a
// UUID in any case, and accepts keys, including keys made of hex digits or with
// a UUID inside them, and unknown and null values. A rejection names the argument
// and tells the user to reference the object's key.
func TestKeyNotID(t *testing.T) {
	for _, tt := range []struct {
		name   string
		value  types.String
		reject bool
	}{
		{name: "lower-case UUID", value: types.StringValue("4f6c1e2a-7b3d-4c5e-9f8a-1b2c3d4e5f60"),
			reject: true},
		{name: "upper-case UUID", value: types.StringValue("4F6C1E2A-7B3D-4C5E-9F8A-1B2C3D4E5F60"),
			reject: true},
		{name: "mixed-case UUID", value: types.StringValue("4f6C1e2A-7b3D-4c5E-9f8A-1b2C3d4E5f60"),
			reject: true},
		{name: "key", value: types.StringValue("document")},
		{name: "key of hex digits", value: types.StringValue("deadbeef")},
		{name: "UUID without dashes", value: types.StringValue("4f6c1e2a7b3d4c5e9f8a1b2c3d4e5f60")},
		{name: "key that contains a UUID",
			value: types.StringValue("doc-4f6c1e2a-7b3d-4c5e-9f8a-1b2c3d4e5f60")},
		{name: "UUID followed by a suffix",
			value: types.StringValue("4f6c1e2a-7b3d-4c5e-9f8a-1b2c3d4e5f60-copy")},
		{name: "UUID with a non-hex digit",
			value: types.StringValue("4f6c1e2a-7b3d-4c5e-9f8a-1b2c3d4e5f6g")},
		{name: "empty", value: types.StringValue("")},
		{name: "unknown", value: types.StringUnknown()},
		{name: "null", value: types.StringNull()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := validator.StringRequest{
				Path:        path.Root("subject_resource"),
				ConfigValue: tt.value,
			}
			var response validator.StringResponse
			KeyNotID("permitio_resource").ValidateString(t.Context(), request, &response)

			if !tt.reject {
				if response.Diagnostics.HasError() {
					t.Fatalf("KeyNotID rejected %s: %v", tt.value, response.Diagnostics)
				}
				return
			}
			if response.Diagnostics.ErrorsCount() != 1 {
				t.Fatalf("KeyNotID gave %d errors for %s, want 1: %v",
					response.Diagnostics.ErrorsCount(), tt.value, response.Diagnostics)
			}
			detail := response.Diagnostics.Errors()[0].Detail()
			for _, want := range []string{
				"subject_resource", tt.value.ValueString(), "permitio_resource.<name>.key",
			} {
				if !strings.Contains(detail, want) {
					t.Errorf("the error for %s does not contain %q:\n%s", tt.value, want, detail)
				}
			}
		})
	}
}
