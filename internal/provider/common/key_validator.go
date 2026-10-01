package common

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// idShape matches a value that has the form of a Permit ID: a UUID, in any case.
var idShape = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// KeyNotID returns a validator for an argument that names an object of
// objectType, such as permitio_resource, by its key. It rejects a value that has
// the form of a Permit ID. Use it only where developers choose the keys, such as
// resources, roles and relations, because it also rejects a key that has the form
// of a UUID. Unknown and null values pass.
func KeyNotID(objectType string) validator.String {
	return keyNotIDValidator{objectType: objectType}
}

type keyNotIDValidator struct {
	objectType string
}

func (v keyNotIDValidator) Description(context.Context) string {
	return fmt.Sprintf("value must be the key of a %s, not its ID", v.objectType)
}

func (v keyNotIDValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v keyNotIDValidator) ValidateString(
	_ context.Context, request validator.StringRequest, response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}
	value := request.ConfigValue.ValueString()
	if !idShape.MatchString(value) {
		return
	}
	response.Diagnostics.AddAttributeError(request.Path, "Expected a key, got an ID",
		fmt.Sprintf("%s is %q, which has the form of a Permit ID. %s takes the key of a %s, "+
			"not its ID. Reference the key, such as %s.<name>.key, not its id. A key that "+
			"has the form of a UUID is not accepted here either.",
			request.Path, value, request.Path, v.objectType, v.objectType))
}
