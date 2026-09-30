package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
		"data.permitio_user_attribute.lookup": testResourceState("permitio_user_attribute",
			map[string]string{"key": "tfacc_1_lookup"}),
	})
	toRoleGranting := func(role, onResource, linkedBy string) map[string]fakeReply {
		body := fmt.Sprintf(`{"key":"tfacc-1-admin","granted_to":{"users_with_role":[`+
			`{"role":%q,"on_resource":%q,"linked_by_relation":%q}]}}`,
			role, onResource, linkedBy)
		return map[string]fakeReply{toRoleGet: {http.StatusOK, body}}
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
				proxyGet, relationGet, attributeGet, tenantGet, toRoleGet,
			}))
			if got := requests(); !slices.Equal(got, want) {
				t.Errorf("requests = %q, want one lookup per managed object %q", got, want)
			}
		})
	}
}

func TestCheckDestroyRejectsUnknownType(t *testing.T) {
	requests := newFakePermitAPI(t, nil)
	state := testState(map[string]*terraform.ResourceState{
		"permitio_role_assignment.a": testResourceState("permitio_role_assignment",
			map[string]string{"user": "u"}),
	})

	err := testAccCheckDestroy(state)

	const want = "no destroy check for resource type permitio_role_assignment"
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
