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
	UpdatedAt        types.String `tfsdk:"updated_at"`
}

func tfModelFromRelationshipTupleRead(tuple models.RelationshipTupleRead) ResourceInstanceRoleAssignmentModel {
	user := strings.TrimPrefix(tuple.Subject, "__user:")

	resource := tuple.Object
	resourceInstance := ""
	if parts := strings.SplitN(tuple.Object, ":", 2); len(parts) == 2 {
		resource = parts[0]
		resourceInstance = parts[1]
	}

	return ResourceInstanceRoleAssignmentModel{
		Id:               types.StringValue(tuple.Id),
		OrganizationId:   types.StringValue(tuple.OrganizationId),
		ProjectId:        types.StringValue(tuple.ProjectId),
		EnvironmentId:    types.StringValue(tuple.EnvironmentId),
		User:             types.StringValue(user),
		Role:             types.StringValue(tuple.Relation),
		Tenant:           types.StringValue(tuple.Tenant),
		Resource:         types.StringValue(resource),
		ResourceInstance: types.StringValue(resourceInstance),
		CreatedAt:        types.StringValue(tuple.CreatedAt.String()),
		UpdatedAt:        types.StringValue(tuple.UpdatedAt.String()),
	}
}
