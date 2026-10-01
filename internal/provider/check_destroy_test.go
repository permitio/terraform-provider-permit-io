package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/user_attributes"
)

// testAccCheckDestroy asks the Permit API for every managed object in the state
// Terraform just destroyed. A 404 means the object is gone; an object that is still
// there, or any other error, fails the test.
func testAccCheckDestroy(s *terraform.State) error {
	ctx := context.Background()
	client, err := testAccPermitClient()
	if err != nil {
		return fmt.Errorf("building the client for the destroy check: %w", err)
	}
	for address, rs := range s.RootModule().Resources {
		if strings.HasPrefix(address, "data.") {
			continue
		}
		exists, err := testAccObjectExists(ctx, client, rs)
		if err != nil {
			return fmt.Errorf("checking that %s was destroyed: %w", address, err)
		}
		if exists {
			return fmt.Errorf("%s still exists in Permit after destroy", address)
		}
	}
	return nil
}

func testAccObjectExists(
	ctx context.Context, client *permit.Client, rs *terraform.ResourceState,
) (bool, error) {
	attrs := rs.Primary.Attributes
	var err error
	switch rs.Type {
	case "permitio_resource":
		_, err = client.Api.Resources.Get(ctx, attrs["key"])
	case "permitio_role":
		if attrs["resource"] != "" {
			_, err = client.Api.ResourceRoles.Get(ctx, attrs["resource"], attrs["key"])
		} else {
			_, err = client.Api.Roles.Get(ctx, attrs["key"])
		}
	case "permitio_user_set", "permitio_resource_set":
		_, err = client.Api.ConditionSets.Get(ctx, attrs["key"])
	case "permitio_proxy_config":
		_, err = client.Api.ProxyConfigs.Get(ctx, attrs["key"])
	case "permitio_relation":
		_, err = client.Api.ResourceRelations.Get(ctx, attrs["object_resource"], attrs["key"])
	case "permitio_user_attribute":
		_, err = client.Api.ResourceAttributes.Get(ctx, user_attributes.UserKey, attrs["key"])
	case "permitio_tenant":
		_, err = client.Api.Tenants.Get(ctx, attrs["key"])
	case "permitio_role_derivation":
		return testAccRoleDerivationExists(ctx, client, attrs)
	case "permitio_resource_instance":
		_, err = client.Api.ResourceInstances.Get(ctx, attrs["resource"]+":"+attrs["key"])
	case "permitio_role_assignment":
		return testAccAssignmentExists(ctx, client, testAccAssignmentKeys{
			user: attrs["user"], role: attrs["role"], tenant: attrs["tenant"],
		})
	case "permitio_resource_instance_role_assignment":
		return testAccAssignmentExists(ctx, client, testAccAssignmentKeys{
			user: attrs["user"], role: attrs["role"], tenant: attrs["tenant"],
			instance: attrs["resource"] + ":" + attrs["resource_instance"],
		})
	case "permitio_group_resource_instance_role_assignment":
		return testAccGroupRoleExists(ctx, testAccGroupRoleKeys{
			group: attrs["group"], role: attrs["role"], resource: attrs["resource"],
			instance: attrs["resource_instance"],
		})
	default:
		return false, fmt.Errorf("no destroy check for resource type %s", rs.Type)
	}
	return testAccExistsFromErr(err)
}

// testAccRoleDerivationExists looks for the derivation among the grants of its
// target role. The derivation is gone when the grant is missing or the role is.
func testAccRoleDerivationExists(
	ctx context.Context, client *permit.Client, attrs map[string]string,
) (bool, error) {
	role, err := client.Api.ResourceRoles.Get(ctx, attrs["resource"], attrs["to_role"])
	if err != nil {
		return testAccExistsFromErr(err)
	}
	if role.GrantedTo == nil {
		return false, nil
	}
	for _, grant := range role.GrantedTo.UsersWithRole {
		if grant.Role == attrs["role"] && grant.OnResource == attrs["on_resource"] &&
			grant.LinkedByRelation == attrs["linked_by"] {
			return true, nil
		}
	}
	return false, nil
}

// testAccAssignmentKeys names a user's role assignment by keys: in the tenant itself
// when instance is empty, or else on instance, which is resource:instance as the API
// lists it.
type testAccAssignmentKeys struct {
	user, role, tenant, instance string
}

func (k testAccAssignmentKeys) String() string {
	where := "tenant " + k.tenant
	if k.instance != "" {
		where += " on " + k.instance
	}
	return fmt.Sprintf("the assignment of role %s to user %s in %s", k.role, k.user, where)
}

// testAccFindAssignment returns the user's role assignment with these keys, and
// whether there is one. It lists the user's assignments without filtering by role or
// tenant, so that it still works once those are destroyed, and reads one page of the
// most the SDK asks for, 100: an acceptance test's user is its own and has two
// assignments at most.
func testAccFindAssignment(
	ctx context.Context, client *permit.Client, keys testAccAssignmentKeys,
) (models.RoleAssignmentRead, bool, error) {
	assignments, err := client.Api.RoleAssignments.List(ctx, 1, 100, keys.user, "", "")
	if err != nil {
		return models.RoleAssignmentRead{}, false,
			fmt.Errorf("listing the role assignments of user %s: %w", keys.user, err)
	}
	// The SDK returns nil for an empty page.
	if assignments == nil {
		return models.RoleAssignmentRead{}, false, nil
	}
	for _, a := range *assignments {
		if a.User == keys.user && a.Role == keys.role && a.Tenant == keys.tenant &&
			a.GetResourceInstance() == keys.instance {
			return a, true, nil
		}
	}
	return models.RoleAssignmentRead{}, false, nil
}

func testAccAssignmentExists(
	ctx context.Context, client *permit.Client, keys testAccAssignmentKeys,
) (bool, error) {
	_, found, err := testAccFindAssignment(ctx, client, keys)
	if err != nil {
		return testAccExistsFromErr(err)
	}
	return found, nil
}

// testAccGroupRoleKeys names a group's role on a resource instance by keys.
type testAccGroupRoleKeys struct {
	group, role, resource, instance string
}

// testAccGroupRoleExists looks for the role on the resource instance among the roles
// of the group, in one page: an acceptance test's group is its own and has one role.
// The role is gone when it is missing or the group is.
func testAccGroupRoleExists(ctx context.Context, keys testAccGroupRoleKeys) (bool, error) {
	answer, err := testAccGroupsRequest(ctx, http.MethodGet,
		"/"+url.PathEscape(keys.group)+"/roles?page=1&per_page=100", nil)
	if err != nil {
		return testAccExistsFromErr(
			fmt.Errorf("listing the roles of group %s: %w", keys.group, err))
	}
	var page struct {
		Data []struct {
			Key      string `json:"key"`
			Resource struct {
				Key string `json:"key"`
			} `json:"resource"`
			ResourceInstance struct {
				Key string `json:"key"`
			} `json:"resource_instance"`
		} `json:"data"`
	}
	if err := json.Unmarshal(answer, &page); err != nil {
		return false, fmt.Errorf("parsing the roles of group %s: %w", keys.group, err)
	}
	for _, role := range page.Data {
		if role.Key == keys.role && role.Resource.Key == keys.resource &&
			role.ResourceInstance.Key == keys.instance {
			return true, nil
		}
	}
	return false, nil
}

// testAccGroupsRequest sends method to path under the groups of the environment
// PERMITIO_API_KEY is scoped to, with body as JSON unless it is nil, and returns the
// body of a 2xx answer. The SDK has no groups API, so the request goes through the
// provider's HTTP client, as the provider's group role requests do.
func testAccGroupsRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	cfg, err := testAccPermitConfig()
	if err != nil {
		return nil, err
	}
	scope, err := resolveAPIKeyScope(ctx, &cfg)
	if err != nil {
		return nil, err
	}
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding the body of %s groups%s: %w", method, path, err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	target := fmt.Sprintf("%s/v2/schema/%s/%s/groups%s",
		strings.TrimSuffix(cfg.GetApiUrl(), "/"), url.PathEscape(scope.GetProject()),
		url.PathEscape(scope.GetEnvironment()), path)
	req, err := http.NewRequestWithContext(ctx, method, target, requestBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.GetToken())
	req.Header.Set("Content-Type", "application/json")
	resp, err := cfg.GetHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading the answer to %s groups%s: %w", method, path, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &common.APIStatusError{StatusCode: resp.StatusCode, Body: string(answer)}
	}
	return answer, nil
}

func testAccExistsFromErr(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if common.IsNotFoundErr(err) {
		return false, nil
	}
	return false, err
}

// fakeScopeJSON is the API key scope the offline fakes answer with, so SDK calls
// resolve project "proj" and environment "env".
const fakeScopeJSON = `{"organization_id":"org","project_id":"proj","environment_id":"env"}`

type fakeReply struct {
	status int
	body   string
}

// newFakePermitAPI starts a fake Permit API and points testAccPermitClient at it. It
// answers the scope call, replies to other requests from replies or with 404, and
// returns a function listing the other requests it got as "METHOD /path".
func newFakePermitAPI(t *testing.T, replies map[string]fakeReply) func() []string {
	t.Helper()
	var (
		mu       sync.Mutex
		requests []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/api-key/scope" {
			_, _ = io.WriteString(w, fakeScopeJSON)
			return
		}
		request := r.Method + " " + r.URL.Path
		mu.Lock()
		requests = append(requests, request)
		mu.Unlock()
		reply, ok := replies[request]
		if !ok {
			reply = fakeReply{status: http.StatusNotFound, body: `{"detail":"not found"}`}
		}
		w.WriteHeader(reply.status)
		_, _ = io.WriteString(w, reply.body)
	}))
	t.Cleanup(server.Close)
	t.Setenv("PERMITIO_API_URL", server.URL)
	t.Setenv("PERMITIO_API_KEY", "fake-key")
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Sorted(slices.Values(requests))
	}
}

func testState(resources map[string]*terraform.ResourceState) *terraform.State {
	return &terraform.State{
		Modules: []*terraform.ModuleState{{Path: []string{"root"}, Resources: resources}},
	}
}

func testResourceState(resourceType string, attrs map[string]string) *terraform.ResourceState {
	return &terraform.ResourceState{
		Type:    resourceType,
		Primary: &terraform.InstanceState{Attributes: attrs},
	}
}

func TestCheckDestroy(t *testing.T) {
	const (
		resourceGet   = "GET /v2/schema/proj/env/resources/tfacc-1-doc"
		globalRoleGet = "GET /v2/schema/proj/env/roles/tfacc-1-admin"
		fileRoleGet   = "GET /v2/schema/proj/env/resources/tfacc-1-file/roles/tfacc-1-reader"
		userSetGet    = "GET /v2/schema/proj/env/condition_sets/tfacc-1-users"
		resSetGet     = "GET /v2/schema/proj/env/condition_sets/tfacc-1-docs"
		proxyGet      = "GET /v2/facts/proj/env/proxy_configs/tfacc-1-proxy"
		relationGet   = "GET /v2/schema/proj/env/resources/tfacc-1-file/relations/tfacc-1-parent"
		attributeGet  = "GET /v2/schema/proj/env/resources/__user/attributes/tfacc_1_attr"
		tenantGet     = "GET /v2/facts/proj/env/tenants/tfacc-1-acme"
		toRoleGet     = "GET /v2/schema/proj/env/resources/tfacc-1-file/roles/tfacc-1-admin"
		instanceGet   = "GET /v2/facts/proj/env/resource_instances/tfacc-1-doc:tfacc-1-readme"
		assignments   = "GET /v2/facts/proj/env/role_assignments"
		groupRoles    = "GET /v2/schema/proj/env/groups/tfacc-1-team/roles"
	)
	state := testState(map[string]*terraform.ResourceState{
		"permitio_resource.doc": testResourceState("permitio_resource",
			map[string]string{"key": "tfacc-1-doc"}),
		"permitio_role.global": testResourceState("permitio_role",
			map[string]string{"key": "tfacc-1-admin"}),
		"permitio_role.reader": testResourceState("permitio_role",
			map[string]string{"key": "tfacc-1-reader", "resource": "tfacc-1-file"}),
		"permitio_user_set.users": testResourceState("permitio_user_set",
			map[string]string{"key": "tfacc-1-users"}),
		"permitio_resource_set.docs": testResourceState("permitio_resource_set",
			map[string]string{"key": "tfacc-1-docs"}),
		"permitio_proxy_config.proxy": testResourceState("permitio_proxy_config",
			map[string]string{"key": "tfacc-1-proxy"}),
		"permitio_relation.parent": testResourceState("permitio_relation", map[string]string{
			"key": "tfacc-1-parent", "subject_resource": "tfacc-1-folder",
			"object_resource": "tfacc-1-file",
		}),
		"permitio_user_attribute.attr": testResourceState("permitio_user_attribute",
			map[string]string{"key": "tfacc_1_attr"}),
		"permitio_tenant.acme": testResourceState("permitio_tenant",
			map[string]string{"key": "tfacc-1-acme"}),
		"permitio_role_derivation.derive": testResourceState("permitio_role_derivation",
			map[string]string{
				"resource": "tfacc-1-file", "to_role": "tfacc-1-admin",
				"role": "tfacc-1-folder-admin", "on_resource": "tfacc-1-folder",
				"linked_by": "tfacc-1-parent",
			}),
		"permitio_resource_instance.readme": testResourceState("permitio_resource_instance",
			map[string]string{"key": "tfacc-1-readme", "resource": "tfacc-1-doc"}),
		"permitio_role_assignment.reader": testResourceState("permitio_role_assignment",
			map[string]string{
				"user": "tfacc-1-alice", "role": "tfacc-1-reader", "tenant": "tfacc-1-acme",
			}),
		"permitio_resource_instance_role_assignment.reader": testResourceState(
			"permitio_resource_instance_role_assignment", map[string]string{
				"user": "tfacc-1-alice", "role": "tfacc-1-reader", "tenant": "tfacc-1-acme",
				"resource": "tfacc-1-doc", "resource_instance": "tfacc-1-readme",
			}),
		"permitio_group_resource_instance_role_assignment.team": testResourceState(
			"permitio_group_resource_instance_role_assignment", map[string]string{
				"group": "tfacc-1-team", "role": "tfacc-1-reader", "tenant": "tfacc-1-acme",
				"resource": "tfacc-1-doc", "resource_instance": "tfacc-1-readme",
			}),
		"data.permitio_user_attribute.lookup": testResourceState("permitio_user_attribute",
			map[string]string{"key": "tfacc_1_lookup"}),
	})
	toRoleGranting := func(role, onResource, linkedBy string) map[string]fakeReply {
		body := fmt.Sprintf(`{"key":"tfacc-1-admin","granted_to":{"users_with_role":[`+
			`{"role":%q,"on_resource":%q,"linked_by_relation":%q}]}}`,
			role, onResource, linkedBy)
		return map[string]fakeReply{toRoleGet: {http.StatusOK, body}}
	}
	aliceAssignments := func(rows ...string) map[string]fakeReply {
		return map[string]fakeReply{
			assignments: {http.StatusOK, "[" + strings.Join(rows, ",") + "]"},
		}
	}
	teamRoles := func(roles ...string) map[string]fakeReply {
		return map[string]fakeReply{
			groupRoles: {http.StatusOK, `{"data":[` + strings.Join(roles, ",") + `]}`},
		}
	}

	tests := []struct {
		name    string
		replies map[string]fakeReply
		wantErr string
	}{
		{name: "every object answers 404"},
		{
			name:    "an object that still exists fails",
			replies: map[string]fakeReply{resourceGet: {http.StatusOK, `{"key":"tfacc-1-doc"}`}},
			wantErr: "permitio_resource.doc still exists",
		},
		{
			name:    "a server error is not a 404",
			replies: map[string]fakeReply{globalRoleGet: {http.StatusInternalServerError, `{}`}},
			wantErr: "checking that permitio_role.global was destroyed",
		},
		{
			name:    "an auth error is not a 404",
			replies: map[string]fakeReply{attributeGet: {http.StatusUnauthorized, `{}`}},
			wantErr: "checking that permitio_user_attribute.attr was destroyed",
		},
		{
			name:    "a tenant that still exists fails",
			replies: map[string]fakeReply{tenantGet: {http.StatusOK, `{"key":"tfacc-1-acme"}`}},
			wantErr: "permitio_tenant.acme still exists",
		},
		{
			name: "a derivation still granted on its target role fails",
			replies: toRoleGranting(
				"tfacc-1-folder-admin", "tfacc-1-folder", "tfacc-1-parent"),
			wantErr: "permitio_role_derivation.derive still exists",
		},
		{
			name:    "a grant of another role passes",
			replies: toRoleGranting("tfacc-1-other", "tfacc-1-folder", "tfacc-1-parent"),
		},
		{
			name: "a grant on another resource passes",
			replies: toRoleGranting(
				"tfacc-1-folder-admin", "tfacc-1-other", "tfacc-1-parent"),
		},
		{
			name: "a grant through another relation passes",
			replies: toRoleGranting(
				"tfacc-1-folder-admin", "tfacc-1-folder", "tfacc-1-other"),
		},
		{
			name: "a target role granted to no one passes",
			replies: map[string]fakeReply{
				toRoleGet: {http.StatusOK, `{"key":"tfacc-1-admin"}`},
			},
		},
		{
			name: "an error reading the target role is not a 404",
			replies: map[string]fakeReply{
				toRoleGet: {http.StatusInternalServerError, `{}`},
			},
			wantErr: "checking that permitio_role_derivation.derive was destroyed",
		},
		{
			name:    "a resource instance that still exists fails",
			replies: map[string]fakeReply{instanceGet: {http.StatusOK, `{"key":"tfacc-1-readme"}`}},
			wantErr: "permitio_resource_instance.readme still exists",
		},
		{
			name: "a tenant-level assignment that still exists fails",
			replies: aliceAssignments(
				assignmentRow("tfacc-1-alice", "tfacc-1-reader", "tfacc-1-acme", "")),
			wantErr: "permitio_role_assignment.reader still exists",
		},
		{
			// The assignment on the instance is not the tenant-level one, so only the
			// resource instance role assignment still exists.
			name: "an assignment on the instance that still exists fails",
			replies: aliceAssignments(assignmentRow(
				"tfacc-1-alice", "tfacc-1-reader", "tfacc-1-acme", "tfacc-1-doc:tfacc-1-readme")),
			wantErr: "permitio_resource_instance_role_assignment.reader still exists",
		},
		{
			name: "assignments of another user, role, tenant or instance pass",
			replies: aliceAssignments(
				assignmentRow("tfacc-1-bob", "tfacc-1-reader", "tfacc-1-acme", ""),
				assignmentRow("tfacc-1-alice", "tfacc-1-writer", "tfacc-1-acme", ""),
				assignmentRow("tfacc-1-alice", "tfacc-1-reader", "tfacc-1-other", ""),
				assignmentRow("tfacc-1-alice", "tfacc-1-reader", "tfacc-1-acme",
					"tfacc-1-doc:tfacc-1-other"),
				assignmentRow("tfacc-1-alice", "tfacc-1-reader", "tfacc-1-acme",
					"tfacc-1-other:tfacc-1-readme"),
			),
		},
		{
			name: "an error listing assignments is not a 404",
			replies: map[string]fakeReply{
				assignments: {http.StatusInternalServerError, `{}`},
			},
			wantErr: "listing the role assignments of user tfacc-1-alice",
		},
		{
			name:    "a group role that still exists fails",
			replies: teamRoles(groupRole("tfacc-1-reader", "tfacc-1-doc", "tfacc-1-readme")),
			wantErr: "permitio_group_resource_instance_role_assignment.team still exists",
		},
		{
			name: "group roles of another role, resource or instance pass",
			replies: teamRoles(
				groupRole("tfacc-1-writer", "tfacc-1-doc", "tfacc-1-readme"),
				groupRole("tfacc-1-reader", "tfacc-1-other", "tfacc-1-readme"),
				groupRole("tfacc-1-reader", "tfacc-1-doc", "tfacc-1-other"),
			),
		},
		{
			name: "an error listing group roles is not a 404",
			replies: map[string]fakeReply{
				groupRoles: {http.StatusInternalServerError, `{}`},
			},
			wantErr: "checking that permitio_group_resource_instance_role_assignment.team " +
				"was destroyed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := newFakePermitAPI(t, tt.replies)

			err := testAccCheckDestroy(state)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("testAccCheckDestroy() = %v, want an error containing %q",
						err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("testAccCheckDestroy() = %v, want nil", err)
			}
			want := slices.Sorted(slices.Values([]string{
				resourceGet, globalRoleGet, fileRoleGet, userSetGet, resSetGet,
				proxyGet, relationGet, attributeGet, tenantGet, toRoleGet, instanceGet,
				assignments, assignments, groupRoles,
			}))
			if got := requests(); !slices.Equal(got, want) {
				t.Errorf("requests = %q, want one lookup per managed object %q", got, want)
			}
		})
	}
}

// assignmentRow is a role assignment as the API lists it: on instance, written
// resource:instance, or in the tenant itself when instance is empty.
func assignmentRow(user, role, tenant, instance string) string {
	resourceInstance := ""
	if instance != "" {
		resourceInstance = fmt.Sprintf(`"resource_instance":%q,`, instance)
	}
	return fmt.Sprintf(`{"id":"tfacc-1-assignment","user":%q,"role":%q,"tenant":%q,%s`+
		`"user_id":"u","role_id":"r","tenant_id":"t","organization_id":"org",`+
		`"project_id":"proj","environment_id":"env","created_at":"2026-01-01T00:00:00Z"}`,
		user, role, tenant, resourceInstance)
}

// groupRole is a role of a group on a resource instance as the API lists it.
func groupRole(role, resource, instance string) string {
	return fmt.Sprintf(`{"key":%q,"resource":{"key":%q},"resource_instance":{"key":%q}}`,
		role, resource, instance)
}

// TestCheckDestroyRejectsUnknownType checks that an object of a type the destroy
// check does not know fails the check rather than passing unchecked.
func TestCheckDestroyRejectsUnknownType(t *testing.T) {
	requests := newFakePermitAPI(t, nil)
	state := testState(map[string]*terraform.ResourceState{
		"permitio_unknown.a": testResourceState("permitio_unknown",
			map[string]string{"key": "tfacc-1-a"}),
	})

	err := testAccCheckDestroy(state)

	const want = "no destroy check for resource type permitio_unknown"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("testAccCheckDestroy() = %v, want an error containing %q", err, want)
	}
	if got := requests(); len(got) != 0 {
		t.Errorf("requests = %q, want none", got)
	}
}

func TestCheckDestroyRejectsMalformedTimeout(t *testing.T) {
	requests := newFakePermitAPI(t, nil)
	t.Setenv("PERMITIO_TIMEOUT", "soon")
	state := testState(map[string]*terraform.ResourceState{
		"permitio_resource.doc": testResourceState("permitio_resource",
			map[string]string{"key": "tfacc-1-doc"}),
	})

	err := testAccCheckDestroy(state)

	const want = "building the client for the destroy check"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("testAccCheckDestroy() = %v, want an error containing %q", err, want)
	}
	if got := requests(); len(got) != 0 {
		t.Errorf("requests = %q, want none", got)
	}
}
