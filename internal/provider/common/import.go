package common

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// ImportState sets the attributes of an imported object from its import ID. The
// format names those attributes in the order the ID gives them, separated by ":",
// such as "object_resource:key". With one attribute, the whole ID is its value,
// so a key that contains ":" can be imported. With more, the ID must have one
// non-empty part for each attribute: an object with a key that contains ":"
// cannot be imported, and its ID fails with an error that gives the format.
func ImportState(ctx context.Context, format string, request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	attributes := strings.Split(format, ":")
	parts := []string{request.ID}
	if len(attributes) > 1 {
		parts = strings.Split(request.ID, ":")
	}
	if len(parts) != len(attributes) || slices.Contains(parts, "") {
		detail := fmt.Sprintf("Expected an import ID in the format %q, got %q.", format, request.ID)
		if len(attributes) > 1 {
			detail += " Each part must be non-empty and cannot contain \":\", so an " +
				"object whose key contains \":\" cannot be imported."
		}
		response.Diagnostics.AddError("Invalid import ID", detail)
		return
	}
	for i, attribute := range attributes {
		response.Diagnostics.Append(
			response.State.SetAttribute(ctx, path.Root(attribute), parts[i])...)
	}
}
