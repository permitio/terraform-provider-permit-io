package group_resource_instance_role_assignments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/config"
)

type groupResourceInstanceRoleAssignmentClient struct {
	// api is the provider's connection to the Permit API, which every request of
	// this client goes through. It is nil when the provider has not been configured.
	api *config.API
}

// errNotConfigured is the error of a request made before the provider was
// configured, when there is no API URL, API key or HTTP client to send it with.
var errNotConfigured = errors.New("the Permit.io provider is not configured, so there is " +
	"no API URL, API key or HTTP client to send the request with")

// errNoEnvironment is the error of a request made with an API key that is not
// scoped to one environment, which the group roles path must name.
var errNoEnvironment = errors.New("the Permit.io API key is not scoped to an environment, " +
	"so there is no project and environment to send the group role request to; use an " +
	"environment API key")

// rolesURL returns the URL of the roles of group, in the project and environment
// of the provider's API key. It fails when the provider has not been configured or
// its API key is not scoped to an environment.
func (c *groupResourceInstanceRoleAssignmentClient) rolesURL(group string) (string, error) {
	if c.api == nil || c.api.HTTPClient == nil {
		return "", errNotConfigured
	}
	if c.api.ProjectID == "" || c.api.EnvironmentID == "" {
		return "", errNoEnvironment
	}
	return fmt.Sprintf("%s/v2/schema/%s/%s/groups/%s/roles",
		strings.TrimSuffix(c.api.URL, "/"), url.PathEscape(c.api.ProjectID),
		url.PathEscape(c.api.EnvironmentID), url.PathEscape(group)), nil
}

// authorize sets the provider's API key and the JSON content type on req.
func (c *groupResourceInstanceRoleAssignmentClient) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.api.Key)
	req.Header.Set("Content-Type", "application/json")
}

func (c *groupResourceInstanceRoleAssignmentClient) Create(ctx context.Context, plan *GroupResourceInstanceRoleAssignmentModel) error {
	rolesURL, err := c.rolesURL(plan.Group.ValueString())
	if err != nil {
		return err
	}

	// Prepare request body
	body := GroupAddRole{
		Role:             plan.Role.ValueString(),
		Resource:         plan.Resource.ValueString(),
		ResourceInstance: plan.ResourceInstance.ValueString(),
		Tenant:           plan.Tenant.ValueString(),
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rolesURL,
		bytes.NewBuffer(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.authorize(req)

	// Execute request
	resp, err := c.api.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return &common.APIStatusError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	// Generate ID for Terraform state
	plan.Id = plan.Group
	return nil
}

// Read returns data when the group has the role on the instance, with id set to
// the group key, which an import leaves unset.
func (c *groupResourceInstanceRoleAssignmentClient) Read(ctx context.Context, data GroupResourceInstanceRoleAssignmentModel) (GroupResourceInstanceRoleAssignmentModel, error) {
	rolesURL, err := c.rolesURL(data.Group.ValueString())
	if err != nil {
		return GroupResourceInstanceRoleAssignmentModel{}, err
	}

	found, err := c.findRole(ctx, rolesURL, data)
	// The pages are not a snapshot of the group's roles. A role removed during the
	// walk shifts later roles onto pages already read, and a page's total_count can
	// already leave out a role that its data still holds, which ends the walk early.
	// Either hides a role that still exists, so a miss gets one more walk before the
	// assignment counts as gone.
	if err == nil && !found {
		found, err = c.findRole(ctx, rolesURL, data)
	}
	if err != nil {
		return GroupResourceInstanceRoleAssignmentModel{}, err
	}
	if !found {
		return GroupResourceInstanceRoleAssignmentModel{},
			fmt.Errorf("group resource instance role assignment %w", common.ErrNotFound)
	}

	data.Id = data.Group
	return data, nil
}

// maxGroupRolePages bounds how many pages findRole reads, so that an API that
// keeps answering with pages of roles cannot keep a refresh listing forever. It
// allows 100,000 roles of one group.
const maxGroupRolePages = 1000

// findRole walks the group's roles page by page and reports whether the assignment
// is there. The walk ends on an empty page, or once it has seen as many roles as
// the total_count of a page that has one. A page can hold fewer roles than it was
// asked for and still not be the last, since total_count counts roles that the
// API leaves out of the pages, so a short page does not end the walk.
func (c *groupResourceInstanceRoleAssignmentClient) findRole(ctx context.Context, rolesURL string,
	data GroupResourceInstanceRoleAssignmentModel) (bool, error) {
	seen := 0
	for page := 1; page <= maxGroupRolePages; page++ {
		result, err := c.listRolesPage(ctx, rolesURL, page)
		if err != nil {
			return false, fmt.Errorf(
				"list roles of group %q (page %d): %w", data.Group.ValueString(), page, err)
		}

		for _, item := range result.Data {
			if item.Key == data.Role.ValueString() &&
				item.Resource.Key == data.Resource.ValueString() &&
				item.ResourceInstance.Key == data.ResourceInstance.ValueString() {
				return true, nil
			}
		}

		seen += len(result.Data)
		if len(result.Data) == 0 || result.TotalCount != nil && seen >= *result.TotalCount {
			return false, nil
		}
	}
	return false, fmt.Errorf("list roles of group %q: stopped after %d pages of up to %d "+
		"roles without reaching the last page",
		data.Group.ValueString(), maxGroupRolePages, groupRolesPerPage)
}

// groupRolesPerPage is the largest page size the group roles endpoint accepts.
const groupRolesPerPage = 100

type groupRolesPage struct {
	Data []struct {
		Key              string `json:"key"`
		ResourceInstance struct {
			Key string `json:"key"`
		} `json:"resource_instance"`
		Resource struct {
			Key string `json:"key"`
		} `json:"resource"`
	} `json:"data"`
	// TotalCount is nil when the response leaves total_count out.
	TotalCount *int `json:"total_count"`
}

// listRolesPage fetches one page of up to groupRolesPerPage roles from rolesURL.
func (c *groupResourceInstanceRoleAssignmentClient) listRolesPage(
	ctx context.Context, rolesURL string, page int,
) (groupRolesPage, error) {
	query := url.Values{
		"page":     {strconv.Itoa(page)},
		"per_page": {strconv.Itoa(groupRolesPerPage)},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rolesURL+"?"+query.Encode(), nil)
	if err != nil {
		return groupRolesPage{}, fmt.Errorf("failed to create request: %w", err)
	}
	c.authorize(req)

	resp, err := c.api.HTTPClient.Do(req)
	if err != nil {
		return groupRolesPage{}, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return groupRolesPage{}, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return groupRolesPage{},
			&common.APIStatusError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	var result groupRolesPage
	if err := json.Unmarshal(respBody, &result); err != nil {
		return groupRolesPage{}, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (c *groupResourceInstanceRoleAssignmentClient) Delete(ctx context.Context, plan *GroupResourceInstanceRoleAssignmentModel) error {
	rolesURL, err := c.rolesURL(plan.Group.ValueString())
	if err != nil {
		return err
	}

	// Prepare request body
	body := GroupAddRole{
		Role:             plan.Role.ValueString(),
		Resource:         plan.Resource.ValueString(),
		ResourceInstance: plan.ResourceInstance.ValueString(),
		Tenant:           plan.Tenant.ValueString(),
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, rolesURL,
		bytes.NewBuffer(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.authorize(req)

	// Execute request
	resp, err := c.api.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for error details
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return &common.APIStatusError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	return nil
}
