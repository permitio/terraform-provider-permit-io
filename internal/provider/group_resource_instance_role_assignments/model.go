package group_resource_instance_role_assignments

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type GroupResourceInstanceRoleAssignmentModel struct {
	Id               types.String `tfsdk:"id"`
	Group            types.String `tfsdk:"group"`
	Role             types.String `tfsdk:"role"`
	Resource         types.String `tfsdk:"resource"`
	ResourceInstance types.String `tfsdk:"resource_instance"`
	Tenant           types.String `tfsdk:"tenant"`
}

// GroupAddRole represents the API request body.
type GroupAddRole struct {
	Role             string `json:"role"`
	Resource         string `json:"resource"`
	ResourceInstance string `json:"resource_instance"`
	Tenant           string `json:"tenant"`
}
