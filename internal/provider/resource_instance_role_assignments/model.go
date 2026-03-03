package resource_instance_role_assignments

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
)

type ResourceInstanceRoleAssignmentModel struct {
	Id               types.String `tfsdk:"id"`
	OrganizationId   types.String `tfsdk:"organization_id"`
	ProjectId        types.String `tfsdk:"project_id"`
	EnvironmentId    types.String `tfsdk:"environment_id"`
	User             types.String `tfsdk:"user"`
	Role             types.String `tfsdk:"role"`
	Tenant           types.String `tfsdk:"tenant"`
	Resource         types.String `tfsdk:"resource"`
	ResourceInstance types.String `tfsdk:"resource_instance"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

func tfModelFromRoleAssignmentRead(assignment models.RoleAssignmentRead) ResourceInstanceRoleAssignmentModel {
	resource := ""
	resourceInstance := ""
	if assignment.ResourceInstance != nil {
		if parts := strings.SplitN(*assignment.ResourceInstance, ":", 2); len(parts) == 2 {
			resource = parts[0]
			resourceInstance = parts[1]
		}
	}

	return ResourceInstanceRoleAssignmentModel{
		Id:               types.StringValue(assignment.Id),
		OrganizationId:   types.StringValue(assignment.OrganizationId),
		ProjectId:        types.StringValue(assignment.ProjectId),
		EnvironmentId:    types.StringValue(assignment.EnvironmentId),
		User:             types.StringValue(assignment.User),
		Role:             types.StringValue(assignment.Role),
		Tenant:           types.StringValue(assignment.Tenant),
		Resource:         types.StringValue(resource),
		ResourceInstance: types.StringValue(resourceInstance),
		CreatedAt:        types.StringValue(assignment.CreatedAt.String()),
	}
}
