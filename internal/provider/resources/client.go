package resources

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

type ResourceClient struct {
	client *permit.Client
}

type ResourceMethods interface {
	ResourceRead(ctx context.Context, data ResourceModel) (ResourceModel, error)
	ResourceCreate(ctx context.Context, actions map[string]models.ActionBlockEditable, resourcePlan *ResourceModel) error
	ResourceUpdate(ctx context.Context, resourcePlan *ResourceModel) error
}

func (d *ResourceClient) ResourceRead(ctx context.Context, data ResourceModel) (ResourceModel, error) {
	var resourceKeyOrId string
	if data.Key.IsNull() {
		resourceKeyOrId = data.Id.ValueString()
	} else {
		resourceKeyOrId = data.Key.ValueString()
	}

	resource, err := d.client.Api.Resources.Get(ctx, resourceKeyOrId)

	if err != nil {
		return ResourceModel{}, err
	}

	var (
		urn         types.String
		description types.String
		actions     map[string]actionsModel
	)

	if resource.Urn == nil {
		urn = types.StringNull()
	} else {
		urn = types.StringValue(*resource.Urn)
	}

	if resource.Description == nil {
		description = types.StringNull()
	} else {
		description = types.StringValue(*resource.Description)
	}

	if resource.Actions != nil {
		actions = make(map[string]actionsModel)
		for key, action := range *resource.Actions {
			actionName := *action.Name
			actionNew := actionsModel{
				Id:   types.StringValue(action.Id),
				Name: types.StringValue(actionName),
			}
			if action.Description == nil {
				actionNew.Description = types.StringNull()
			} else {
				actionNew.Description = types.StringValue(*action.Description)
			}
			actions[key] = actionNew
		}

	}

	state := ResourceModel{
		Id:             types.StringValue(resource.Id),
		OrganizationId: types.StringValue(resource.OrganizationId),
		ProjectId:      types.StringValue(resource.ProjectId),
		EnvironmentId:  types.StringValue(resource.EnvironmentId),
		CreatedAt:      types.StringValue(resource.CreatedAt.String()),
		UpdatedAt:      types.StringValue(resource.UpdatedAt.String()),
		Key:            types.StringValue(resource.Key),
		Name:           types.StringValue(resource.Name),
		Urn:            urn,
		Description:    description,
		Actions:        actions,
	}
	return state, nil
}

func (r *ResourceClient) ResourceCreate(ctx context.Context, resourcePlan *ResourceModel) error {
	var (
		actions map[string]models.ActionBlockEditable
		urn     *string
	)
	actions = make(map[string]models.ActionBlockEditable)
	for actionKey, action := range resourcePlan.Actions {
		actions[actionKey] = models.ActionBlockEditable{
			Name:        action.Name.ValueStringPointer(),
			Description: action.Description.ValueStringPointer(),
		}
	}
	urn = nil
	if !resourcePlan.Urn.IsUnknown() {
		urn = resourcePlan.Urn.ValueStringPointer()
	}
	resourceCreate := models.ResourceCreate{
		Key:         resourcePlan.Key.ValueString(),
		Name:        resourcePlan.Name.ValueString(),
		Urn:         urn,
		Description: resourcePlan.Description.ValueStringPointer(),
		Actions:     actions,
		Attributes:  nil,
	}
	tflog.Info(ctx, fmt.Sprint("Creating resource: %v", resourceCreate))
	resourceRead, err := r.client.Api.Resources.Create(ctx, resourceCreate)
	if err != nil {
		return err
	}
	actionsRead := make(map[string]actionsModel)
	for key, action := range *resourceRead.Actions {
		actionsRead[key] = actionsModel{
			Id:          types.StringValue(action.Id),
			Name:        types.StringPointerValue(action.Name),
			Description: types.StringPointerValue(action.Description),
		}
	}
	resourcePlan.Actions = actionsRead
	resourcePlan.Urn = types.StringPointerValue(resourceRead.Urn)
	resourcePlan.Description = types.StringPointerValue(resourceRead.Description)
	resourcePlan.CreatedAt = types.StringValue(resourceRead.CreatedAt.String())
	resourcePlan.UpdatedAt = types.StringValue(resourceRead.UpdatedAt.String())
	resourcePlan.Id = types.StringValue(resourceRead.Id)
	resourcePlan.OrganizationId = types.StringValue(resourceRead.OrganizationId)
	resourcePlan.ProjectId = types.StringValue(resourceRead.ProjectId)
	resourcePlan.EnvironmentId = types.StringValue(resourceRead.EnvironmentId)
	return nil
}

func (r *ResourceClient) ResourceUpdate(ctx context.Context, resourcePlan *ResourceModel, resourceState *ResourceModel) error {
	actions := make(map[string]models.ActionBlockEditable)
	for actionKey, action := range resourcePlan.Actions {
		// TODO: Known bug with Go SDK - null description doesn't get updated correctly
		actions[actionKey] = models.ActionBlockEditable{
			Name:        action.Name.ValueStringPointer(),
			Description: action.Description.ValueStringPointer(),
		}
	}
	resourceUpdate := models.ResourceUpdate{
		Name:        resourcePlan.Name.ValueStringPointer(),
		Urn:         resourceState.Urn.ValueStringPointer(),
		Description: resourcePlan.Description.ValueStringPointer(),
		Actions:     &actions,
	}
	for actionKey, action := range *resourceUpdate.Actions {
		tflog.Info(ctx, fmt.Sprintf("Updating action: %s, %v", actionKey, action))
	}
	resourceRead, err := r.client.Api.Resources.Update(ctx, resourcePlan.Key.ValueString(), resourceUpdate)
	if err != nil {
		return err
	}

	resourcePlan.Name = types.StringValue(resourceRead.Name)
	if resourceRead.Urn != nil {
		resourcePlan.Urn = types.StringValue(*resourceRead.Urn)
	}
	if resourceRead.Description != nil {
		resourcePlan.Description = types.StringValue(*resourceRead.Description)
	}
	if resourceRead.Actions != nil {
		actions := make(map[string]actionsModel)
		for actionKey, action := range *resourceRead.Actions {
			var (
				name        types.String
				description types.String
			)
			if action.Name != nil {
				name = types.StringValue(*action.Name)
			} else {
				name = types.StringValue(actionKey)
			}
			if action.Description != nil {
				description = types.StringValue(*action.Description)
			} else {
				description = types.StringNull()
			}
			actions[actionKey] = actionsModel{
				Id:          types.StringValue(action.Id),
				Name:        name,
				Description: description,
			}
		}
		resourcePlan.Actions = actions
		resourcePlan.UpdatedAt = types.StringValue(resourceRead.UpdatedAt.String())
		resourcePlan.CreatedAt = types.StringValue(resourceRead.CreatedAt.String())
		resourcePlan.EnvironmentId = types.StringValue(resourceRead.EnvironmentId)
		resourcePlan.ProjectId = types.StringValue(resourceRead.ProjectId)
		resourcePlan.Id = types.StringValue(resourceRead.Id)
		resourcePlan.OrganizationId = types.StringValue(resourceRead.OrganizationId)

	}
	return nil
}
