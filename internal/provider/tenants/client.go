package tenants

import (
	"context"
	"encoding/json"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

type tenantClient struct {
	client *permit.Client
}

func (c *tenantClient) Create(ctx context.Context, plan tenantModel) (tenantModel, error) {
	// Parse attributes from JSON string
	var attributes map[string]interface{}
	if !plan.Attributes.IsNull() && plan.Attributes.ValueString() != "" {
		err := json.Unmarshal([]byte(plan.Attributes.ValueString()), &attributes)
		if err != nil {
			return tenantModel{}, err
		}
	}

	tenantCreate := models.TenantCreate{
		Key:         plan.Key.ValueString(),
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueStringPointer(),
		Attributes:  attributes,
	}

	createdTenant, err := c.client.Api.Tenants.Create(ctx, tenantCreate)

	if err != nil {
		return tenantModel{}, err
	}

	return tfModelFromTenantRead(*createdTenant), nil
}

func (c *tenantClient) Read(ctx context.Context, key string) (tenantModel, error) {
	tenantRead, err := c.client.Api.Tenants.Get(ctx, key)

	if err != nil {
		return tenantModel{}, err
	}

	return tfModelFromTenantRead(*tenantRead), nil
}

func (c *tenantClient) Update(ctx context.Context, plan tenantModel) (tenantModel, error) {
	// Parse attributes from JSON string
	var attributes map[string]interface{}
	if !plan.Attributes.IsNull() && plan.Attributes.ValueString() != "" {
		err := json.Unmarshal([]byte(plan.Attributes.ValueString()), &attributes)
		if err != nil {
			return tenantModel{}, err
		}
	}

	tenantUpdate := models.TenantUpdate{
		Name:        plan.Name.ValueStringPointer(),
		Description: plan.Description.ValueStringPointer(),
		Attributes:  attributes,
	}

	updatedTenant, err := c.client.Api.Tenants.Update(ctx, plan.Key.ValueString(), tenantUpdate)

	if err != nil {
		return tenantModel{}, err
	}

	return tfModelFromTenantRead(*updatedTenant), nil
}

func (c *tenantClient) Delete(ctx context.Context, key string) error {
	return c.client.Api.Tenants.Delete(ctx, key)
}
