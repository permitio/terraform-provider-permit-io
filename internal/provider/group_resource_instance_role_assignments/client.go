package group_resource_instance_role_assignments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/permitio/permit-golang/pkg/permit"
)

type groupResourceInstanceRoleAssignmentClient struct {
	client *permit.Client
	// Cache context info after first retrieval
	cachedProjectId string
	cachedEnvId     string
	cachedApiUrl    string
	cachedToken     string
}

// getContextInfo extracts project and environment IDs from a tenant API call.
func (c *groupResourceInstanceRoleAssignmentClient) getContextInfo(ctx context.Context) (projectId, envId, apiUrl, token string, err error) {
	// Return cached values if available
	if c.cachedProjectId != "" && c.cachedEnvId != "" {
		return c.cachedProjectId, c.cachedEnvId, c.cachedApiUrl, c.cachedToken, nil
	}

	// Call Tenants.List() to get at least one tenant which contains project_id and environment_id
	tenants, err := c.client.Api.Tenants.List(ctx, 1, 1)
	if err != nil {
		err = fmt.Errorf("failed to get context info from tenants API: %w", err)
		return
	}

	if len(tenants) == 0 {
		err = fmt.Errorf("no tenants found - cannot determine project_id and environment_id. Make sure at least one tenant exists in your Permit environment")
		return
	}

	// Extract project_id and environment_id from the first tenant
	firstTenant := tenants[0]
	projectId = firstTenant.ProjectId
	envId = firstTenant.EnvironmentId

	// Use cached token and API URL if available (set during Configure)
	if c.cachedToken != "" {
		token = c.cachedToken
		apiUrl = c.cachedApiUrl
		if apiUrl == "" {
			apiUrl = "https://api.permit.io"
		}
	} else {
		// Fallback: try environment variables
		token = getTokenFromEnv()
		apiUrl = "https://api.permit.io"
		if token == "" {
			err = fmt.Errorf("API token not found - ensure provider is configured with api_key")
			return
		}
	}

	// Cache the values
	c.cachedProjectId = projectId
	c.cachedEnvId = envId
	c.cachedApiUrl = apiUrl
	c.cachedToken = token

	return
}

// getTokenFromEnv gets the API token from environment variables.
func getTokenFromEnv() string {
	// Check the same environment variable the provider uses
	if token := getEnv("PERMITIO_API_KEY"); token != "" {
		return token
	}
	if token := getEnv("PERMIT_API_KEY"); token != "" {
		return token
	}
	return ""
}

func getEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func (c *groupResourceInstanceRoleAssignmentClient) Create(ctx context.Context, plan *GroupResourceInstanceRoleAssignmentModel) error {
	projectId, envId, apiUrl, token, err := c.getContextInfo(ctx)
	if err != nil {
		return err
	}

	httpClient := http.DefaultClient

	apiUrl = strings.TrimSuffix(apiUrl, "/")
	rolesURL := fmt.Sprintf("%s/v2/schema/%s/%s/groups/%s/roles",
		apiUrl, projectId, envId, plan.Group.ValueString())

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
	req, err := http.NewRequestWithContext(ctx, "POST", rolesURL, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := httpClient.Do(req)
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
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Generate ID for Terraform state
	plan.Id = plan.Group
	return nil
}

func (c *groupResourceInstanceRoleAssignmentClient) Read(ctx context.Context, data GroupResourceInstanceRoleAssignmentModel) (GroupResourceInstanceRoleAssignmentModel, error) {
	projectId, envId, apiUrl, token, err := c.getContextInfo(ctx)
	if err != nil {
		return GroupResourceInstanceRoleAssignmentModel{}, err
	}

	apiUrl = strings.TrimSuffix(apiUrl, "/")
	rolesURL := fmt.Sprintf("%s/v2/schema/%s/%s/groups/%s/roles",
		apiUrl, projectId, envId, data.Group.ValueString())

	found, err := findRole(ctx, rolesURL, token, data)
	// Paging runs outside any snapshot: a delete that commits mid-walk shifts later roles
	// onto pages already read, and one that commits between the backend's data and count
	// queries for a page ends the walk early. Either hides a role that still exists, so a
	// miss gets one more walk before the assignment counts as gone.
	if err == nil && !found {
		found, err = findRole(ctx, rolesURL, token, data)
	}
	if err != nil {
		return GroupResourceInstanceRoleAssignmentModel{}, err
	}
	if !found {
		return GroupResourceInstanceRoleAssignmentModel{},
			fmt.Errorf("group resource instance role assignment not found")
	}

	return data, nil
}

// findRole walks the group's roles page by page and reports whether the assignment is there.
func findRole(ctx context.Context, rolesURL, token string,
	data GroupResourceInstanceRoleAssignmentModel) (bool, error) {
	// page_count is optional in the API response, so stop on total_count instead.
	seen := 0
	for page := 1; ; page++ {
		result, err := listRolesPage(ctx, rolesURL, token, page)
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
		if len(result.Data) == 0 || seen >= *result.TotalCount {
			return false, nil
		}
	}
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
	TotalCount *int `json:"total_count"`
}

// listRolesPage fetches one page of up to groupRolesPerPage roles from rolesURL.
func listRolesPage(ctx context.Context, rolesURL, token string, page int) (groupRolesPage, error) {
	httpClient := http.DefaultClient

	query := url.Values{
		"page":     {strconv.Itoa(page)},
		"per_page": {strconv.Itoa(groupRolesPerPage)},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rolesURL+"?"+query.Encode(), nil)
	if err != nil {
		return groupRolesPage{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
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
			fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result groupRolesPage
	if err := json.Unmarshal(respBody, &result); err != nil {
		return groupRolesPage{}, fmt.Errorf("failed to parse response: %w", err)
	}
	// Defaulting total_count to 0 would end the walk after page 1, so an assignment on a
	// later page would read as a miss and be dropped from state.
	if result.TotalCount == nil {
		return groupRolesPage{}, fmt.Errorf("failed to parse response: missing total_count")
	}

	return result, nil
}

func (c *groupResourceInstanceRoleAssignmentClient) Delete(ctx context.Context, plan *GroupResourceInstanceRoleAssignmentModel) error {
	projectId, envId, apiUrl, token, err := c.getContextInfo(ctx)
	if err != nil {
		return err
	}

	httpClient := http.DefaultClient

	apiUrl = strings.TrimSuffix(apiUrl, "/")
	rolesURL := fmt.Sprintf("%s/v2/schema/%s/%s/groups/%s/roles",
		apiUrl, projectId, envId, plan.Group.ValueString())

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
	req, err := http.NewRequestWithContext(ctx, "DELETE", rolesURL, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := httpClient.Do(req)
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
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
