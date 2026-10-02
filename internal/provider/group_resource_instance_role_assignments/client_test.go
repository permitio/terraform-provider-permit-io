package group_resource_instance_role_assignments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/config"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/httpclient"
)

// TestHTTPRoutesMatchTheClient sends each request the client builds itself to a
// mock that serves only the route for it, and checks that the route receives it.
// The group and the other keys are placeholders: the mock answers 404 for them.
func TestHTTPRoutesMatchTheClient(t *testing.T) {
	model := GroupResourceInstanceRoleAssignmentModel{
		Group:            types.StringValue("x"),
		Role:             types.StringValue("x"),
		Resource:         types.StringValue("x"),
		ResourceInstance: types.StringValue("x"),
		Tenant:           types.StringValue("x"),
	}
	client := func(url string) *groupResourceInstanceRoleAssignmentClient {
		return &groupResourceInstanceRoleAssignmentClient{
			api: &config.API{
				HTTPClient: httpclient.New("test"), URL: url, Key: mockpermit.APIKey,
				ProjectID: mockpermit.ProjectID, EnvironmentID: mockpermit.EnvironmentID,
			},
		}
	}
	const pkg = "group_resource_instance_role_assignments."

	mockpermit.CheckHTTPRoutes(t, mockpermit.GroupRoles,
		map[string]func(ctx context.Context, url string){
			pkg + "Create (HTTP)": func(ctx context.Context, url string) {
				plan := model
				_ = client(url).Create(ctx, &plan)
			},
			pkg + "listRolesPage (HTTP)": func(ctx context.Context, url string) {
				_, _ = client(url).Read(ctx, model)
			},
			pkg + "Delete (HTTP)": func(ctx context.Context, url string) {
				plan := model
				_ = client(url).Delete(ctx, &plan)
			},
		})
}

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
		dropKey   string // counted in total_count but left out of data
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
		// total_count can count roles that the API leaves out of a page, so page 1 is short
		// without being the last. r0's deletion then shifts r100 onto page 1 and leaves
		// page 2 empty: a walk that saw under a full page still needs the second walk.
		{
			name:      "short page, deleted mid-walk",
			roles:     groupRoles(101),
			dropKey:   "r5",
			role:      "r100",
			deleteR0:  true,
			wantPages: []int{1, 2, 1},
		},
		// Page 1 returns 99 rows (r5 omitted) with total_count 99, although r100 still
		// exists, so the first walk ends on that single short page without asking for
		// page 2. r0 is then deleted, which moves r100 onto page 1, where only the second
		// walk finds it: skipping the second walk after a short page drops a live role.
		{
			name:      "short single page, count skewed",
			roles:     groupRoles(101),
			dropKey:   "r5",
			role:      "r100",
			deleteR0:  true,
			totalBias: -2,
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
		// Without total_count only an empty page ends the walk, since a short page may not
		// be the last.
		{
			name:      "missing total_count",
			roles:     groupRoles(250),
			role:      "r150",
			omitTotal: true,
			wantPages: []int{1, 2},
		},
		{
			name:      "missing total_count, missing role",
			roles:     groupRoles(250),
			role:      "missing",
			omitTotal: true,
			wantErr:   "not found",
			wantPages: []int{1, 2, 3, 4, 1, 2, 3, 4},
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
					// A real error body has no total_count, so code that parsed before checking
					// the status would report a parse error and hide the status.
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					err := json.NewEncoder(w).Encode(map[string]any{
						"detail": http.StatusText(status),
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
					if role.key == tt.dropKey {
						continue
					}
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
				api: testAPI(server.Client(), server.URL),
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.cancelCtx {
				cancel()
			}
			want := GroupResourceInstanceRoleAssignmentModel{
				Id:               types.StringValue("developers"),
				Group:            types.StringValue("developers"),
				Role:             types.StringValue(tt.role),
				Resource:         types.StringValue("workspace"),
				ResourceInstance: types.StringValue("ws-1"),
				Tenant:           types.StringValue("default"),
			}
			// An import leaves id unset, and Read sets it.
			imported := want
			imported.Id = types.StringNull()
			got, err := client.Read(ctx, imported)

			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("Read(%q) error = %v, want nil", tt.role, err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("Read(%q) error = %v, want error containing %q", tt.role, err, tt.wantErr)
			}
			// resource.go writes the returned model into state.
			if tt.wantErr == "" && got != want {
				t.Errorf("Read(%q) = %+v, want %+v", tt.role, got, want)
			}
			// resource.go removes the assignment from state when common.IsNotFoundErr
			// reports the error as not found, and fails the read on any other error.
			wantNotFound := tt.wantErr == "not found"
			if err != nil && common.IsNotFoundErr(err) != wantNotFound {
				t.Errorf("Read(%q) error = %v: IsNotFoundErr = %v, want %v",
					tt.role, err, !wantNotFound, wantNotFound)
			}
			if !slices.Equal(pages, tt.wantPages) {
				t.Errorf("Read(%q) requested pages %v, want %v", tt.role, pages, tt.wantPages)
			}
		})
	}
}

// developersEditor is an assignment of the editor role on workspace ws-1 to the
// developers group.
var developersEditor = GroupResourceInstanceRoleAssignmentModel{
	Group:            types.StringValue("developers"),
	Role:             types.StringValue("editor"),
	Resource:         types.StringValue("workspace"),
	ResourceInstance: types.StringValue("ws-1"),
	Tenant:           types.StringValue("acme"),
}

// clientCalls are the calls of the client that each send a request.
var clientCalls = map[string]func(ctx context.Context,
	c *groupResourceInstanceRoleAssignmentClient) error{
	"Create": func(ctx context.Context, c *groupResourceInstanceRoleAssignmentClient) error {
		plan := developersEditor
		return c.Create(ctx, &plan)
	},
	"Read": func(ctx context.Context, c *groupResourceInstanceRoleAssignmentClient) error {
		_, err := c.Read(ctx, developersEditor)
		return err
	},
	"Delete": func(ctx context.Context, c *groupResourceInstanceRoleAssignmentClient) error {
		plan := developersEditor
		return c.Delete(ctx, &plan)
	},
}

// TestRequestsGoThroughTheProviderClient answers the first request of each call
// with 429 Too Many Requests and checks that the call still succeeds, which only
// the provider's retrying HTTP client does, that every attempt names the provider
// in its User-Agent, and that the call sends nothing but its group role request, in
// the project and environment of the provider's API key.
func TestRequestsGoThroughTheProviderClient(t *testing.T) {
	for name, call := range clientCalls {
		t.Run(name, func(t *testing.T) {
			var agents []string
			handler := func(w http.ResponseWriter, r *http.Request) {
				agents = append(agents, r.Header.Get("User-Agent"))
				if r.URL.Path != "/v2/schema/proj/env/groups/developers/roles" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					http.Error(w, "unexpected request", http.StatusNotFound)
					return
				}
				switch {
				case len(agents) == 1:
					w.Header().Set("Retry-After", "0")
					http.Error(w, "slow down", http.StatusTooManyRequests)
				case r.Method == http.MethodGet:
					_, _ = fmt.Fprint(w, `{"data": [{"key": "editor",
						"resource": {"key": "workspace"}, "resource_instance": {"key": "ws-1"}}],
						"total_count": 1}`)
				case r.Method == http.MethodDelete:
					w.WriteHeader(http.StatusNoContent)
				default:
					_, _ = fmt.Fprint(w, `{}`)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(handler))
			defer server.Close()
			client := &groupResourceInstanceRoleAssignmentClient{
				api: testAPI(httpclient.New("test"), server.URL),
			}

			if err := call(t.Context(), client); err != nil {
				t.Errorf("%s() error = %v, want the retry to succeed", name, err)
			}
			want := httpclient.UserAgent("test")
			if len(agents) != 2 || agents[0] != want || agents[1] != want {
				t.Errorf("%s() sent requests with the User-Agents %q, want two with %q",
					name, agents, want)
			}
		})
	}
}

// TestRequestsNeedTheProviderConfiguration checks that a client the provider has
// not configured, or configured with an API key that is not scoped to one
// environment, fails every call without sending a request, even when API keys are
// set in the environment.
func TestRequestsNeedTheProviderConfiguration(t *testing.T) {
	t.Setenv("PERMITIO_API_KEY", "key-from-env")
	t.Setenv("PERMIT_API_KEY", "key-from-env")
	// A request would otherwise go to the real Permit API.
	blocked := &http.Client{Transport: roundTripFunc(
		func(r *http.Request) (*http.Response, error) {
			t.Errorf("sent %s %s", r.Method, r.URL)
			return nil, errors.New("requests are blocked in this test")
		})}
	defaultTransport := http.DefaultTransport
	http.DefaultTransport = blocked.Transport
	t.Cleanup(func() { http.DefaultTransport = defaultTransport })

	tests := []struct {
		name string
		api  *config.API
		want error
	}{
		{name: "not configured", want: errNotConfigured},
		{
			name: "no HTTP client",
			api: &config.API{
				URL: "https://api.example.com", Key: "key", ProjectID: "proj", EnvironmentID: "env",
			},
			want: errNotConfigured,
		},
		{
			name: "no project",
			api: &config.API{
				HTTPClient: blocked, URL: "https://api.example.com", Key: "key",
				EnvironmentID: "env",
			},
			want: errNoEnvironment,
		},
		{
			name: "no environment",
			api: &config.API{
				HTTPClient: blocked, URL: "https://api.example.com", Key: "key", ProjectID: "proj",
			},
			want: errNoEnvironment,
		},
	}
	for _, tt := range tests {
		for name, call := range clientCalls {
			client := &groupResourceInstanceRoleAssignmentClient{api: tt.api}
			if err := call(t.Context(), client); !errors.Is(err, tt.want) {
				t.Errorf("%s: %s() error = %v, want %v", tt.name, name, err, tt.want)
			}
		}
	}
}

// TestGroupKeyIsEscaped sends each call for a group whose key holds characters that
// would otherwise end its path segment or the path, and checks that every request
// reaches the roles of that group.
func TestGroupKeyIsEscaped(t *testing.T) {
	const group = "dev/ops#1?x y"
	var got []string
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/schema/proj/env/groups/{group}/roles",
		func(w http.ResponseWriter, r *http.Request) {
			got = append(got, r.Method+" "+r.PathValue("group"))
			switch r.Method {
			case http.MethodGet:
				_, _ = fmt.Fprint(w, `{"data": [{"key": "editor",
					"resource": {"key": "workspace"}, "resource_instance": {"key": "ws-1"}}],
					"total_count": 1}`)
			case http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				_, _ = fmt.Fprint(w, `{}`)
			}
		})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := &groupResourceInstanceRoleAssignmentClient{api: testAPI(server.Client(), server.URL)}
	model := developersEditor
	model.Group = types.StringValue(group)

	plan := model
	if err := client.Create(t.Context(), &plan); err != nil {
		t.Errorf("Create() error = %v", err)
	}
	if _, err := client.Read(t.Context(), model); err != nil {
		t.Errorf("Read() error = %v", err)
	}
	plan = model
	if err := client.Delete(t.Context(), &plan); err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	want := []string{http.MethodPost + " " + group, http.MethodGet + " " + group,
		http.MethodDelete + " " + group}
	if !slices.Equal(got, want) {
		t.Errorf("requests = %q, want %q", got, want)
	}
}

// testAPI returns a connection to url through httpClient, with the API key token,
// in the project proj and the environment env.
func testAPI(httpClient *http.Client, url string) *config.API {
	return &config.API{
		HTTPClient: httpClient, URL: url, Key: "token", ProjectID: "proj", EnvironmentID: "env",
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestReadStopsAtThePageLimit runs Read against an API that answers every page with
// a full page of other roles and no total_count, and checks that Read gives up after
// maxGroupRolePages pages with an error that says so, instead of listing forever or
// reporting the assignment as gone.
func TestReadStopsAtThePageLimit(t *testing.T) {
	requests := 0
	fullPage := `{"data": [` + strings.Repeat(`{"key": "viewer", "resource": {"key": "workspace"},
		"resource_instance": {"key": "ws-1"}},`, groupRolesPerPage-1) +
		`{"key": "viewer", "resource": {"key": "workspace"},
		"resource_instance": {"key": "ws-1"}}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > maxGroupRolePages {
			t.Errorf("request %d: %s, after %d pages", requests, r.URL, maxGroupRolePages)
			http.Error(w, "too many requests", http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprint(w, fullPage)
	}))
	defer server.Close()
	client := &groupResourceInstanceRoleAssignmentClient{api: testAPI(server.Client(), server.URL)}

	_, err := client.Read(t.Context(), developersEditor)

	want := `list roles of group "developers": stopped after 1000 pages of up to 100 roles ` +
		`without reaching the last page`
	if err == nil || err.Error() != want {
		t.Errorf("Read() error = %v, want %q", err, want)
	}
	if common.IsNotFoundErr(err) {
		t.Errorf("Read() error = %v counts as not found, which would drop the assignment", err)
	}
	if requests != maxGroupRolePages {
		t.Errorf("Read() sent %d requests, want %d", requests, maxGroupRolePages)
	}
}
