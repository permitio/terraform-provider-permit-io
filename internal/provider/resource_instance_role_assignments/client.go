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
	// Format: subject is __user:user_key, object is resource_key:instance_key
	subject := fmt.Sprintf("__user:%s", plan.User.ValueString())
	tenant := plan.Tenant.ValueString()

	tupleCreate := models.NewRelationshipTupleCreate(
		subject,
		plan.Role.ValueString(),
		plan.ResourceInstance.ValueString(),
	)
	tupleCreate.SetTenant(tenant)

	tuple, err := c.client.Api.RelationshipTuples.Create(ctx, *tupleCreate)
	if err != nil {
		return err
	}
	*plan = tfModelFromRelationshipTupleRead(*tuple)
	return nil
}

func (c *resourceInstanceRoleAssignmentClient) Read(ctx context.Context, data ResourceInstanceRoleAssignmentModel) (ResourceInstanceRoleAssignmentModel, error) {
	subject := fmt.Sprintf("__user:%s", data.User.ValueString())

	tuples, err := c.client.Api.RelationshipTuples.List(
		ctx,
		1, 1, // page, perPage
		data.Tenant.ValueString(),
		subject,
		data.Role.ValueString(),
		data.ResourceInstance.ValueString(),
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

	tupleDelete := models.RelationshipTupleDelete{
		Subject:  subject,
		Relation: plan.Role.ValueString(),
		Object:   plan.ResourceInstance.ValueString(),
	}

	return c.client.Api.RelationshipTuples.Delete(ctx, tupleDelete)
}
