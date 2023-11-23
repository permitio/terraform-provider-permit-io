package relations

import (
	"context"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

type relationClient struct {
	client *permit.Client
}

func (c *relationClient) Create(ctx context.Context, plan relationModel) (relationModel, error) {
	relationCreate := models.RelationCreate{
		Key:             plan.Key.ValueString(),
		Name:            plan.Name.ValueString(),
		Description:     plan.Description.ValueStringPointer(),
		SubjectResource: plan.SubjectResource.ValueString(),
	}

	createdRelation, err := c.client.Api.ResourceRelations.Create(ctx, plan.ObjectResource.ValueString(), relationCreate)

	if err != nil {
		return invalidModel, err
	}

	return tfModelFromSDK(*createdRelation), nil
}

func (c *relationClient) Read(ctx context.Context, objectResourceKey, key string) (relationModel, error) {
	readRelation, err := c.client.Api.ResourceRelations.Get(ctx, objectResourceKey, key)

	if err != nil {
		return invalidModel, err
	}

	return tfModelFromSDK(*readRelation), nil
}

func (c *relationClient) Delete(ctx context.Context, objectResourceKey, key string) error {
	return c.client.Api.ResourceRelations.Delete(ctx, objectResourceKey, key)
}
