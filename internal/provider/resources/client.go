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
		actions = actionsFromSDK(*resource.Actions)
	}
	attributes = newAttributesModelsFromSDKWithPlan(resource.Attributes, data.Attributes)

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
	if resourceRead.Actions == nil {
		return fmt.Errorf("the API answered the create of resource %q with no actions",
			resourcePlan.Key.ValueString())
	}
	resourcePlan.Actions = actionsFromSDK(*resourceRead.Actions)
	resourcePlan.Attributes = newAttributesModelsFromSDKWithPlan(resourceRead.Attributes, resourcePlan.Attributes)
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

	resourcePlan.Attributes = newAttributesModelsFromSDKWithPlan(resourceRead.Attributes, resourcePlan.Attributes)
	resourcePlan.Name = types.StringValue(resourceRead.Name)
	resourcePlan.Description = types.StringPointerValue(resourceRead.Description)
	resourcePlan.Urn = types.StringPointerValue(resourceRead.Urn)
	if resourceRead.Actions != nil {
		resourcePlan.Actions = actionsFromSDK(*resourceRead.Actions)
	}
	resourcePlan.UpdatedAt = types.StringValue(resourceRead.UpdatedAt.String())
	resourcePlan.CreatedAt = types.StringValue(resourceRead.CreatedAt.String())
	resourcePlan.EnvironmentId = types.StringValue(resourceRead.EnvironmentId)
	resourcePlan.ProjectId = types.StringValue(resourceRead.ProjectId)
	resourcePlan.Id = types.StringValue(resourceRead.Id)
	resourcePlan.OrganizationId = types.StringValue(resourceRead.OrganizationId)

	return nil
}

// actionsFromSDK converts the actions the API returns for a resource. An action
// that the API returns without a name gets its key as the name.
func actionsFromSDK(read map[string]models.ActionBlockRead) map[string]actionsModel {
	actions := make(map[string]actionsModel, len(read))
	for actionKey, action := range read {
		name := types.StringValue(actionKey)
		if action.Name != nil {
			name = types.StringValue(*action.Name)
		}
		actions[actionKey] = actionsModel{
			Id:          types.StringValue(action.Id),
			Name:        name,
			Description: types.StringPointerValue(action.Description),
		}
	}
	return actions
}
