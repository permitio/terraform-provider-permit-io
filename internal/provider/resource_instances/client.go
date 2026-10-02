package resource_instances

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

type resourceInstanceClient struct {
	client *permit.Client
}

func (c *resourceInstanceClient) Create(ctx context.Context, plan resourceInstanceModel) (resourceInstanceModel, error) {
	instanceCreate := models.NewResourceInstanceCreate(
		plan.Key.ValueString(),
		plan.Resource.ValueString(),
	)
	instanceCreate.SetTenant(plan.Tenant.ValueString())

	if !plan.Attributes.IsNull() {
		attributes, err := common.DecodeJSONObject(plan.Attributes.ValueString())
		if err != nil {
			return resourceInstanceModel{}, fmt.Errorf("attributes: %w", err)
		}
		instanceCreate.SetAttributes(attributes)
	}

	created, err := c.client.Api.ResourceInstances.Create(ctx, *instanceCreate)
	if err != nil {
		return resourceInstanceModel{}, err
	}
	if created == nil {
		return resourceInstanceModel{}, fmt.Errorf("create returned nil response")
	}

	return tfModelFromResourceInstanceRead(*created, plan.Attributes)
}

// Read returns the instance with this key of this resource. priorAttributes is
// the attributes of the prior state; see tfModelFromResourceInstanceRead.
func (c *resourceInstanceClient) Read(ctx context.Context, key string, resource string,
	priorAttributes jsontypes.Normalized,
) (resourceInstanceModel, error) {
	instanceId := fmt.Sprintf("%s:%s", resource, key)
	instance, err := c.client.Api.ResourceInstances.Get(ctx, instanceId)
	if err != nil {
		return resourceInstanceModel{}, err
	}
	if instance == nil {
		return resourceInstanceModel{}, fmt.Errorf("instance %s %w", instanceId, common.ErrNotFound)
	}

	return tfModelFromResourceInstanceRead(*instance, priorAttributes)
}

// Update sends the attributes of plan. The API replaces an instance's attributes
// whole and keeps them when the request leaves them out, so attributes left out
// of the configuration are sent as {} to clear them.
func (c *resourceInstanceClient) Update(ctx context.Context, plan resourceInstanceModel) (resourceInstanceModel, error) {
	attributes := map[string]any{}
	if !plan.Attributes.IsNull() {
		var err error
		attributes, err = common.DecodeJSONObject(plan.Attributes.ValueString())
		if err != nil {
			return resourceInstanceModel{}, fmt.Errorf("attributes: %w", err)
		}
	}

	instanceUpdate := models.NewResourceInstanceUpdate()
	instanceUpdate.SetAttributes(attributes)

	instanceId := fmt.Sprintf("%s:%s", plan.Resource.ValueString(), plan.Key.ValueString())
	updated, err := c.client.Api.ResourceInstances.Update(ctx, instanceId, *instanceUpdate)
	if err != nil {
		return resourceInstanceModel{}, err
	}
	if updated == nil {
		return resourceInstanceModel{}, fmt.Errorf("update returned nil response for %s", instanceId)
	}

	return tfModelFromResourceInstanceRead(*updated, plan.Attributes)
}

func (c *resourceInstanceClient) Delete(ctx context.Context, key string, resource string) error {
	instanceId := fmt.Sprintf("%s:%s", resource, key)
	return c.client.Api.ResourceInstances.Delete(ctx, instanceId)
}
