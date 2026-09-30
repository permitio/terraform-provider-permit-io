package resource_instance_role_assignments

import (
	"context"
	"fmt"

	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

// assignmentsPerPage is the largest page size the SDK's RoleAssignments.List
// accepts.
const assignmentsPerPage = 100

// maxAssignmentPages bounds how many pages a walk of findAssignment lists, so that
// an API that keeps answering with assignments cannot keep a refresh listing
// forever. It allows 100,000 assignments of one role to one user in one tenant.
const maxAssignmentPages = 1000

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

// Read finds the assignment among the user's assignments of the role in the tenant.
// The SDK cannot filter that list by resource instance, so Read walks it a page at
// a time. The pages are not a snapshot of the list: an assignment removed during
// the walk shifts later ones onto pages already read, which hides one that still
// exists. So a miss gets one more walk before the assignment counts as gone.
func (c *resourceInstanceRoleAssignmentClient) Read(ctx context.Context, data ResourceInstanceRoleAssignmentModel) (ResourceInstanceRoleAssignmentModel, error) {
	found, ok, err := c.findAssignment(ctx, data)
	if err == nil && !ok {
		found, ok, err = c.findAssignment(ctx, data)
	}
	if err != nil {
		return ResourceInstanceRoleAssignmentModel{}, err
	}
	if !ok {
		return ResourceInstanceRoleAssignmentModel{},
			fmt.Errorf("resource instance role assignment %w", common.ErrNotFound)
	}
	return found, nil
}

// findAssignment walks the user's assignments of the role in the tenant page by
// page until it finds the one on the resource instance or reads an empty page. The
// API does not promise that a page with fewer assignments than asked for is the
// last, so a short page does not end the walk.
func (c *resourceInstanceRoleAssignmentClient) findAssignment(ctx context.Context,
	data ResourceInstanceRoleAssignmentModel) (ResourceInstanceRoleAssignmentModel, bool, error) {
	resourceInstance := fmt.Sprintf("%s:%s", data.Resource.ValueString(), data.ResourceInstance.ValueString())

	for page := 1; page <= maxAssignmentPages; page++ {
		assignments, err := c.client.Api.RoleAssignments.List(
			ctx,
			page, assignmentsPerPage,
			data.User.ValueString(),
			data.Role.ValueString(),
			data.Tenant.ValueString(),
		)
		if err != nil {
			return ResourceInstanceRoleAssignmentModel{}, false,
				fmt.Errorf("list role assignments (page %d): %w", page, err)
		}
		// The SDK returns nil for an empty page.
		if assignments == nil || len(*assignments) == 0 {
			return ResourceInstanceRoleAssignmentModel{}, false, nil
		}
		for _, a := range *assignments {
			if a.ResourceInstance != nil && *a.ResourceInstance == resourceInstance {
				return tfModelFromRoleAssignmentRead(a), true, nil
			}
		}
	}

	return ResourceInstanceRoleAssignmentModel{}, false, fmt.Errorf(
		"list role assignments: stopped after %d pages of up to %d assignments of role %q "+
			"to user %q in tenant %q without reaching the last page",
		maxAssignmentPages, assignmentsPerPage, data.Role.ValueString(),
		data.User.ValueString(), data.Tenant.ValueString())
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
