package group_resource_instance_role_assignments

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type groupRole struct {
	key, resource, instance string
}

// groupRoles returns roles r0..r(n-1), all on workspace instance ws-1.
func groupRoles(n int) []groupRole {
	roles := make([]groupRole, n)
	for i := range roles {
		roles[i] = groupRole{fmt.Sprintf("r%d", i), "workspace", "ws-1"}
	}
	return roles
}

func TestRead(t *testing.T) {
	tests := []struct {
		name      string
		roles     []groupRole
		role      string
		errOn     int    // request number that fails with a 500
		badBodyOn int    // request number that returns a malformed body
		deleteR0  bool   // another client deletes r0 after the first request
		wantErr   string // "" means found
		wantPages []int
	}{
		// 250 roles is three pages at groupRolesPerPage.
		{name: "first page", roles: groupRoles(250), role: "r5", wantPages: []int{1}},
		{name: "later page", roles: groupRoles(250), role: "r210", wantPages: []int{1, 2, 3}},
		{
			name:      "missing",
			roles:     groupRoles(250),
			role:      "missing",
			wantErr:   "not found",
			wantPages: []int{1, 2, 3, 1, 2, 3},
		},
		{name: "no roles", role: "r0", wantErr: "not found", wantPages: []int{1}},
		// A full last page must not trigger a request for an empty one.
		{
			name:      "exactly one page",
			roles:     groupRoles(groupRolesPerPage),
			role:      "missing",
			wantErr:   "not found",
			wantPages: []int{1},
		},
		// r100 shifts from page 2 onto page 1, which the first walk already read.
		{
			name:      "deleted mid-walk",
			roles:     groupRoles(250),
			role:      "r100",
			deleteR0:  true,
			wantPages: []int{1, 2, 3, 1},
		},
		// The same role on another instance or resource is not a match.
		{
			name:      "near misses",
			roles:     []groupRole{{"r5", "workspace", "ws-2"}, {"r5", "doc", "ws-1"}},
			role:      "r5",
			wantErr:   "not found",
			wantPages: []int{1},
		},
		// A failed page must fail the refresh, not report the assignment as gone.
		{
			name:      "server error on later page",
			roles:     groupRoles(250),
			role:      "r210",
			errOn:     2,
			wantErr:   `list roles of group "developers" (page 2): API request failed with status 500`,
			wantPages: []int{1, 2},
		},
		{
			name:      "bad body on later page",
			roles:     groupRoles(250),
			role:      "r210",
			badBodyOn: 2,
			wantErr:   "failed to parse response",
			wantPages: []int{1, 2},
		},
		{
			name:      "server error on second walk",
			roles:     groupRoles(250),
			role:      "missing",
			errOn:     4,
			wantErr:   `list roles of group "developers" (page 1): API request failed with status 500`,
			wantPages: []int{1, 2, 3, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pages []int
			roles := tt.roles
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
				if r.Method != http.MethodGet ||
					r.URL.Path != "/v2/schema/proj/env/groups/developers/roles" ||
					r.Header.Get("Authorization") != "Bearer token" ||
					page < 1 || perPage < 1 || perPage > 100 {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				pages = append(pages, page)

				switch len(pages) {
				case tt.errOn:
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				case tt.badBodyOn:
					if _, err := io.WriteString(w, "{"); err != nil {
						t.Errorf("failed to write response: %v", err)
					}
					return
				}

				data := []map[string]any{}
				start := min((page-1)*perPage, len(roles))
				end := min(page*perPage, len(roles))
				for _, role := range roles[start:end] {
					data = append(data, map[string]any{
						"key":               role.key,
						"resource":          map[string]string{"key": role.resource},
						"resource_instance": map[string]string{"key": role.instance},
					})
				}
				// page_count is optional in the API spec, so leave it out.
				if err := json.NewEncoder(w).Encode(map[string]any{
					"data":        data,
					"total_count": len(roles),
				}); err != nil {
					t.Errorf("failed to encode response: %v", err)
				}
				if tt.deleteR0 && len(pages) == 1 {
					roles = roles[1:]
				}
			}))
			defer server.Close()

			client := &groupResourceInstanceRoleAssignmentClient{
				cachedProjectId: "proj",
				cachedEnvId:     "env",
				cachedApiUrl:    server.URL,
				cachedToken:     "token",
			}
			_, err := client.Read(t.Context(), GroupResourceInstanceRoleAssignmentModel{
				Group:            types.StringValue("developers"),
				Role:             types.StringValue(tt.role),
				Resource:         types.StringValue("workspace"),
				ResourceInstance: types.StringValue("ws-1"),
				Tenant:           types.StringValue("default"),
			})

			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("Read(%q) error = %v, want nil", tt.role, err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("Read(%q) error = %v, want error containing %q", tt.role, err, tt.wantErr)
			}
			if !slices.Equal(pages, tt.wantPages) {
				t.Errorf("Read(%q) requested pages %v, want %v", tt.role, pages, tt.wantPages)
			}
		})
	}
}
