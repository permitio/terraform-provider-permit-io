package conditionsets

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

// userSetModel holds the attributes of a user set, which a resource set has too.
type userSetModel struct {
	Id             types.String         `tfsdk:"id"`
	OrganizationId types.String         `tfsdk:"organization_id"`
	ProjectId      types.String         `tfsdk:"project_id"`
	EnvironmentId  types.String         `tfsdk:"environment_id"`
	Key            types.String         `tfsdk:"key"`
	Name           types.String         `tfsdk:"name"`
	Description    types.String         `tfsdk:"description"`
	Conditions     jsontypes.Normalized `tfsdk:"conditions"`
	ParentId       types.String         `tfsdk:"parent_id"`
}

// ConditionSetModel holds a user set or a resource set. A user set has no
// resource attribute, so its Resource stays null.
type ConditionSetModel struct {
	userSetModel
	Resource types.String `tfsdk:"resource"`
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

	conditions, err := common.JSONObjectValue(conditionSet.Conditions, data.Conditions)
	if err != nil {
		return ConditionSetModel{}, fmt.Errorf("conditions: %w", err)
	}

	// Handle resource: if API returns null, keep it null to maintain consistency.
	// The API returns the resource's key; keep its ID where the state has that,
	// so that naming the resource by ID does not plan an update on every plan.
	var resource types.String
	switch {
	case conditionSet.Resource == nil:
		resource = types.StringPointerValue(nil)
	case data.Resource.ValueString() == conditionSet.Resource.Id:
		resource = data.Resource
	default:
		resource = types.StringValue(conditionSet.Resource.Key)
	}

	description := descriptionValue(conditionSet.Description, data.Description)

	// Handle parent_id: if API returns null, keep it null to maintain consistency
	var parentId types.String
	if conditionSet.ParentId != nil {
		parentIdBytes, err := json.Marshal(conditionSet.ParentId)
		if err != nil {
			return ConditionSetModel{}, err
		}
		var parentIdStr string
		err = json.Unmarshal(parentIdBytes, &parentIdStr)
		if err != nil {
			return ConditionSetModel{}, err
		}
		parentId = types.StringValue(parentIdStr)
	} else {
		parentId = types.StringPointerValue(nil)
	}

	state := ConditionSetModel{
		userSetModel: userSetModel{
			Id:             types.StringValue(conditionSet.Id),
			OrganizationId: types.StringValue(conditionSet.OrganizationId),
			ProjectId:      types.StringValue(conditionSet.ProjectId),
			EnvironmentId:  types.StringValue(conditionSet.EnvironmentId),
			Key:            types.StringValue(conditionSet.Key),
			Name:           types.StringValue(conditionSet.Name),
			Description:    description,
			ParentId:       parentId,
			Conditions:     conditions,
		},
		Resource: resource,
	}

	return state, nil
}

// NamesSetResource reports whether resource, the ID or the key of a resource,
// names the resource of the resource set with this key.
func (c *ConditionSetClient) NamesSetResource(ctx context.Context, setKey, resource string,
) (bool, error) {
	conditionSet, err := c.client.Api.ConditionSets.Get(ctx, setKey)
	if err != nil {
		return false, err
	}
	current := conditionSet.Resource
	return current != nil && (resource == current.Id || resource == current.Key), nil
}

func (c *ConditionSetClient) Create(ctx context.Context, conditionSetType models.ConditionSetType, conditionSetPlan *ConditionSetModel) error {
	conditions, err := common.DecodeJSONObject(conditionSetPlan.Conditions.ValueString())
	if err != nil {
		return fmt.Errorf("conditions: %w", err)
	}

	conditionSetCreate := models.ConditionSetCreate{
		Key:         conditionSetPlan.Key.ValueString(),
		Name:        conditionSetPlan.Name.ValueString(),
		Description: common.KnownStringPointer(conditionSetPlan.Description),
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

	// Only set parent_id if it's not null and not empty
	if !conditionSetPlan.ParentId.IsNull() && conditionSetPlan.ParentId.ValueString() != "" {
		var parentId models.ParentId
		parentIdStr := conditionSetPlan.ParentId.ValueString()
		err = json.Unmarshal([]byte(fmt.Sprintf("\"%s\"", parentIdStr)), &parentId)

		if err != nil {
			return err
		}

		conditionSetCreate.ParentId = &parentId
	}

	conditionSetRead, err := c.client.Api.ConditionSets.Create(ctx, conditionSetCreate)

	if err != nil {
		return err
	}

	conditionSetPlan.Description = descriptionValue(conditionSetRead.Description,
		conditionSetPlan.Description)
	// Handle parent_id from API response
	if conditionSetRead.ParentId != nil {
		parentIdBytes, err := json.Marshal(conditionSetRead.ParentId)
		if err != nil {
			return err
		}
		var parentIdStr string
		err = json.Unmarshal(parentIdBytes, &parentIdStr)
		if err != nil {
			return err
		}
		conditionSetPlan.ParentId = types.StringValue(parentIdStr)
	} else {
		conditionSetPlan.ParentId = types.StringPointerValue(nil)
	}
	conditionSetPlan.Id = types.StringValue(conditionSetRead.Id)
	conditionSetPlan.OrganizationId = types.StringValue(conditionSetRead.OrganizationId)
	conditionSetPlan.ProjectId = types.StringValue(conditionSetRead.ProjectId)
	conditionSetPlan.EnvironmentId = types.StringValue(conditionSetRead.EnvironmentId)

	return nil
}

func (c *ConditionSetClient) Update(ctx context.Context, conditionSetPlan *ConditionSetModel) error {
	conditions, err := common.DecodeJSONObject(conditionSetPlan.Conditions.ValueString())
	if err != nil {
		return fmt.Errorf("conditions: %w", err)
	}

	// A description the configuration leaves out is sent as "", which clears it:
	// the Go SDK leaves a null description out of the request, and the API keeps
	// the description then.
	description := conditionSetPlan.Description.ValueString()
	csUpdate := models.ConditionSetUpdate{
		Name:        conditionSetPlan.Name.ValueStringPointer(),
		Description: &description,
		Conditions:  conditions,
	}

	// Only set parent_id if it's not null and not empty
	if !conditionSetPlan.ParentId.IsNull() && conditionSetPlan.ParentId.ValueString() != "" {
		var parentId models.ParentId
		parentIdStr := conditionSetPlan.ParentId.ValueString()
		err = json.Unmarshal([]byte(fmt.Sprintf("\"%s\"", parentIdStr)), &parentId)

		if err != nil {
			return err
		}

		csUpdate.ParentId = &parentId
	}

	conditionSetRead, err := c.client.Api.ConditionSets.Update(ctx, conditionSetPlan.Key.ValueString(), csUpdate)

	if err != nil {
		return err
	}

	updatedConditions, err := common.JSONObjectValue(conditionSetRead.Conditions,
		conditionSetPlan.Conditions)
	if err != nil {
		return fmt.Errorf("conditions: %w", err)
	}

	conditionSetPlan.Name = types.StringValue(conditionSetRead.Name)
	conditionSetPlan.Description = descriptionValue(conditionSetRead.Description,
		conditionSetPlan.Description)
	// Handle parent_id from API response
	if conditionSetRead.ParentId != nil {
		parentIdBytes, err := json.Marshal(conditionSetRead.ParentId)
		if err != nil {
			return err
		}
		var parentIdStr string
		err = json.Unmarshal(parentIdBytes, &parentIdStr)
		if err != nil {
			return err
		}
		conditionSetPlan.ParentId = types.StringValue(parentIdStr)
	} else {
		conditionSetPlan.ParentId = types.StringPointerValue(nil)
	}
	conditionSetPlan.EnvironmentId = types.StringValue(conditionSetRead.EnvironmentId)
	conditionSetPlan.ProjectId = types.StringValue(conditionSetRead.ProjectId)
	conditionSetPlan.Id = types.StringValue(conditionSetRead.Id)
	conditionSetPlan.OrganizationId = types.StringValue(conditionSetRead.OrganizationId)
	conditionSetPlan.Conditions = updatedConditions

	return nil
}

func (c *ConditionSetClient) Delete(ctx context.Context, key string) error {
	return c.client.Api.ConditionSets.Delete(ctx, key)
}

// descriptionValue returns the description of a condition set that the API
// answered with, where prior is the description of the plan or the prior state.
// The provider clears a description by sending "", so when the API answers with
// no description or "", the value keeps a null or "" that prior has.
func descriptionValue(api *string, prior types.String) types.String {
	if (api == nil || *api == "") && (prior.IsNull() || prior.Equal(types.StringValue(""))) {
		return prior
	}
	return types.StringPointerValue(api)
}
