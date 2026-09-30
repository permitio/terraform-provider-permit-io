package common

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestImportState imports IDs in a one-attribute and a two-attribute format, and
// checks the attributes each sets, or the error it fails with: a one-attribute ID
// is not split, and only a composite format's error says that a part cannot
// contain ":".
func TestImportState(t *testing.T) {
	const composite = ` Each part must be non-empty and cannot contain ":", so an ` +
		`object whose key contains ":" cannot be imported.`
	for _, tt := range []struct {
		format, id string
		// want is the attributes the import sets, when it succeeds.
		want map[string]string
		// wantError is the detail of the error the import fails with.
		wantError string
	}{
		{format: "key", id: "billing", want: map[string]string{"key": "billing"}},
		{format: "key", id: "billing:eu", want: map[string]string{"key": "billing:eu"}},
		{
			format: "key", id: "",
			wantError: `Expected an import ID in the format "key", got "".`,
		},
		{
			format: "object_resource:key", id: "file:parent",
			want: map[string]string{"object_resource": "file", "key": "parent"},
		},
		{
			format: "object_resource:key", id: "file",
			wantError: `Expected an import ID in the format "object_resource:key", got "file".` +
				composite,
		},
		{
			format: "object_resource:key", id: "file:parent:x",
			wantError: `Expected an import ID in the format "object_resource:key", ` +
				`got "file:parent:x".` + composite,
		},
		{
			format: "object_resource:key", id: ":parent",
			wantError: `Expected an import ID in the format "object_resource:key", got ":parent".` +
				composite,
		},
		{
			format: "object_resource:key", id: "file:",
			wantError: `Expected an import ID in the format "object_resource:key", got "file:".` +
				composite,
		},
	} {
		ctx := context.Background()
		stateSchema := schema.Schema{Attributes: map[string]schema.Attribute{
			"key":             schema.StringAttribute{Required: true},
			"object_resource": schema.StringAttribute{Optional: true},
		}}
		response := resource.ImportStateResponse{State: tfsdk.State{
			Schema: stateSchema,
			Raw:    tftypes.NewValue(stateSchema.Type().TerraformType(ctx), nil),
		}}

		ImportState(ctx, tt.format, resource.ImportStateRequest{ID: tt.id}, &response)

		if tt.wantError != "" {
			errs := response.Diagnostics.Errors()
			if len(errs) != 1 || errs[0].Summary() != "Invalid import ID" ||
				errs[0].Detail() != tt.wantError {
				t.Errorf("ImportState(%q, %q) diagnostics = %v, want one error %q", tt.format,
					tt.id, response.Diagnostics, tt.wantError)
			}
			continue
		}
		if response.Diagnostics.HasError() {
			t.Errorf("ImportState(%q, %q) failed: %v", tt.format, tt.id, response.Diagnostics)
			continue
		}
		for attribute, want := range tt.want {
			var got string
			response.Diagnostics.Append(
				response.State.GetAttribute(ctx, path.Root(attribute), &got)...)
			if got != want {
				t.Errorf("ImportState(%q, %q) set %s = %q, want %q", tt.format, tt.id,
					attribute, got, want)
			}
		}
	}
}
