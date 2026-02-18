package role_assignments

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
)

type RoleAssignmentModel struct {
	Id             types.String `tfsdk:"id"`
	OrganizationId types.String `tfsdk:"organization_id"`
	ProjectId      types.String `tfsdk:"project_id"`
	EnvironmentId  types.String `tfsdk:"environment_id"`
	User           types.String `tfsdk:"user"`
	Role           types.String `tfsdk:"role"`
	Tenant         types.String `tfsdk:"tenant"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

func tfModelFromRoleAssignmentRead(assignment models.RoleAssignmentRead) RoleAssignmentModel {
	return RoleAssignmentModel{
		Id:             types.StringValue(assignment.Id),
		OrganizationId: types.StringValue(assignment.OrganizationId),
		ProjectId:      types.StringValue(assignment.ProjectId),
		EnvironmentId:  types.StringValue(assignment.EnvironmentId),
		User:           types.StringValue(assignment.User),
		Role:           types.StringValue(assignment.Role),
		Tenant:         types.StringValue(assignment.Tenant),
		CreatedAt:      types.StringValue(assignment.CreatedAt.String()),
	}
}
