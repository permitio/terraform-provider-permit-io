package resource_instance_role_assignments

import (
	"context"
	"fmt"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

type resourceInstanceRoleAssignmentClient struct {
	client *permit.Client
}

func (c *resourceInstanceRoleAssignmentClient) Create(ctx context.Context, plan *ResourceInstanceRoleAssignmentModel) error {
	subject := fmt.Sprintf("__user:%s", plan.User.ValueString())
	object := fmt.Sprintf("%s:%s", plan.Resource.ValueString(), plan.ResourceInstance.ValueString())

	tupleCreate := models.NewRelationshipTupleCreate(
		subject,
		plan.Role.ValueString(),
		object,
	)
	tupleCreate.SetTenant(plan.Tenant.ValueString())

	tuple, err := c.client.Api.RelationshipTuples.Create(ctx, *tupleCreate)
	if err != nil {
		return err
	}
	*plan = tfModelFromRelationshipTupleRead(*tuple)
	return nil
}

func (c *resourceInstanceRoleAssignmentClient) Read(ctx context.Context, data ResourceInstanceRoleAssignmentModel) (ResourceInstanceRoleAssignmentModel, error) {
	subject := fmt.Sprintf("__user:%s", data.User.ValueString())
	object := fmt.Sprintf("%s:%s", data.Resource.ValueString(), data.ResourceInstance.ValueString())

	tuples, err := c.client.Api.RelationshipTuples.List(
		ctx,
		1, 1, // page, perPage
		data.Tenant.ValueString(),
		subject,
		data.Role.ValueString(),
		object,
	)
	if err != nil {
		return ResourceInstanceRoleAssignmentModel{}, err
	}
	if tuples == nil || len(*tuples) == 0 {
		return ResourceInstanceRoleAssignmentModel{}, fmt.Errorf("resource instance role assignment not found")
	}
	return tfModelFromRelationshipTupleRead((*tuples)[0]), nil
}

func (c *resourceInstanceRoleAssignmentClient) Delete(ctx context.Context, plan *ResourceInstanceRoleAssignmentModel) error {
	subject := fmt.Sprintf("__user:%s", plan.User.ValueString())
	object := fmt.Sprintf("%s:%s", plan.Resource.ValueString(), plan.ResourceInstance.ValueString())

	tupleDelete := models.RelationshipTupleDelete{
		Subject:  subject,
		Relation: plan.Role.ValueString(),
		Object:   object,
	}

	return c.client.Api.RelationshipTuples.Delete(ctx, tupleDelete)
}
