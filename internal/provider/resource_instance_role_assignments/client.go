package resource_instance_role_assignments

import (
	"context"
	"fmt"

	"github.com/permitio/permit-golang/pkg/permit"
)

type resourceInstanceRoleAssignmentClient struct {
	client *permit.Client
}

func (c *resourceInstanceRoleAssignmentClient) Create(ctx context.Context, plan *ResourceInstanceRoleAssignmentModel) error {
	resourceInstance := fmt.Sprintf("%s:%s", plan.Resource.ValueString(), plan.ResourceInstance.ValueString())

	assignment, err := c.client.Api.Users.AssignResourceRole(
		ctx,
		plan.User.ValueString(),
		plan.Role.ValueString(),
		plan.Tenant.ValueString(),
		resourceInstance,
	)
	if err != nil {
		return err
	}
	*plan = tfModelFromRoleAssignmentRead(*assignment)
	return nil
}

func (c *resourceInstanceRoleAssignmentClient) Read(ctx context.Context, data ResourceInstanceRoleAssignmentModel) (ResourceInstanceRoleAssignmentModel, error) {
	resourceInstance := fmt.Sprintf("%s:%s", data.Resource.ValueString(), data.ResourceInstance.ValueString())

	assignments, err := c.client.Api.RoleAssignments.List(
		ctx,
		1, 100, // page, perPage
		data.User.ValueString(),
		data.Role.ValueString(),
		data.Tenant.ValueString(),
	)
	if err != nil {
		return ResourceInstanceRoleAssignmentModel{}, err
	}
	if assignments == nil {
		return ResourceInstanceRoleAssignmentModel{}, fmt.Errorf("resource instance role assignment not found")
	}

	for _, a := range *assignments {
		if a.ResourceInstance != nil && *a.ResourceInstance == resourceInstance {
			return tfModelFromRoleAssignmentRead(a), nil
		}
	}

	return ResourceInstanceRoleAssignmentModel{}, fmt.Errorf("resource instance role assignment not found")
}

func (c *resourceInstanceRoleAssignmentClient) Delete(ctx context.Context, plan *ResourceInstanceRoleAssignmentModel) error {
	resourceInstance := fmt.Sprintf("%s:%s", plan.Resource.ValueString(), plan.ResourceInstance.ValueString())

	_, err := c.client.Api.Users.UnassignResourceRole(
		ctx,
		plan.User.ValueString(),
		plan.Role.ValueString(),
		plan.Tenant.ValueString(),
		resourceInstance,
	)
	return err
}
