package user_attributes

import (
	"context"

	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

type userAttributesClient struct {
	client *permit.Client
}

func (c *userAttributesClient) Create(ctx context.Context, plan userAttributeModel) (userAttributeModel, error) {

	attributeType, err := models.NewAttributeTypeFromValue(plan.Type.ValueString())

	if err != nil {
		return userAttributeModel{}, err
	}

	attributeCreate := models.ResourceAttributeCreate{}
	attributeCreate.SetKey(plan.Key.ValueString())
	attributeCreate.SetType(*attributeType)
	attributeCreate.SetDescription(plan.Description.ValueString())

	createdAttribute, err := c.client.Api.ResourceAttributes.Create(ctx, UserKey, attributeCreate)

	if err != nil {
		return userAttributeModel{}, err
	}

	return tfModelFromSDK(*createdAttribute), nil
}

func (c *userAttributesClient) Read(ctx context.Context, key string) (userAttributeModel, error) {
	readAttribute, err := c.client.Api.ResourceAttributes.Get(ctx, UserKey, key)

	if err != nil {
		return userAttributeModel{}, err
	}

	return tfModelFromSDK(*readAttribute), nil
}

func (c *userAttributesClient) Delete(ctx context.Context, key string) error {
	return c.client.Api.ResourceAttributes.Delete(ctx, UserKey, key)
}

func (c *userAttributesClient) Update(ctx context.Context, key string, plan userAttributeModel) (userAttributeModel, error) {
	attributeType, err := models.NewAttributeTypeFromValue(plan.Type.ValueString())

	if err != nil {
		return userAttributeModel{}, err
	}

	attributeUpdate := models.ResourceAttributeUpdate{}
	attributeUpdate.SetType(*attributeType)
	attributeUpdate.SetDescription(plan.Description.ValueString())

	updatedAttribute, err := c.client.Api.ResourceAttributes.Update(ctx, UserKey, key, attributeUpdate)

	if err != nil {
		return userAttributeModel{}, err
	}

	return tfModelFromSDK(*updatedAttribute), nil
}
