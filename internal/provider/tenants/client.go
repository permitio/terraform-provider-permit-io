package tenants

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

type tenantClient struct {
	client *permit.Client
}

func (c *tenantClient) Create(ctx context.Context, plan tenantModel) (tenantModel, error) {
	var attributes map[string]any
	if !plan.Attributes.IsNull() {
		var err error
		attributes, err = common.DecodeJSONObject(plan.Attributes.ValueString())
		if err != nil {
			return tenantModel{}, fmt.Errorf("attributes: %w", err)
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

	return tfModelFromTenantRead(*createdTenant, plan.Attributes)
}

// Read returns the tenant with this key. priorAttributes is the attributes of the
// prior state; see tfModelFromTenantRead.
func (c *tenantClient) Read(ctx context.Context, key string,
	priorAttributes jsontypes.Normalized,
) (tenantModel, error) {
	tenantRead, err := c.client.Api.Tenants.Get(ctx, key)

	if err != nil {
		return tenantModel{}, err
	}

	return tfModelFromTenantRead(*tenantRead, priorAttributes)
}

// Update sends the plan's name, description and attributes. The API replaces a
// tenant's attributes whole and keeps them when the request leaves them out, so
// attributes left out of the configuration are sent as {} to clear them.
func (c *tenantClient) Update(ctx context.Context, plan tenantModel) (tenantModel, error) {
	attributes := map[string]any{}
	if !plan.Attributes.IsNull() {
		var err error
		attributes, err = common.DecodeJSONObject(plan.Attributes.ValueString())
		if err != nil {
			return tenantModel{}, fmt.Errorf("attributes: %w", err)
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

	return tfModelFromTenantRead(*updatedTenant, plan.Attributes)
}

func (c *tenantClient) Delete(ctx context.Context, key string) error {
	return c.client.Api.Tenants.Delete(ctx, key)
}
