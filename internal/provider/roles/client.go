package roles

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

type RoleClient struct {
	client *permit.Client
}

type RoleMethods interface {
	RoleRead(ctx context.Context, data RoleModel) (RoleModel, error)
	RoleCreate(ctx context.Context, actions map[string]models.ActionBlockEditable, resourcePlan *RoleModel) error
	RoleUpdate(ctx context.Context, resourcePlan *RoleModel) error
}

func (d *RoleClient) RoleRead(ctx context.Context, data RoleModel) (RoleModel, error, diag.Diagnostics) {
	var (
		resourceKeyOrId string
		diags           diag.Diagnostics
	)
	if data.Key.IsNull() {
		resourceKeyOrId = data.Id.ValueString()
	} else {
		resourceKeyOrId = data.Key.ValueString()
	}
	role, err := d.client.Api.Roles.Get(ctx, resourceKeyOrId)
	if err != nil {
		return RoleModel{}, err, diags
	}
	permissions, diagsAddition := types.ListValueFrom(ctx, types.StringType, role.Permissions)
	diags.Append(diagsAddition...)
	extends, diagsAddition := types.ListValueFrom(ctx, types.StringType, role.Extends)
	diags.Append(diagsAddition...)

	state := RoleModel{
		Id:             types.StringValue(role.Id),
		OrganizationId: types.StringValue(role.OrganizationId),
		ProjectId:      types.StringValue(role.ProjectId),
		EnvironmentId:  types.StringValue(role.EnvironmentId),
		CreatedAt:      types.StringValue(role.CreatedAt.String()),
		UpdatedAt:      types.StringValue(role.UpdatedAt.String()),
		Key:            types.StringValue(role.Key),
		Name:           types.StringValue(role.Name),
		Description:    types.StringPointerValue(role.Description),
		Permissions:    permissions,
		Extends:        extends,
	}
	return state, nil, diags
}

func (r *RoleClient) RoleCreate(ctx context.Context, rolePlan *RoleModel) (error, diag.Diagnostics) {
	var diags diag.Diagnostics

	permissions := make([]string, 0)
	for _, permission := range rolePlan.Permissions.Elements() {
		permissions = append(permissions, RemoveDoubleQuotes(permission.String()))
	}
	extends := make([]string, 0)
	for _, extend := range rolePlan.Extends.Elements() {
		extends = append(extends, RemoveDoubleQuotes(extend.String()))

	}
	roleCreate := models.RoleCreate{
		Key:         rolePlan.Key.ValueString(),
		Name:        rolePlan.Name.ValueString(),
		Description: rolePlan.Description.ValueStringPointer(),
		Permissions: permissions,
		Extends:     extends,
		Attributes:  nil, // TODO: Add attributes
	}
	roleRead, err := r.client.Api.Roles.Create(ctx, roleCreate)
	if err != nil {
		return err, diags
	}
	rolePlan.Description = types.StringPointerValue(roleRead.Description)
	rolePlan.CreatedAt = types.StringValue(roleRead.CreatedAt.String())
	rolePlan.UpdatedAt = types.StringValue(roleRead.UpdatedAt.String())
	rolePlan.Id = types.StringValue(roleRead.Id)
	rolePlan.OrganizationId = types.StringValue(roleRead.OrganizationId)
	rolePlan.ProjectId = types.StringValue(roleRead.ProjectId)
	rolePlan.EnvironmentId = types.StringValue(roleRead.EnvironmentId)
	extendsPlan, diagsAddition := types.ListValueFrom(ctx, types.StringType, roleRead.Extends)
	diags.Append(diagsAddition...)
	permissionsPlan, diagsAddition := types.ListValueFrom(ctx, types.StringType, roleRead.Permissions)
	diags.Append(diagsAddition...)
	rolePlan.Extends = extendsPlan
	rolePlan.Permissions = permissionsPlan

	return nil, diags
}

func stringListToAttrList(list []string) []attr.Value {
	attrs := make([]attr.Value, 0)
	for _, item := range list {
		attrs = append(attrs, types.StringValue(item))
	}
	return attrs
}

func (r *RoleClient) RoleUpdate(ctx context.Context, rolePlan *RoleModel) (error, diag.Diagnostics) {
	var (
		diags               diag.Diagnostics
		extendsAsGoType     []string
		permissionsAsGoType []string
	)
	for _, extend := range rolePlan.Extends.Elements() {
		extendsAsGoType = append(extendsAsGoType, RemoveDoubleQuotes(extend.String()))
	}
	for _, permission := range rolePlan.Permissions.Elements() {
		permissionsAsGoType = append(permissionsAsGoType, RemoveDoubleQuotes(permission.String()))
	}
	roleUpdate := models.RoleUpdate{
		Name:        rolePlan.Name.ValueStringPointer(),
		Description: rolePlan.Description.ValueStringPointer(),
		Permissions: permissionsAsGoType,
		Extends:     extendsAsGoType,
	}
	tflog.Info(ctx, fmt.Sprintf("Updating role: %v", permissionsAsGoType))

	roleRead, err := r.client.Api.Roles.Update(ctx, rolePlan.Key.ValueString(), roleUpdate)
	if err != nil {
		return err, diags
	}
	rolePlan.Description = types.StringPointerValue(roleRead.Description)
	rolePlan.Name = types.StringValue(roleRead.Name)
	rolePlan.UpdatedAt = types.StringValue(roleRead.UpdatedAt.String())
	rolePlan.CreatedAt = types.StringValue(roleRead.CreatedAt.String())
	rolePlan.EnvironmentId = types.StringValue(roleRead.EnvironmentId)
	rolePlan.ProjectId = types.StringValue(roleRead.ProjectId)
	rolePlan.Id = types.StringValue(roleRead.Id)
	rolePlan.OrganizationId = types.StringValue(roleRead.OrganizationId)
	extends, diagsAddition := types.ListValueFrom(ctx, types.StringType, roleRead.Extends)
	diags.Append(diagsAddition...)
	permissions, diagsAddition := types.ListValueFrom(ctx, types.StringType, roleRead.Permissions)
	diags.Append(diagsAddition...)
	rolePlan.Extends = extends
	rolePlan.Permissions = permissions

	return nil, diags
}
