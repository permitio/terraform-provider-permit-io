package conditionsets

import (
	"context"
	"encoding/json"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

type ConditionSetModel struct {
	Id             types.String `tfsdk:"id"`
	OrganizationId types.String `tfsdk:"organization_id"`
	ProjectId      types.String `tfsdk:"project_id"`
	EnvironmentId  types.String `tfsdk:"environment_id"`
	Key            types.String `tfsdk:"key"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Conditions     types.String `tfsdk:"conditions"`
	Resource       types.String `tfsdk:"resource"`
}

type ConditionSetClient struct {
	client *permit.Client
}

func (c *ConditionSetClient) Read(ctx context.Context, data ConditionSetModel) (ConditionSetModel, error) {
	var keyOrId string

	if data.Key.IsNull() {
		keyOrId = data.Id.ValueString()
	} else {
		keyOrId = data.Key.ValueString()
	}

	conditionSet, err := c.client.Api.ConditionSets.Get(ctx, keyOrId)

	if err != nil {
		return ConditionSetModel{}, err
	}

	conditionsMarshalled, err := json.Marshal(conditionSet.Conditions)

	if err != nil {
		return ConditionSetModel{}, err
	}

	var resourceKey string

	if conditionSet.Resource != nil {
		resourceKey = conditionSet.Resource.Key
	}

	state := ConditionSetModel{
		Id:             types.StringValue(conditionSet.Id),
		OrganizationId: types.StringValue(conditionSet.OrganizationId),
		ProjectId:      types.StringValue(conditionSet.ProjectId),
		EnvironmentId:  types.StringValue(conditionSet.EnvironmentId),
		Key:            types.StringValue(conditionSet.Key),
		Name:           types.StringValue(conditionSet.Name),
		Description:    types.StringPointerValue(conditionSet.Description),
		Resource:       types.StringValue(resourceKey),
		Conditions:     types.StringValue(string(conditionsMarshalled)),
	}

	return state, nil
}

func (c *ConditionSetClient) Create(ctx context.Context, conditionSetType models.ConditionSetType, conditionSetPlan *ConditionSetModel) error {
	var conditions map[string]any
	err := json.Unmarshal([]byte(conditionSetPlan.Conditions.ValueString()), &conditions)

	if err != nil {
		return err
	}

	conditionSetCreate := models.ConditionSetCreate{
		Key:         conditionSetPlan.Key.ValueString(),
		Name:        conditionSetPlan.Name.ValueString(),
		Description: conditionSetPlan.Description.ValueStringPointer(),
		Type:        &conditionSetType,
		Conditions:  conditions,
	}

	if !conditionSetPlan.Resource.IsNull() {
		var resourceId models.ResourceId

		err = json.Unmarshal([]byte(conditionSetPlan.Resource.String()), &resourceId)

		if err != nil {
			return err
		}

		conditionSetCreate.ResourceId = &resourceId
	}

	conditionSetRead, err := c.client.Api.ConditionSets.Create(ctx, conditionSetCreate)

	if err != nil {
		return err
	}

	conditionSetPlan.Description = types.StringPointerValue(conditionSetRead.Description)
	conditionSetPlan.Id = types.StringValue(conditionSetRead.Id)
	conditionSetPlan.OrganizationId = types.StringValue(conditionSetRead.OrganizationId)
	conditionSetPlan.ProjectId = types.StringValue(conditionSetRead.ProjectId)
	conditionSetPlan.EnvironmentId = types.StringValue(conditionSetRead.EnvironmentId)

	return nil
}

func (c *ConditionSetClient) Update(ctx context.Context, conditionSetPlan *ConditionSetModel) error {
	var conditions map[string]any
	err := json.Unmarshal([]byte(conditionSetPlan.Conditions.ValueString()), &conditions)

	if err != nil {
		return err
	}

	csUpdate := models.ConditionSetUpdate{
		Name:        conditionSetPlan.Name.ValueStringPointer(),
		Description: conditionSetPlan.Description.ValueStringPointer(),
		Conditions:  conditions,
	}

	conditionSetRead, err := c.client.Api.ConditionSets.Update(ctx, conditionSetPlan.Key.ValueString(), csUpdate)

	if err != nil {
		return err
	}

	conditionsMarshalled, err := json.Marshal(conditionSetRead.Conditions)

	if err != nil {
		return err
	}

	conditionSetPlan.Name = types.StringValue(conditionSetRead.Name)
	conditionSetPlan.Description = types.StringPointerValue(conditionSetRead.Description)
	conditionSetPlan.EnvironmentId = types.StringValue(conditionSetRead.EnvironmentId)
	conditionSetPlan.ProjectId = types.StringValue(conditionSetRead.ProjectId)
	conditionSetPlan.Id = types.StringValue(conditionSetRead.Id)
	conditionSetPlan.OrganizationId = types.StringValue(conditionSetRead.OrganizationId)
	conditionSetPlan.Conditions = types.StringValue(string(conditionsMarshalled))

	return nil
}

func (c *ConditionSetClient) Delete(ctx context.Context, key string) error {
	return c.client.Api.ConditionSets.Delete(ctx, key)
}
