package user_attributes

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
)

const UserKey = "__user"

type userAttributeModel struct {
	Id             types.String `tfsdk:"id"`
	OrganizationId types.String `tfsdk:"organization_id"`
	ProjectId      types.String `tfsdk:"project_id"`
	EnvironmentId  types.String `tfsdk:"environment_id"`

	ResourceId  types.String `tfsdk:"resource_id"`
	ResourceKey types.String `tfsdk:"resource_key"` // Will always be "__user"

	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`

	Type        types.String `tfsdk:"type"`
	Key         types.String `tfsdk:"key"`
	Description types.String `tfsdk:"description"`
}

func tfModelFromSDK(m models.ResourceAttributeRead) userAttributeModel {
	return userAttributeModel{
		Id:             types.StringValue(m.Id),
		OrganizationId: types.StringValue(m.OrganizationId),
		ProjectId:      types.StringValue(m.ProjectId),
		EnvironmentId:  types.StringValue(m.EnvironmentId),

		ResourceId:  types.StringValue(m.ResourceId),
		ResourceKey: types.StringValue(UserKey), // Will always be "__user"

		CreatedAt: types.StringValue(m.CreatedAt.String()),
		UpdatedAt: types.StringValue(m.UpdatedAt.String()),

		Type:        types.StringValue(string(m.Type)),
		Key:         types.StringValue(m.Key),
		Description: types.StringValue(*m.Description),
	}
}
