package conditionsets

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// normalizeEmptyStringToNull is a plan modifier that treats empty strings as null
// This ensures consistency between the API behavior and Terraform state
type normalizeEmptyStringToNull struct{}

func (m normalizeEmptyStringToNull) Description(ctx context.Context) string {
	return "Normalizes empty strings to null for consistency with API behavior"
}

func (m normalizeEmptyStringToNull) MarkdownDescription(ctx context.Context) string {
	return "Normalizes empty strings to null for consistency with API behavior"
}

func (m normalizeEmptyStringToNull) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// If the planned value is an empty string, change it to null
	if !req.PlanValue.IsNull() && !req.PlanValue.IsUnknown() {
		if req.PlanValue.ValueString() == "" {
			resp.PlanValue = types.StringNull()
		}
	}
}

// NormalizeEmptyStringToNull returns a plan modifier that normalizes empty strings to null
func NormalizeEmptyStringToNull() planmodifier.String {
	return normalizeEmptyStringToNull{}
}