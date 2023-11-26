package roles

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/samber/lo"
)

type roleModel struct {
	Id             types.String `tfsdk:"id"`
	OrganizationId types.String `tfsdk:"organization_id"`
	ProjectId      types.String `tfsdk:"project_id"`
	EnvironmentId  types.String `tfsdk:"environment_id"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
	Key            types.String `tfsdk:"key"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Permissions    types.Set    `tfsdk:"permissions"`
	Extends        types.Set    `tfsdk:"extends"`

	ResourceId types.String `tfsdk:"resource_id"`
	Resource   types.String `tfsdk:"resource"`
}

func (m *roleModel) isResourceRole() bool {
	return !m.Resource.IsNull()
}

func tfModelFromRoleRead(m models.RoleRead) roleModel {
	r := roleModel{}
	r.Id = types.StringValue(m.Id)
	r.Key = types.StringValue(m.Key)
	r.Name = types.StringValue(m.Name)
	r.Description = types.StringPointerValue(m.Description)
	r.EnvironmentId = types.StringValue(m.EnvironmentId)
	r.ProjectId = types.StringValue(m.ProjectId)
	r.OrganizationId = types.StringValue(m.OrganizationId)
	r.CreatedAt = types.StringValue(m.CreatedAt.String())
	r.UpdatedAt = types.StringValue(m.UpdatedAt.String())

	permissionValues := lo.Map(m.Permissions, func(item string, index int) attr.Value {
		return types.StringValue(item)
	})
	r.Permissions = types.SetValueMust(types.StringType, permissionValues)

	extendValues := lo.Map(m.Extends, func(item string, index int) attr.Value {
		return types.StringValue(item)
	})
	r.Extends = types.SetValueMust(types.StringType, extendValues)

	return r
}

func tfModelFromResourceRoleRead(resourceKey string, m models.ResourceRoleRead) roleModel {
	r := roleModel{}
	r.Id = types.StringValue(m.Id)
	r.Key = types.StringValue(m.Key)
	r.Name = types.StringValue(m.Name)
	r.Description = types.StringPointerValue(m.Description)
	r.EnvironmentId = types.StringValue(m.EnvironmentId)
	r.ProjectId = types.StringValue(m.ProjectId)
	r.OrganizationId = types.StringValue(m.OrganizationId)
	r.CreatedAt = types.StringValue(m.CreatedAt.String())
	r.UpdatedAt = types.StringValue(m.UpdatedAt.String())

	permissionValues := lo.Map(m.Permissions, func(item string, index int) attr.Value {
		return types.StringValue(item)
	})
	r.Permissions = types.SetValueMust(types.StringType, permissionValues)

	extendValues := lo.Map(m.Extends, func(item string, index int) attr.Value {
		return types.StringValue(item)
	})
	r.Extends = types.SetValueMust(types.StringType, extendValues)

	r.Resource = types.StringValue(resourceKey)
	r.ResourceId = types.StringValue(m.ResourceId)

	return r
}
