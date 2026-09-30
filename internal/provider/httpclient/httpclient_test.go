package httpclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflogtest"
)

// server is a test HTTP server that answers the requests it gets in order from
// statuses, and then with 200 OK. It records the body of every request.
type server struct {
	URL string

	mu       sync.Mutex
	statuses []int
	// retryAfter is the Retry-After header of every answer that is not 200.
	retryAfter string
	bodies     []string
	agents     []string
}

func newServer(t *testing.T, retryAfter string, statuses ...int) *server {
	t.Helper()
	s := &server{statuses: statuses, retryAfter: retryAfter}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the request body: %v", err)
		}
		s.mu.Lock()
		s.bodies = append(s.bodies, string(body))
		s.agents = append(s.agents, r.Header.Get("User-Agent"))
		status := http.StatusOK
		if len(s.statuses) > 0 {
			status, s.statuses = s.statuses[0], s.statuses[1:]
		}
		s.mu.Unlock()
		if status != http.StatusOK && s.retryAfter != "" {
			w.Header().Set("Retry-After", s.retryAfter)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	return s
}

func (s *server) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...)
}

// quickTransport retries with waits short enough for a test, unless the response
// says otherwise with Retry-After.
func quickTransport() *Transport {
	return &Transport{
		UserAgent:   UserAgent("1.2.3"),
		MaxAttempts: 4,
		BaseDelay:   time.Millisecond,
		MaxDelay:    5 * time.Second,
	}
}

// send sends a request with method and body through transport and returns the
// response status.
func send(t *testing.T, transport http.RoundTripper, method, url, body string) int {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestRetriesTooManyRequestsAfterRetryAfter(t *testing.T) {
	srv := newServer(t, "1", http.StatusTooManyRequests)

	start := time.Now()
	status := send(t, quickTransport(), http.MethodGet, srv.URL, "")
	elapsed := time.Since(start)

	if status != http.StatusOK || len(srv.requests()) != 2 {
		t.Fatalf("got %d after %d requests, want 200 after 2 (a 429, then its retry)",
			status, len(srv.requests()))
	}
	if elapsed < time.Second {
		t.Errorf("the retry came %s after the 429, want at least its Retry-After of 1s",
			elapsed)
	}
}

func TestRetryRules(t *testing.T) {
	tests := []struct {
		method string
		status int
		want   int
	}{
		{http.MethodGet, http.StatusTooManyRequests, 2},
		{http.MethodPost, http.StatusTooManyRequests, 2},
		{http.MethodPatch, http.StatusTooManyRequests, 2},
		{http.MethodGet, http.StatusBadGateway, 2},
		{http.MethodGet, http.StatusServiceUnavailable, 2},
		{http.MethodGet, http.StatusGatewayTimeout, 2},
		{http.MethodPut, http.StatusServiceUnavailable, 2},
		{http.MethodDelete, http.StatusServiceUnavailable, 2},
		{http.MethodPost, http.StatusServiceUnavailable, 1},
		{http.MethodPost, http.StatusBadGateway, 1},
		{http.MethodPost, http.StatusGatewayTimeout, 1},
		{http.MethodPatch, http.StatusServiceUnavailable, 1},
		{http.MethodGet, http.StatusInternalServerError, 1},
		{http.MethodGet, http.StatusNotFound, 1},
		{http.MethodDelete, http.StatusConflict, 1},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+http.StatusText(tt.status), func(t *testing.T) {
			srv := newServer(t, "", tt.status)

			status := send(t, quickTransport(), tt.method, srv.URL, "")

			if got := len(srv.requests()); got != tt.want {
				t.Errorf("requests = %d, want %d", got, tt.want)
			}
			wantStatus := tt.status
			if tt.want == 2 {
				wantStatus = http.StatusOK
			}
			if status != wantStatus {
				t.Errorf("status = %d, want %d", status, wantStatus)
			}
		})
	}
}

// failFirst is a base transport whose first attempt fails with err, and which
// sends later attempts to the network.
type failFirst struct {
	err   error
	mu    sync.Mutex
	calls int
}

func (f *failFirst) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.calls++
	first := f.calls == 1
	f.mu.Unlock()
	if first {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, f.err
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestRetriesConnectionErrors(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	reset := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	noHost := &net.OpError{Op: "dial", Net: "tcp",
		Err: &net.DNSError{Err: "no such host", Name: "api.invalid", IsNotFound: true}}
	tests := []struct {
		name   string
		method string
		err    error
		want   int
	}{
		{"GET that could not connect", http.MethodGet, refused, 2},
		{"POST that could not connect", http.MethodPost, refused, 2},
		{"PATCH that could not connect", http.MethodPatch, refused, 2},
		{"GET whose connection was reset", http.MethodGet, reset, 2},
		{"DELETE whose connection closed early", http.MethodDelete, io.ErrUnexpectedEOF, 2},
		{"POST whose connection was reset", http.MethodPost, reset, 1},
		{"GET to a host that does not resolve", http.MethodGet, noHost, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer(t, "")
			base := &failFirst{err: tt.err}
			transport := quickTransport()
			transport.Base = base
			req, err := http.NewRequestWithContext(t.Context(), tt.method, srv.URL,
				strings.NewReader(`{"key":"acme"}`))
			if err != nil {
				t.Fatal(err)
			}

			resp, err := transport.RoundTrip(req)

			if resp != nil {
				_ = resp.Body.Close()
			}
			if base.calls != tt.want {
				t.Fatalf("attempts = %d, want %d", base.calls, tt.want)
			}
			if tt.want == 1 && !errors.Is(err, tt.err) {
				t.Errorf("RoundTrip() error = %v, want %v", err, tt.err)
			}
			if tt.want == 2 && (err != nil || resp.StatusCode != http.StatusOK) {
				t.Errorf("RoundTrip() = %v, %v, want 200 from the retry", resp, err)
			}
			if tt.want == 2 && srv.requests()[0] != `{"key":"acme"}` {
				t.Errorf("retried body = %q, want the original body", srv.requests()[0])
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// TestRetriesRefusedConnection checks that the error http.DefaultTransport returns
// for a refused connection is one the Transport retries, for a POST too.
func TestRetriesRefusedConnection(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	var attempts atomic.Int32
	transport := quickTransport()
	transport.Base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return http.DefaultTransport.RoundTrip(req)
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, closed.URL,
		strings.NewReader(`{"key":"acme"}`))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.RoundTrip(req)

	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, syscall.ECONNREFUSED) || attempts.Load() != 4 {
		t.Errorf("RoundTrip() error = %v after %d attempts, want %v after MaxAttempts (4)",
			err, attempts.Load(), syscall.ECONNREFUSED)
	}
}

// bodyRecorder is a base transport that reads the body of every attempt and
// answers 429 until the last attempt, which it answers with 200. It reads the body
// itself, as http.Transport does, because http.Transport can also resend a body
// on its own after some failures, which would hide a Transport that does not.
type bodyRecorder struct {
	attempts int
	bodies   []string
}

func (b *bodyRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	b.bodies = append(b.bodies, string(body))
	status := http.StatusTooManyRequests
	if len(b.bodies) == b.attempts {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Status:     strconv.Itoa(status) + " " + http.StatusText(status),
		Header:     http.Header{"Retry-After": {"0"}},
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

func TestReplaysBody(t *testing.T) {
	const body = `{"key":"acme","name":"Acme"}`
	base := &bodyRecorder{attempts: 3}
	transport := quickTransport()
	transport.Base = base

	status := send(t, transport, http.MethodPost, "http://permit.test/v2/tenants", body)

	if status != http.StatusOK || len(base.bodies) != 3 {
		t.Fatalf("got %d after %d attempts, want 200 after 3", status, len(base.bodies))
	}
	for i, sent := range base.bodies {
		if sent != body {
			t.Errorf("attempt %d sent body %q, want %q", i+1, sent, body)
		}
	}
}

func TestDoesNotRetryBodyItCannotReplay(t *testing.T) {
	srv := newServer(t, "0", http.StatusTooManyRequests)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL,
		io.NopCloser(strings.NewReader("once")))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := quickTransport().RoundTrip(req)

	if err != nil {
		t.Fatalf("RoundTrip() error = %v, want the 429", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || len(srv.requests()) != 1 {
		t.Errorf("got %d after %d requests, want 429 after 1",
			resp.StatusCode, len(srv.requests()))
	}
}

func TestFailsWhenBodyReplayFails(t *testing.T) {
	srv := newServer(t, "0", http.StatusTooManyRequests)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL,
		strings.NewReader("once"))
	if err != nil {
		t.Fatal(err)
	}
	errReplay := errors.New("body already consumed")
	req.GetBody = func() (io.ReadCloser, error) { return nil, errReplay }

	resp, err := quickTransport().RoundTrip(req)

	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, errReplay) || len(srv.requests()) != 1 {
		t.Errorf("RoundTrip() error = %v after %d requests, want %q after 1",
			err, len(srv.requests()), errReplay)
	}
}

func TestStopsAtMaxAttempts(t *testing.T) {
	srv := newServer(t, "", http.StatusServiceUnavailable, http.StatusServiceUnavailable,
		http.StatusServiceUnavailable, http.StatusServiceUnavailable,
		http.StatusServiceUnavailable)

	status := send(t, quickTransport(), http.MethodGet, srv.URL, "")

	if status != http.StatusServiceUnavailable || len(srv.requests()) != 4 {
		t.Errorf("got %d after %d requests, want 503 after MaxAttempts (4)",
			status, len(srv.requests()))
	}
}

func TestDoesNotWaitPastDeadline(t *testing.T) {
	srv := newServer(t, "60", http.StatusTooManyRequests)
	transport := quickTransport()
	transport.MaxDelay = time.Minute
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}

	start := time.Now()
	resp, err := client.Get(srv.URL)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("GET error = %v, want the 429 response before the client timeout", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || len(srv.requests()) != 1 {
		t.Errorf("got %d after %d requests, want 429 after 1",
			resp.StatusCode, len(srv.requests()))
	}
	if elapsed > time.Second {
		t.Errorf("GET took %s, want the 429 at once instead of a wait past the timeout",
			elapsed)
	}
}

func TestStopsWaitingWhenCancelled(t *testing.T) {
	srv := newServer(t, "30", http.StatusTooManyRequests)
	transport := quickTransport()
	transport.MaxDelay = time.Minute
	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(100*time.Millisecond, cancel)

	start := time.Now()
	resp, err := transport.RoundTrip(req)

	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, context.Canceled) || time.Since(start) > 5*time.Second {
		t.Errorf("RoundTrip() error = %v after %s, want %v as soon as it is cancelled",
			err, time.Since(start), context.Canceled)
	}
}

func TestSetsUserAgent(t *testing.T) {
	srv := newServer(t, "0", http.StatusTooManyRequests)
	client := New("1.2.3")
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "OpenAPI-Generator/1.0.0/go")

	resp, err := client.Do(req)

	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for i, agent := range srv.agents {
		if agent != "terraform-provider-permitio/1.2.3" {
			t.Errorf("attempt %d User-Agent = %q, want %q", i+1, agent,
				"terraform-provider-permitio/1.2.3")
		}
	}
	if len(srv.agents) != 2 {
		t.Errorf("requests = %d, want 2 (a 429, then its retry)", len(srv.agents))
	}
	if got := req.Header.Get("User-Agent"); got != "OpenAPI-Generator/1.0.0/go" {
		t.Errorf("the caller's request User-Agent = %q, want it unchanged", got)
	}
}

// TestRetryLogLeavesOutURL checks what a retry logs: the method, the reason, the
// attempt and the wait, and never the request URL, whose path and query can hold
// the keys of users and other objects.
func TestRetryLogLeavesOutURL(t *testing.T) {
	const objectKey = "user-key-in-the-url"
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	tests := []struct {
		name   string
		url    string
		reason string
	}{
		{"429", newServer(t, "0", http.StatusTooManyRequests).URL, "429 Too Many Requests"},
		{"refused connection", closed.URL, "connection refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			ctx := tflogtest.RootLogger(t.Context(), &output)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet,
				tt.url+"/v2/facts/users/"+objectKey+"?search="+objectKey, nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, _ := quickTransport().RoundTrip(req)

			if resp != nil {
				_ = resp.Body.Close()
			}
			if strings.Contains(output.String(), objectKey) {
				t.Errorf("retry log = %s, want no request URL", output.String())
			}
			entries, err := tflogtest.MultilineJSONDecode(&output)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) == 0 {
				t.Fatal("no retry logged, want one per retry")
			}
			wantKeys := []string{"@level", "@message", "@module", "attempt", "method",
				"reason", "wait"}
			for _, entry := range entries {
				if keys := slices.Sorted(maps.Keys(entry)); !slices.Equal(keys, wantKeys) {
					t.Errorf("retry log fields = %v, want %v", keys, wantKeys)
				}
				if reason, _ := entry["reason"].(string); !strings.Contains(reason, tt.reason) {
					t.Errorf("retry log reason = %q, want it to contain %q", reason, tt.reason)
				}
			}
		})
	}
}

func TestDelay(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	transport := &Transport{BaseDelay: time.Second, MaxDelay: 16 * time.Second}
	tests := []struct {
		name       string
		attempt    int
		retryAfter string
		want       time.Duration
	}{
		{name: "backoff starts at BaseDelay", attempt: 1, want: time.Second},
		{name: "backoff doubles", attempt: 3, want: 4 * time.Second},
		{name: "backoff stops at MaxDelay", attempt: 100, want: 16 * time.Second},
		{name: "Retry-After seconds", attempt: 4, retryAfter: "3", want: 3 * time.Second},
		{name: "Retry-After zero", attempt: 3, retryAfter: "0", want: 0},
		{name: "Retry-After over MaxDelay", attempt: 1, retryAfter: "120", want: 16 * time.Second},
		{
			name: "Retry-After date", attempt: 1,
			retryAfter: now.Add(5 * time.Second).Format(http.TimeFormat), want: 5 * time.Second,
		},
		{
			name: "Retry-After date in the past", attempt: 1,
			retryAfter: now.Add(-time.Minute).Format(http.TimeFormat), want: 0,
		},
		{
			name: "malformed Retry-After falls back to backoff", attempt: 2,
			retryAfter: "soon", want: 2 * time.Second,
		},
		{
			name: "negative Retry-After falls back to backoff", attempt: 2,
			retryAfter: "-1", want: 2 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.retryAfter != "" {
				resp.Header.Set("Retry-After", tt.retryAfter)
			}
			seen := map[time.Duration]bool{}
			for range 50 {
				got := transport.delay(tt.attempt, resp, now)
				if got < tt.want || got > tt.want+tt.want/4 {
					t.Fatalf("delay() = %s, want between %s and %s",
						got, tt.want, tt.want+tt.want/4)
				}
				seen[got] = true
			}
			if tt.want > 0 && len(seen) < 2 {
				t.Errorf("delay() returned %s every time, want jitter", tt.want)
			}
		})
	}

	t.Run("after a connection error", func(t *testing.T) {
		if got := transport.delay(2, nil, now); got < 2*time.Second ||
			got > 2*time.Second+time.Second/2 {
			t.Errorf("delay() = %s, want the backoff of 2s", got)
		}
	})
}
