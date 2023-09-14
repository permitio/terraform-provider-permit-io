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
	ResourceCreate(ctx context.Context, resourcePlan *ResourceModel) error
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
		actions    map[string]actionsModel
		attributes attributesModel
	)

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
	attributes = newAttributesModelsFromSDK(resource.Attributes)

	state := ResourceModel{
		Id:             types.StringValue(resource.Id),
		OrganizationId: types.StringValue(resource.OrganizationId),
		ProjectId:      types.StringValue(resource.ProjectId),
		EnvironmentId:  types.StringValue(resource.EnvironmentId),
		CreatedAt:      types.StringValue(resource.CreatedAt.String()),
		UpdatedAt:      types.StringValue(resource.UpdatedAt.String()),
		Key:            types.StringValue(resource.Key),
		Name:           types.StringValue(resource.Name),
		Urn:            types.StringPointerValue(resource.Urn),
		Description:    types.StringPointerValue(resource.Description),
		Actions:        actions,
		Attributes:     attributes,
	}
	return state, nil
}

func (r *ResourceClient) ResourceCreate(ctx context.Context, resourcePlan *ResourceModel) error {
	var (
		actions    map[string]models.ActionBlockEditable
		attributes map[string]models.AttributeBlockEditable
		urn        *string
	)
	attributes = resourcePlan.Attributes.toSDK()
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
		Attributes:  &attributes,
	}
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
	resourcePlan.Attributes = newAttributesModelsFromSDK(resourceRead.Attributes)
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

func (r *ResourceClient) ResourceUpdate(ctx context.Context, resourcePlan *ResourceModel) error {
	actions := make(map[string]models.ActionBlockEditable)
	for actionKey, action := range resourcePlan.Actions {
		// TODO: Known bug with Go SDK - null description doesn't get updated correctly
		actions[actionKey] = models.ActionBlockEditable{
			Name:        action.Name.ValueStringPointer(),
			Description: action.Description.ValueStringPointer(),
		}
	}
	attributes := resourcePlan.Attributes.toSDK()
	resourceUpdate := models.ResourceUpdate{
		Name:        resourcePlan.Name.ValueStringPointer(),
		Urn:         resourcePlan.Urn.ValueStringPointer(),
		Description: resourcePlan.Description.ValueStringPointer(),
		Actions:     &actions,
		Attributes:  &attributes,
	}
	for actionKey, action := range *resourceUpdate.Actions {
		tflog.Info(ctx, fmt.Sprintf("Updating action: %s, %v", actionKey, action))
	}
	resourceRead, err := r.client.Api.Resources.Update(ctx, resourcePlan.Key.ValueString(), resourceUpdate)
	if err != nil {
		return err
	}

	resourcePlan.Attributes = newAttributesModelsFromSDK(resourceRead.Attributes)
	resourcePlan.Name = types.StringValue(resourceRead.Name)
	resourcePlan.Description = types.StringPointerValue(resourceRead.Description)
	resourcePlan.Urn = types.StringPointerValue(resourceRead.Urn)
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
	}
	resourcePlan.UpdatedAt = types.StringValue(resourceRead.UpdatedAt.String())
	resourcePlan.CreatedAt = types.StringValue(resourceRead.CreatedAt.String())
	resourcePlan.EnvironmentId = types.StringValue(resourceRead.EnvironmentId)
	resourcePlan.ProjectId = types.StringValue(resourceRead.ProjectId)
	resourcePlan.Id = types.StringValue(resourceRead.Id)
	resourcePlan.OrganizationId = types.StringValue(resourceRead.OrganizationId)

	return nil
}
