package role_assignments

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

const (
	listPath = "/v2/facts/proj/env/role_assignments"
	// inTenant stands for the tenant-level assignment in a list of instances.
	inTenant = ""
)

// aliceViewer is alice's assignment of the viewer role in acme, which each test
// reads.
var aliceViewer = RoleAssignmentModel{
	User:   types.StringValue("alice"),
	Role:   types.StringValue("viewer"),
	Tenant: types.StringValue("acme"),
}

// assignment returns alice's assignment of the viewer role in acme, as the role
// assignment list returns it: on instance, or in the tenant when instance is
// inTenant.
func assignment(instance string) map[string]any {
	row := map[string]any{
		"id": "assignment-in-tenant", "user": "alice", "role": "viewer", "tenant": "acme",
		"user_id": "u", "role_id": "r", "tenant_id": "t", "organization_id": "org",
		"project_id": "proj", "environment_id": "env", "created_at": "2026-01-01T00:00:00Z",
		"resource_instance": nil,
	}
	if instance != inTenant {
		row["id"] = "assignment-on-" + instance
		row["resource_instance"] = instance
		row["resource_instance_id"] = "instance-" + instance
	}
	return row
}

// instances returns n document instances.
func instances(n int) []string {
	listed := make([]string, n)
	for i := range listed {
		listed[i] = fmt.Sprintf("document:doc-%d", i)
	}
	return listed
}

// newClient returns a client of the API at url, in the project proj and the
// environment env.
func newClient(url string) roleAssignmentClient {
	return roleAssignmentClient{client: permit.NewPermit(
		config.NewConfigBuilder("key").WithApiUrl(url).
			WithContext(config.NewPermitContext(config.EnvironmentAPIKeyLevel, "proj", "env")).
			Build())}
}

func TestRead(t *testing.T) {
	tests := []struct {
		name string
		// listed are alice's assignments of the viewer role in acme, in list order: an
		// instance for each assignment on one, and inTenant for the tenant-level one.
		listed []string
		// errOn is the number of the request that fails with errStatus.
		errOn     int
		errStatus int
		// shortFirst leaves the first five assignments out of page 1, which makes the
		// page short without being the last.
		shortFirst bool
		// deleteFirst removes the first assignment after the first request.
		deleteFirst bool
		// blankInstance sends the tenant-level assignment's resource_instance as ""
		// instead of null.
		blankInstance bool
		wantErr       string // "" means found
		notFound      bool
		wantPages     []int
	}{
		{
			name:      "instance assignments listed first",
			listed:    append(instances(2), inTenant),
			wantPages: []int{1},
		},
		{
			name:          "tenant assignment with an empty resource_instance",
			listed:        append(instances(2), inTenant),
			blankInstance: true,
			wantPages:     []int{1},
		},
		{
			name:      "later page",
			listed:    append(instances(250), inTenant),
			wantPages: []int{1, 2, 3},
		},
		{
			name:      "instance assignments only",
			listed:    instances(250),
			wantErr:   "not found",
			notFound:  true,
			wantPages: []int{1, 2, 3, 4, 1, 2, 3, 4},
		},
		{
			name:      "no assignments",
			wantErr:   "not found",
			notFound:  true,
			wantPages: []int{1, 1},
		},
		{
			name:       "short page before the last",
			listed:     append(instances(100), inTenant),
			shortFirst: true,
			wantPages:  []int{1, 2},
		},
		// The tenant-level assignment moves from page 2 onto page 1, which the first
		// walk already read.
		{
			name:        "deleted mid-walk",
			listed:      slices.Concat(instances(100), []string{inTenant}, instances(49)),
			deleteFirst: true,
			wantPages:   []int{1, 2, 3, 1},
		},
		// A failed page fails the refresh, whichever page it is, and never reads as a
		// deletion.
		{
			name:      "server error on a later page",
			listed:    append(instances(250), inTenant),
			errOn:     2,
			errStatus: http.StatusInternalServerError,
			wantErr:   "list role assignments (page 2): ",
			wantPages: []int{1, 2},
		},
		{
			name:      "server error on the second walk",
			listed:    instances(150),
			errOn:     4,
			errStatus: http.StatusInternalServerError,
			wantErr:   "list role assignments (page 1): ",
			wantPages: []int{1, 2, 3, 1},
		},
		// The wrapped error keeps the SDK's, so a 404 still reads as not found.
		{
			name:      "not found on a later page",
			listed:    append(instances(250), inTenant),
			errOn:     2,
			errStatus: http.StatusNotFound,
			wantErr:   "list role assignments (page 2): ",
			notFound:  true,
			wantPages: []int{1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pages []int
			listed := tt.listed
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query()
				page, err := strconv.Atoi(query.Get("page"))
				if r.Method != http.MethodGet || r.URL.Path != listPath || err != nil ||
					query.Get("per_page") != strconv.Itoa(assignmentsPerPage) ||
					query.Get("user") != "alice" || query.Get("role") != "viewer" ||
					query.Get("tenant") != "acme" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				pages = append(pages, page)
				// A walk that never stops fails here instead of hanging until the test timeout.
				if len(pages) > 20 {
					t.Errorf("too many requests: %v", pages)
					http.Error(w, "too many requests", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if len(pages) == tt.errOn {
					w.WriteHeader(tt.errStatus)
					_, _ = fmt.Fprintf(w, `{"detail": %q}`, http.StatusText(tt.errStatus))
					return
				}

				start := min((page-1)*assignmentsPerPage, len(listed))
				end := min(page*assignmentsPerPage, len(listed))
				if tt.shortFirst && page == 1 {
					start = min(5, end)
				}
				rows := []map[string]any{}
				for _, instance := range listed[start:end] {
					row := assignment(instance)
					if tt.blankInstance && instance == inTenant {
						row["resource_instance"] = ""
					}
					rows = append(rows, row)
				}
				if err := json.NewEncoder(w).Encode(rows); err != nil {
					t.Errorf("writing the response: %v", err)
				}
				if tt.deleteFirst && len(pages) == 1 {
					listed = listed[1:]
				}
			}))
			defer server.Close()
			client := newClient(server.URL)

			got, err := client.Read(t.Context(), aliceViewer)

			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("Read() error = %v, want nil", err)
			case tt.wantErr == "" && got.Id.ValueString() != "assignment-in-tenant":
				t.Errorf("Read() = %+v, want the tenant-level assignment", got)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("Read() error = %v, want one containing %q", err, tt.wantErr)
			}
			// resource.go removes the assignment from state when common.IsNotFoundErr
			// reports the error as not found, and fails the refresh on any other error.
			if err != nil && common.IsNotFoundErr(err) != tt.notFound {
				t.Errorf("Read() error = %v: IsNotFoundErr = %v, want %v",
					err, !tt.notFound, tt.notFound)
			}
			if !slices.Equal(pages, tt.wantPages) {
				t.Errorf("Read() requested pages %v, want %v", pages, tt.wantPages)
			}
		})
	}
}

// TestReadStopsAtThePageLimit runs Read against an API that answers every page with
// a full page of assignments on instances, and checks that Read gives up after
// maxAssignmentPages pages with an error that says so, instead of listing forever or
// reporting the assignment as gone.
func TestReadStopsAtThePageLimit(t *testing.T) {
	var mu sync.Mutex
	var pages []int
	fullPage := make([]map[string]any, assignmentsPerPage)
	for i, instance := range instances(assignmentsPerPage) {
		fullPage[i] = assignment(instance)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != listPath {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		page, err := strconv.Atoi(r.URL.Query().Get("page"))
		mu.Lock()
		pages = append(pages, page)
		tooMany := len(pages) > maxAssignmentPages
		mu.Unlock()
		if err != nil || tooMany {
			t.Errorf("unexpected request %s, after %d pages", r.URL, maxAssignmentPages)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(fullPage); err != nil {
			t.Errorf("writing the response: %v", err)
		}
	}))
	defer server.Close()
	client := newClient(server.URL)

	_, err := client.Read(t.Context(), aliceViewer)

	want := `stopped after 1000 pages of up to 100 assignments of role "viewer" to user ` +
		`"alice" in tenant "acme" without reaching the last page`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Read() error = %v, want one containing %q", err, want)
	}
	if common.IsNotFoundErr(err) {
		t.Errorf("Read() error = %v counts as not found, which would drop the assignment", err)
	}
	if len(pages) != maxAssignmentPages || pages[0] != 1 ||
		pages[len(pages)-1] != maxAssignmentPages {
		t.Errorf("Read() listed %d pages, want pages 1 to %d", len(pages), maxAssignmentPages)
	}
}
