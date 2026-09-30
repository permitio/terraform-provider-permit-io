package provider

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// configure runs the provider's Configure with an empty provider block, so the
// API URL, key and timeout come from the environment.
func configure(t *testing.T) *fwprovider.ConfigureResponse {
	t.Helper()
	ctx := t.Context()
	p := New("test")()
	var schemaResp fwprovider.SchemaResponse
	p.Schema(ctx, fwprovider.SchemaRequest{}, &schemaResp)
	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx),
			map[string]tftypes.Value{
				"api_url": tftypes.NewValue(tftypes.String, nil),
				"api_key": tftypes.NewValue(tftypes.String, nil),
				"timeout": tftypes.NewValue(tftypes.Number, nil),
			}),
	}
	resp := &fwprovider.ConfigureResponse{}
	p.Configure(ctx, fwprovider.ConfigureRequest{Config: config}, resp)
	return resp
}

// configuredClient runs Configure and returns the SDK client it gives resources.
func configuredClient(t *testing.T) *permit.Client {
	t.Helper()
	resp := configure(t)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure() diagnostics = %v, want none", resp.Diagnostics)
	}
	client, ok := resp.ResourceData.(*permit.Client)
	if !ok {
		t.Fatalf("Configure() ResourceData is %T, want *permit.Client", resp.ResourceData)
	}
	return client
}

// TestConfigureResolvesScopeOnce creates 50 tenants at once through the client
// Configure hands to resources, as a parallel apply does. The API key scope is
// read once, in Configure: the SDK would otherwise read it again on every call
// that starts before one has finished, racing on where it stores the answer.
func TestConfigureResolvesScopeOnce(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Tenants)
	client := configuredClient(t)
	const creates = 50

	var wg sync.WaitGroup
	errs := make(chan error, creates)
	for i := range creates {
		wg.Go(func() {
			key := fmt.Sprintf("tenant-%d", i)
			if _, err := client.Api.Tenants.Create(t.Context(),
				*models.NewTenantCreate(key, key)); err != nil {
				errs <- fmt.Errorf("creating %s: %w", key, err)
			}
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
	if got := len(m.Requests(http.MethodGet, apiKeyScopePath)); got != 1 {
		t.Errorf("API key scope requests = %d, want 1", got)
	}
	posts := m.Requests(http.MethodPost, "/v2/facts/"+mockpermit.ProjectID+"/"+
		mockpermit.EnvironmentID+"/tenants")
	if len(posts) != creates {
		t.Errorf("tenant creates = %d, want %d", len(posts), creates)
	}
	for _, r := range append(m.Requests(http.MethodGet, apiKeyScopePath), posts...) {
		if got := r.Header.Get("User-Agent"); got != "terraform-provider-permitio/test" {
			t.Errorf("%s %s User-Agent = %q, want %q", r.Method, r.Path, got,
				"terraform-provider-permitio/test")
			break
		}
	}
}

// scopeServer points the provider at a fake API that serves only the API key
// scope. It answers the first scope requests with replies, in order, and later ones
// with a scope, and it counts them.
func scopeServer(t *testing.T, replies ...func(w http.ResponseWriter)) *atomic.Int32 {
	t.Helper()
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != apiKeyScopePath {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		n := int(count.Add(1))
		w.Header().Set("Content-Type", "application/json")
		if n <= len(replies) {
			replies[n-1](w)
			return
		}
		_, _ = io.WriteString(w, fakeScopeJSON)
	}))
	t.Cleanup(server.Close)
	t.Setenv("PERMITIO_API_URL", server.URL)
	t.Setenv("PERMITIO_API_KEY", "fake-key")
	return &count
}

// configureError runs Configure, checks that it failed without handing resources a
// client, and returns the error detail after checking that it contains every
// string in want and not the SDK's ContextError.
func configureError(t *testing.T, want ...string) string {
	t.Helper()
	resp := configure(t)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Configure() has no error, want one")
	}
	if resp.ResourceData != nil || resp.DataSourceData != nil {
		t.Error("Configure() failed but set the client, want none")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, w := range want {
		if !strings.Contains(detail, w) {
			t.Errorf("error detail = %q, want it to contain %q", detail, w)
		}
	}
	if strings.Contains(detail, "ContextError") {
		t.Errorf("error detail = %q, want the API's answer, not the SDK's ContextError",
			detail)
	}
	return detail
}

func TestConfigureFailsWithScopeError(t *testing.T) {
	const endOfPage = "END-OF-PAGE"
	largePage := "<html><body>" + strings.Repeat("<p>Bad gateway</p>", 200) + endOfPage +
		"</body></html>"
	tests := []struct {
		name    string
		reply   func(w http.ResponseWriter)
		want    []string
		notWant []string
	}{
		{
			name: "rejected API key",
			reply: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"error_code":"UNAUTHORIZED",`+
					`"title":"Unauthorized","message":"Invalid API key"}`)
			},
			want: []string{apiKeyScopePath, "401 Unauthorized", "Invalid API key",
				"rejected the API key", "PERMITIO_API_KEY"},
		},
		{
			name: "error without a message",
			reply: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, "access denied\n")
			},
			want: []string{apiKeyScopePath, "403 Forbidden", "access denied",
				"rejected the API key"},
		},
		{
			name: "unavailable API",
			reply: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "down for maintenance")
			},
			want: []string{apiKeyScopePath, "503 Service Unavailable",
				"down for maintenance", "busy or unavailable", "PERMITIO_TIMEOUT"},
			notWant: []string{"rejected the API key"},
		},
		{
			name: "large HTML error page",
			reply: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusBadGateway)
				_, _ = io.WriteString(w, largePage)
			},
			want: []string{apiKeyScopePath, "502 Bad Gateway", "<p>Bad gateway</p>",
				"... (truncated)"},
			notWant: []string{endOfPage},
		},
		{
			name:    "scope that is not JSON",
			reply:   func(w http.ResponseWriter) { _, _ = io.WriteString(w, "not json") },
			want:    []string{apiKeyScopePath, "invalid character", "PERMITIO_API_URL"},
			notWant: []string{"rejected the API key", "busy or unavailable"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := scopeServer(t, tt.reply)
			t.Setenv("PERMITIO_TIMEOUT", "1")

			detail := configureError(t, tt.want...)

			for _, notWant := range append(tt.notWant, "200 OK") {
				if strings.Contains(detail, notWant) {
					t.Errorf("error detail = %q, want no %q", detail, notWant)
				}
			}
			if got := count.Load(); got != 1 {
				t.Errorf("scope requests = %d, want 1 (the answer is not retried)", got)
			}
		})
	}
}

func TestConfigureFailsWhenAPIUnreachable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	t.Setenv("PERMITIO_API_URL", server.URL)
	t.Setenv("PERMITIO_API_KEY", "fake-key")
	t.Setenv("PERMITIO_TIMEOUT", "1")

	configureError(t, apiKeyScopePath, "connection refused", "PERMITIO_API_URL")
}

func TestConfigureRetriesRateLimitedScope(t *testing.T) {
	count := scopeServer(t, func(w http.ResponseWriter) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	configuredClient(t)

	if got := count.Load(); got != 2 {
		t.Errorf("scope requests = %d, want 2 (a 429, then its retry)", got)
	}
}

// TestConfigureAppliesTimeout checks that PERMITIO_TIMEOUT bounds the requests of
// the client Configure builds, retries and all.
func TestConfigureAppliesTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == apiKeyScopePath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, fakeScopeJSON)
			return
		}
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	t.Setenv("PERMITIO_API_URL", server.URL)
	t.Setenv("PERMITIO_API_KEY", "fake-key")
	t.Setenv("PERMITIO_TIMEOUT", "1")
	client := configuredClient(t)

	start := time.Now()
	_, err := client.Api.Tenants.Get(t.Context(), "acme")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Tenants.Get() of a request that never answers succeeded, want a timeout")
	}
	if elapsed > 3*time.Second {
		t.Errorf("Tenants.Get() returned after %s, want about the 1s timeout", elapsed)
	}
}
