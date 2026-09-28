package group_resource_instance_role_assignments

import (
	"context"
	"encoding/json"
	"fmt"
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
		errOn     int    // request number that fails with errStatus
		errStatus int    // status of the errOn response; 0 means 500
		badBodyOn int    // request number whose data field has the wrong type
		totalBias int    // added to the total_count the stub reports
		omitTotal bool   // the stub leaves total_count out of the response
		deleteR0  bool   // another client deletes r0 after the first request
		skewR0    bool   // r0 is deleted between the first response's data and count
		cancelCtx bool   // Read runs with an already cancelled context
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
		{name: "no roles", role: "r0", wantErr: "not found", wantPages: []int{1, 1}},
		// total_count can overstate the rows actually returned, so an empty page ends the walk.
		{
			name:      "total_count overstates rows",
			role:      "r0",
			totalBias: 5,
			wantErr:   "not found",
			wantPages: []int{1, 1},
		},
		// A full last page must not trigger a request for an empty one.
		{
			name:      "exactly one page",
			roles:     groupRoles(groupRolesPerPage),
			role:      "missing",
			wantErr:   "not found",
			wantPages: []int{1, 1},
		},
		// r100 shifts from page 2 onto page 1, which the first walk already read.
		{
			name:      "deleted mid-walk",
			roles:     groupRoles(250),
			role:      "r100",
			deleteR0:  true,
			wantPages: []int{1, 2, 3, 1},
		},
		// A miss that spans only two pages needs the second walk too.
		{
			name:      "deleted mid-walk, two pages",
			roles:     groupRoles(150),
			role:      "r100",
			deleteR0:  true,
			wantPages: []int{1, 2, 1},
		},
		// data still holds r0..r99 but total_count already reflects r0's deletion, so the
		// first walk ends on page 1 without reaching r100.
		{
			name:      "deleted between data and count",
			roles:     groupRoles(101),
			role:      "r100",
			skewR0:    true,
			wantPages: []int{1, 1},
		},
		// The same role on another instance or resource is not a match.
		{
			name:      "near misses",
			roles:     []groupRole{{"r5", "workspace", "ws-2"}, {"r5", "doc", "ws-1"}},
			role:      "r5",
			wantErr:   "not found",
			wantPages: []int{1, 1},
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
		// Without total_count the walk can't tell where the roles end, so it must not guess.
		{
			name:      "missing total_count",
			roles:     groupRoles(250),
			role:      "r150",
			omitTotal: true,
			wantErr:   "missing total_count",
			wantPages: []int{1},
		},
		{
			name:      "server error on second walk",
			roles:     groupRoles(250),
			role:      "missing",
			errOn:     4,
			wantErr:   `list roles of group "developers" (page 1): API request failed with status 500`,
			wantPages: []int{1, 2, 3, 1},
		},
		// A 4xx fails the refresh like a 5xx; only a real miss may drop the assignment.
		{
			name:      "forbidden",
			roles:     groupRoles(250),
			role:      "r5",
			errOn:     1,
			errStatus: http.StatusForbidden,
			wantErr:   `list roles of group "developers" (page 1): API request failed with status 403`,
			wantPages: []int{1},
		},
		// A cancelled refresh must fail before any request, not read as a deletion.
		{
			name:      "cancelled context",
			roles:     groupRoles(250),
			role:      "r5",
			cancelCtx: true,
			wantErr:   "context canceled",
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
				// A walk that never stops fails here instead of hanging until the test timeout.
				if len(pages) > 20 {
					t.Errorf("too many requests: %v", pages)
					http.Error(w, "too many requests", http.StatusInternalServerError)
					return
				}

				switch len(pages) {
				case tt.errOn:
					status := tt.errStatus
					if status == 0 {
						status = http.StatusInternalServerError
					}
					// An empty-page body, so code that ignored the status would read a miss.
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					err := json.NewEncoder(w).Encode(map[string]any{
						"detail":      http.StatusText(status),
						"data":        []any{},
						"total_count": 0,
					})
					if err != nil {
						t.Errorf("failed to write error response: %v", err)
					}
					return
				case tt.badBodyOn:
					// A type mismatch still fills total_count, so a swallowed parse error
					// would read as an empty page.
					_, err := fmt.Fprintf(w, `{"data": {}, "total_count": %d}`, len(roles))
					if err != nil {
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
				if tt.skewR0 && len(pages) == 1 {
					roles = roles[1:]
				}
				// page_count is optional in the API spec, so leave it out.
				body := map[string]any{"data": data}
				if !tt.omitTotal {
					body["total_count"] = len(roles) + tt.totalBias
				}
				if err := json.NewEncoder(w).Encode(body); err != nil {
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
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.cancelCtx {
				cancel()
			}
			_, err := client.Read(ctx, GroupResourceInstanceRoleAssignmentModel{
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
			// resource.go removes the assignment from state on any "not found" error.
			if tt.wantErr != "not found" && err != nil && strings.Contains(err.Error(), "not found") {
				t.Errorf("Read(%q) error = %v, must not read as not-found", tt.role, err)
			}
			if !slices.Equal(pages, tt.wantPages) {
				t.Errorf("Read(%q) requested pages %v, want %v", tt.role, pages, tt.wantPages)
			}
		})
	}
}
