package resource_instance_role_assignments

import (
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
	ResourceInstance types.String `tfsdk:"resource_instance"`
	CreatedAt        types.String `tfsdk:"created_at"`
	UpdatedAt        types.String `tfsdk:"updated_at"`
}

func tfModelFromRelationshipTupleRead(tuple models.RelationshipTupleRead) ResourceInstanceRoleAssignmentModel {
	return ResourceInstanceRoleAssignmentModel{
		Id:               types.StringValue(tuple.Id),
		OrganizationId:   types.StringValue(tuple.OrganizationId),
		ProjectId:        types.StringValue(tuple.ProjectId),
		EnvironmentId:    types.StringValue(tuple.EnvironmentId),
		User:             types.StringValue(tuple.Subject),
		Role:             types.StringValue(tuple.Relation),
		Tenant:           types.StringValue(tuple.Tenant),
		ResourceInstance: types.StringValue(tuple.Object),
		CreatedAt:        types.StringValue(tuple.CreatedAt.String()),
		UpdatedAt:        types.StringValue(tuple.UpdatedAt.String()),
	}
}
