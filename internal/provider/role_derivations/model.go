package role_derivations

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
)

type roleDerivationModel struct {
	Resource         types.String `tfsdk:"resource"`
	Role             types.String `tfsdk:"role"`
	OnResource       types.String `tfsdk:"on_resource"`
	ToRole           types.String `tfsdk:"to_role"`
	LinkedByRelation types.String `tfsdk:"linked_by"`
}

func tfModelFromDerivedRoleRuleRead(plan roleDerivationModel, m models.DerivedRoleRuleRead) roleDerivationModel {
	r := roleDerivationModel{}

	r.Resource = plan.Resource
	r.Role = plan.Role
	r.OnResource = types.StringValue(m.OnResource)
	r.ToRole = types.StringValue(m.Role)
	r.LinkedByRelation = types.StringValue(m.LinkedByRelation)

	return r
}
