package resource_instances

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

type resourceInstanceClient struct {
	client *permit.Client
}

func (c *resourceInstanceClient) Create(ctx context.Context, plan resourceInstanceModel) (resourceInstanceModel, error) {
	// Parse attributes from JSON string
	var attributes map[string]interface{}
	if !plan.Attributes.IsNull() && plan.Attributes.ValueString() != "" {
		err := json.Unmarshal([]byte(plan.Attributes.ValueString()), &attributes)
		if err != nil {
			return resourceInstanceModel{}, err
		}
	}

	instanceCreate := models.NewResourceInstanceCreate(
		plan.Key.ValueString(),
		plan.Resource.ValueString(),
	)

	if !plan.Tenant.IsNull() {
		instanceCreate.SetTenant(plan.Tenant.ValueString())
	}

	if attributes != nil {
		instanceCreate.SetAttributes(attributes)
	}

	created, err := c.client.Api.ResourceInstances.Create(ctx, *instanceCreate)
	if err != nil {
		return resourceInstanceModel{}, err
	}

	return tfModelFromResourceInstanceRead(*created), nil
}

func (c *resourceInstanceClient) Read(ctx context.Context, key string, resource string) (resourceInstanceModel, error) {
	instanceId := fmt.Sprintf("%s:%s", resource, key)
	instance, err := c.client.Api.ResourceInstances.Get(ctx, instanceId)
	if err != nil {
		return resourceInstanceModel{}, err
	}

	return tfModelFromResourceInstanceRead(*instance), nil
}

func (c *resourceInstanceClient) Update(ctx context.Context, plan resourceInstanceModel) (resourceInstanceModel, error) {
	// Parse attributes from JSON string
	var attributes map[string]interface{}
	if !plan.Attributes.IsNull() && plan.Attributes.ValueString() != "" {
		err := json.Unmarshal([]byte(plan.Attributes.ValueString()), &attributes)
		if err != nil {
			return resourceInstanceModel{}, err
		}
	}

	instanceUpdate := models.NewResourceInstanceUpdate()
	if attributes != nil {
		instanceUpdate.SetAttributes(attributes)
	}

	instanceId := fmt.Sprintf("%s:%s", plan.Resource.ValueString(), plan.Key.ValueString())
	updated, err := c.client.Api.ResourceInstances.Update(ctx, instanceId, *instanceUpdate)
	if err != nil {
		return resourceInstanceModel{}, err
	}

	return tfModelFromResourceInstanceRead(*updated), nil
}

func (c *resourceInstanceClient) Delete(ctx context.Context, key string, resource string) error {
	instanceId := fmt.Sprintf("%s:%s", resource, key)
	return c.client.Api.ResourceInstances.Delete(ctx, instanceId)
}
