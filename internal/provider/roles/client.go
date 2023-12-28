package roles

import (
	"context"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

type roleClient struct {
	client *permit.Client
}

func (c *roleClient) Create(ctx context.Context, plan roleModel) (roleModel, error) {
	permissions, err := common.ConvertElementsToSlice[string](ctx, plan.Permissions.Elements())

	if err != nil {
		return roleModel{}, err
	}

	extends, err := common.ConvertElementsToSlice[string](ctx, plan.Extends.Elements())

	if err != nil {
		return roleModel{}, err
	}

	var createdModel roleModel
	if plan.isResourceRole() {
		roleCreate := models.ResourceRoleCreate{
			Key:         plan.Key.ValueString(),
			Name:        plan.Name.ValueString(),
			Description: plan.Description.ValueStringPointer(),
			Permissions: permissions,
			Extends:     extends,
		}

		createdRole, err := c.client.Api.ResourceRoles.Create(ctx, plan.Resource.ValueString(), roleCreate)

		if err != nil {
			return roleModel{}, err
		}

		createdModel = tfModelFromResourceRoleRead(plan.Resource.ValueString(), *createdRole)
	} else {
		roleCreate := models.RoleCreate{
			Key:         plan.Key.ValueString(),
			Name:        plan.Name.ValueString(),
			Description: plan.Description.ValueStringPointer(),
			Permissions: permissions,
			Extends:     extends,
		}

		createdRole, err := c.client.Api.Roles.Create(ctx, roleCreate)

		if err != nil {
			return roleModel{}, err
		}

		createdModel = tfModelFromRoleRead(*createdRole)
	}

	return createdModel, nil
}

func (c *roleClient) Read(ctx context.Context, key string, resourceKey *string) (roleModel, error) {
	var createdModel roleModel

	if resourceKey != nil {
		roleRead, err := c.client.Api.ResourceRoles.Get(ctx, *resourceKey, key)

		if err != nil {
			return roleModel{}, err
		}

		createdModel = tfModelFromResourceRoleRead(*resourceKey, *roleRead)
	} else {
		roleRead, err := c.client.Api.Roles.Get(ctx, key)

		if err != nil {
			return roleModel{}, err
		}

		createdModel = tfModelFromRoleRead(*roleRead)
	}

	return createdModel, nil
}

func (c *roleClient) Update(ctx context.Context, plan roleModel) (roleModel, error) {
	permissions, err := common.ConvertElementsToSlice[string](ctx, plan.Permissions.Elements())

	if err != nil {
		return roleModel{}, err
	}

	extends, err := common.ConvertElementsToSlice[string](ctx, plan.Extends.Elements())

	if err != nil {
		return roleModel{}, err
	}

	var updatedModel roleModel
	if plan.isResourceRole() {
		roleUpdate := models.ResourceRoleUpdate{
			Name:        plan.Name.ValueStringPointer(),
			Description: plan.Description.ValueStringPointer(),
			Permissions: permissions,
			Extends:     extends,
		}

		updatedRole, err := c.client.Api.ResourceRoles.Update(ctx, plan.Resource.ValueString(), plan.Key.ValueString(), roleUpdate)

		if err != nil {
			return roleModel{}, err
		}

		updatedModel = tfModelFromResourceRoleRead(plan.Resource.ValueString(), *updatedRole)
	} else {
		roleUpdate := models.RoleUpdate{
			Name:        plan.Name.ValueStringPointer(),
			Description: plan.Description.ValueStringPointer(),
			Permissions: permissions,
			Extends:     extends,
		}

		updatedRole, err := c.client.Api.Roles.Update(ctx, plan.Key.ValueString(), roleUpdate)

		if err != nil {
			return roleModel{}, err
		}

		updatedModel = tfModelFromRoleRead(*updatedRole)
	}

	return updatedModel, nil
}

func (c *roleClient) Delete(ctx context.Context, key string, resourceKey *string) error {
	if resourceKey != nil {
		return c.client.Api.ResourceRoles.Delete(ctx, *resourceKey, key)
	} else {
		return c.client.Api.Roles.Delete(ctx, key)
	}
}
