package role_assignments

import (
	"context"
	"fmt"
	"github.com/permitio/permit-golang/pkg/permit"
)

type roleAssignmentClient struct {
	client *permit.Client
}

func (c *roleAssignmentClient) Create(ctx context.Context, plan *RoleAssignmentModel) error {
	assignment, err := c.client.Api.Users.AssignRole(
		ctx,
		plan.User.ValueString(),
		plan.Role.ValueString(),
		plan.Tenant.ValueString(),
	)
	if err != nil {
		return err
	}
	*plan = tfModelFromRoleAssignmentRead(*assignment)
	return nil
}

func (c *roleAssignmentClient) Read(ctx context.Context, data RoleAssignmentModel) (RoleAssignmentModel, error) {
	assignments, err := c.client.Api.RoleAssignments.List(
		ctx,
		1, 1, // page, perPage
		data.User.ValueString(),
		data.Role.ValueString(),
		data.Tenant.ValueString(),
	)
	if err != nil {
		return RoleAssignmentModel{}, err
	}
	if assignments == nil || len(*assignments) == 0 {
		return RoleAssignmentModel{}, fmt.Errorf("role assignment not found")
	}
	return tfModelFromRoleAssignmentRead((*assignments)[0]), nil
}

func (c *roleAssignmentClient) Delete(ctx context.Context, plan *RoleAssignmentModel) error {
	_, err := c.client.Api.Users.UnassignRole(
		ctx,
		plan.User.ValueString(),
		plan.Role.ValueString(),
		plan.Tenant.ValueString(),
	)
	return err
}
