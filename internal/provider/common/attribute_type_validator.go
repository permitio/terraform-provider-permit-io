package common

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/permitio/permit-golang/pkg/models"
)

type AttributeTypeValidator struct{}

func (a AttributeTypeValidator) Description(ctx context.Context) string {
	return "The type of the attribute in the resource."
}

func (a AttributeTypeValidator) MarkdownDescription(ctx context.Context) string {
	return "The type of the attribute in the resource."
}

func (a AttributeTypeValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	// Skip validation for unknown values (e.g., during plan phase with variables)
	if request.ConfigValue.IsUnknown() {
		return
	}
	
	if request.ConfigValue.IsNull() {
		response.Diagnostics.AddError("Invalid resource attribute type",
			fmt.Sprintf("Invalid null resource attribute type: %s", request.Path.String()),
		)
		return
	}

	value := request.ConfigValue.ValueString()
	if !models.AttributeType(value).IsValid() {
		response.Diagnostics.AddError("Invalid resource attribute type",
			fmt.Sprintf("Invalid resource attribute type: %s. Valid types are: %v", value, models.AllowedAttributeTypeEnumValues),
		)
		return
	}
}
