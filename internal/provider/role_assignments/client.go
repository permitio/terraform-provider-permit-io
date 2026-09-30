package role_assignments

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

type roleAssignmentClient struct {
	client *permit.Client
}

// Create assigns the role in the tenant. The request names no resource instance,
// so the API makes, and returns, the tenant-level assignment.
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

// Read finds the tenant-level assignment among the user's assignments of the role
// in the tenant. That list also holds the user's assignments of a resource role
// with the same key on resource instances in the tenant, and it has no filter that
// leaves them out, so Read walks it a page at a time and skips them. The pages are
// not a snapshot of the list: an assignment removed during the walk shifts later
// ones onto pages already read, which hides one that still exists. So a miss gets
// one more walk before the assignment counts as gone.
func (c *roleAssignmentClient) Read(ctx context.Context, data RoleAssignmentModel) (RoleAssignmentModel, error) {
	found, ok, err := c.findAssignment(ctx, data)
	if err == nil && !ok {
		found, ok, err = c.findAssignment(ctx, data)
	}
	if err != nil {
		return RoleAssignmentModel{}, err
	}
	if !ok {
		return RoleAssignmentModel{}, fmt.Errorf("role assignment %w", common.ErrNotFound)
	}
	return found, nil
}

// findAssignment walks the user's assignments of the role in the tenant page by
// page until it finds the one on no resource instance or reads an empty page. The
// API does not promise that a page with fewer assignments than asked for is the
// last, so a short page does not end the walk.
func (c *roleAssignmentClient) findAssignment(ctx context.Context,
	data RoleAssignmentModel) (RoleAssignmentModel, bool, error) {
	for page := 1; page <= maxAssignmentPages; page++ {
		assignments, err := c.client.Api.RoleAssignments.List(
			ctx,
			page, assignmentsPerPage,
			data.User.ValueString(),
			data.Role.ValueString(),
			data.Tenant.ValueString(),
		)
		if err != nil {
			return RoleAssignmentModel{}, false,
				fmt.Errorf("list role assignments (page %d): %w", page, err)
		}
		// The SDK returns nil for an empty page.
		if assignments == nil || len(*assignments) == 0 {
			return RoleAssignmentModel{}, false, nil
		}
		for _, a := range *assignments {
			if a.GetResourceInstance() == "" {
				return tfModelFromRoleAssignmentRead(a), true, nil
			}
		}
	}

	return RoleAssignmentModel{}, false, fmt.Errorf(
		"list role assignments: stopped after %d pages of up to %d assignments of role %q "+
			"to user %q in tenant %q without reaching the last page",
		maxAssignmentPages, assignmentsPerPage, data.Role.ValueString(),
		data.User.ValueString(), data.Tenant.ValueString())
}

// Delete unassigns the role in the tenant. The request names no resource instance,
// so it removes the tenant-level assignment only.
func (c *roleAssignmentClient) Delete(ctx context.Context, plan *RoleAssignmentModel) error {
	_, err := c.client.Api.Users.UnassignRole(
		ctx,
		plan.User.ValueString(),
		plan.Role.ValueString(),
		plan.Tenant.ValueString(),
	)
	return err
}
