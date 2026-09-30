package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	permitConfig "github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

// TestMain routes every request this package's tests make through the 429 retry.
// The SDK's http.Client and the http.DefaultClient used by the group role client
// both leave Transport nil, so they send through http.DefaultTransport.
func TestMain(m *testing.M) {
	http.DefaultTransport = &retry429Transport{
		base:        http.DefaultTransport,
		maxAttempts: 6,
		baseDelay:   time.Second,
		maxDelay:    16 * time.Second,
		// plugin-testing sends the standard logger to TF_LOG's output for every
		// test, which discards it when TF_LOG is unset, as in CI.
		logger: log.New(os.Stderr, "", log.LstdFlags),
	}
	m.Run()
}

// retry429Transport resends a request that the Permit API rejected with 429 Too
// Many Requests. The five acceptance legs call the Permit API at the same time and
// their runs have been answered with 429, so a test retries instead of failing. It
// lives in test code because the provider itself does not retry yet.
type retry429Transport struct {
	base        http.RoundTripper
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	logger      *log.Logger
}

func (rt *retry429Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	attemptReq := req
	for attempt := 1; ; attempt++ {
		resp, err := rt.base.RoundTrip(attemptReq)
		if err != nil || resp.StatusCode != http.StatusTooManyRequests ||
			attempt == rt.maxAttempts {
			return resp, err
		}
		wait := rt.delay(attempt, resp.Header.Get("Retry-After"), time.Now())
		if !canResend(req, wait) {
			return resp, nil
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		rt.logger.Printf("Permit API answered 429 to %s; retry %d of %d in %s",
			req.Method, attempt, rt.maxAttempts-1, wait)
		if err := sleepContext(req.Context(), wait); err != nil {
			return nil, err
		}
		attemptReq, err = rewind(req)
		if err != nil {
			return nil, err
		}
	}
}

// delay is the wait before retry number attempt: the server's Retry-After when it
// sends one, otherwise an exponential backoff, capped at maxDelay and then spread
// by up to a quarter so parallel jobs do not retry in step.
func (rt *retry429Transport) delay(attempt int, retryAfter string, now time.Time) time.Duration {
	wait, ok := parseRetryAfter(retryAfter, now)
	if !ok {
		wait = rt.baseDelay << (attempt - 1)
	}
	wait = min(wait, rt.maxDelay)
	return wait + rand.N(wait/4+1)
}

// parseRetryAfter reads both forms of the Retry-After header: delay seconds and an
// HTTP date.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(date.Sub(now), 0), true
	}
	return 0, false
}

// canResend reports whether req can be sent again after wait: its body must be
// replayable, and the wait must end before the caller's deadline (the SDK client's
// timeout), so the caller sees the 429 instead of a timeout.
func canResend(req *http.Request, wait time.Duration) bool {
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return false
	}
	deadline, ok := req.Context().Deadline()
	return !ok || time.Now().Add(wait).Before(deadline)
}

func rewind(req *http.Request) (*http.Request, error) {
	next := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("replaying the %s request body for a retry: %w",
				req.Method, err)
		}
		next.Body = body
	}
	return next, nil
}

func sleepContext(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// newFirst429Server answers the first request to each method and path with 429 and
// Retry-After: 0, and later ones with next. It records every request body.
func newFirst429Server(
	t *testing.T, next http.HandlerFunc,
) (*httptest.Server, func() map[string][]string) {
	t.Helper()
	var (
		mu     sync.Mutex
		bodies = map[string][]string{}
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		request := r.Method + " " + r.URL.Path
		mu.Lock()
		bodies[request] = append(bodies[request], string(body))
		first := len(bodies[request]) == 1
		mu.Unlock()
		if first {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}))
	t.Cleanup(server.Close)
	return server, func() map[string][]string {
		mu.Lock()
		defer mu.Unlock()
		return bodies
	}
}

func TestRetry429TransportRetriesSDKRequests(t *testing.T) {
	server, bodies := newFirst429Server(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/api-key/scope":
			_, _ = io.WriteString(w, fakeScopeJSON)
		case "/v2/facts/proj/env/tenants":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"key":"tfacc-1-tenant","id":"tenant-id","name":"Tenant"}`)
		default:
			http.NotFound(w, r)
		}
	})
	client := permit.NewPermit(
		permitConfig.NewConfigBuilder("fake-key").WithApiUrl(server.URL).Build())

	tenant, err := client.Api.Tenants.Create(
		t.Context(), *models.NewTenantCreate("tfacc-1-tenant", "Tenant"))

	if err != nil {
		t.Fatalf("Tenants.Create through a 429 = %v, want the retry to succeed", err)
	}
	if tenant.Key != "tfacc-1-tenant" {
		t.Errorf("tenant key = %q, want %q", tenant.Key, "tfacc-1-tenant")
	}
	got := bodies()
	if n := len(got["GET /v2/api-key/scope"]); n != 2 {
		t.Errorf("scope requests = %d, want 2 (a 429, then its retry)", n)
	}
	creates := got["POST /v2/facts/proj/env/tenants"]
	if len(creates) != 2 {
		t.Fatalf("create requests = %d, want 2 (a 429, then its retry)", len(creates))
	}
	if !strings.Contains(creates[0], `"tfacc-1-tenant"`) || creates[1] != creates[0] {
		t.Errorf("create bodies = %q, want the same tenant body sent twice", creates)
	}
}

// streamBody hides its reader's type, so the transport treats it as a stream it
// cannot resend on its own, the way it treats any body that is not in memory.
type streamBody struct{ io.Reader }

func (streamBody) Close() error { return nil }

func TestRetry429TransportRetriesDefaultClientRequests(t *testing.T) {
	server, bodies := newFirst429Server(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	const body = `{"role":"viewer"}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, server.URL+"/roles",
		streamBody{strings.NewReader(body)})
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) { return streamBody{strings.NewReader(body)}, nil }

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		t.Fatalf("DELETE through a 429 = %v, want the retry to succeed", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	deletes := bodies()["DELETE /roles"]
	if len(deletes) != 2 || deletes[0] != body || deletes[1] != body {
		t.Errorf("DELETE bodies = %q, want the body sent twice", deletes)
	}
}

// TestRetry429TransportLogsPastPluginTesting checks that the transport TestMain
// installs still reports a retry after the standard logger is silenced, as
// plugin-testing silences it in every acceptance test when TF_LOG is unset.
func TestRetry429TransportLogsPastPluginTesting(t *testing.T) {
	installed, ok := http.DefaultTransport.(*retry429Transport)
	if !ok {
		t.Fatalf("http.DefaultTransport is %T, want the *retry429Transport from TestMain",
			http.DefaultTransport)
	}
	var logs strings.Builder
	installedOutput, stdOutput := installed.logger.Writer(), log.Writer()
	installed.logger.SetOutput(&logs)
	log.SetOutput(io.Discard)
	t.Cleanup(func() {
		log.SetOutput(stdOutput)
		installed.logger.SetOutput(installedOutput)
	})
	server, bodies := newFirst429Server(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	resp, err := http.Get(server.URL + "/v2/facts/proj/env/users/tfacc-1-user?token=hidden")

	if err != nil {
		t.Fatalf("GET through a 429 = %v, want the retry to succeed", err)
	}
	_ = resp.Body.Close()
	if n := len(bodies()["GET /v2/facts/proj/env/users/tfacc-1-user"]); n != 2 {
		t.Fatalf("requests = %d, want 2 (a 429, then its retry)", n)
	}
	got := logs.String()
	if !strings.Contains(got, "Permit API answered 429 to GET; retry 1 of 5") {
		t.Errorf("log = %q, want a line for the retry", got)
	}
	if strings.Contains(got, "tfacc-1-user") || strings.Contains(got, "hidden") {
		t.Errorf("log = %q, want the method only, not the URL", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestRetry429TransportGivesUp(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)
	quick := &retry429Transport{
		base:        server.Client().Transport,
		maxAttempts: 3,
		baseDelay:   time.Millisecond,
		maxDelay:    2 * time.Millisecond,
		logger:      log.New(io.Discard, "", 0),
	}

	t.Run("after maxAttempts", func(t *testing.T) {
		count.Store(0)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := quick.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip() error = %v, want the last 429 response", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusTooManyRequests || count.Load() != 3 {
			t.Errorf("got status %d after %d requests, want 429 after 3",
				resp.StatusCode, count.Load())
		}
	})

	t.Run("when the wait outlasts the deadline", func(t *testing.T) {
		count.Store(0)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		slow := *quick
		slow.maxDelay = time.Minute
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := slow.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip() error = %v, want the 429 response before the deadline", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusTooManyRequests || count.Load() != 1 {
			t.Errorf("got status %d after %d requests, want 429 after 1",
				resp.StatusCode, count.Load())
		}
	})

	t.Run("when the body cannot be replayed", func(t *testing.T) {
		count.Store(0)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL,
			io.NopCloser(strings.NewReader("once")))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := quick.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip() error = %v, want the 429 response", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusTooManyRequests || count.Load() != 1 {
			t.Errorf("got status %d after %d requests, want 429 after 1",
				resp.StatusCode, count.Load())
		}
	})

	t.Run("when replaying the body fails", func(t *testing.T) {
		count.Store(0)
		errReplay := errors.New("body already consumed")
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL,
			strings.NewReader("once"))
		if err != nil {
			t.Fatal(err)
		}
		req.GetBody = func() (io.ReadCloser, error) { return nil, errReplay }

		resp, err := quick.RoundTrip(req)

		if resp != nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, errReplay) || count.Load() != 1 {
			t.Errorf("RoundTrip() error = %v after %d requests, want %q after 1",
				err, count.Load(), errReplay)
		}
	})

	t.Run("when the request is cancelled during the wait", func(t *testing.T) {
		count.Store(0)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		waiting := *quick
		waiting.maxDelay = time.Minute
		waiting.base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			resp, err := quick.base.RoundTrip(req)
			time.AfterFunc(50*time.Millisecond, cancel)
			return resp, err
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)

		go func() {
			resp, err := waiting.RoundTrip(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
			done <- err
		}()

		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) || count.Load() != 1 {
				t.Errorf("RoundTrip() error = %v after %d requests, want %v after 1",
					err, count.Load(), context.Canceled)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("RoundTrip() still waiting 5s after the request was cancelled, " +
				"want it to stop waiting and return the cancellation")
		}
	})
}

func TestRetry429TransportDelay(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	rt := &retry429Transport{baseDelay: time.Second, maxDelay: 16 * time.Second}
	tests := []struct {
		name       string
		attempt    int
		retryAfter string
		want       time.Duration
	}{
		{name: "backoff starts at baseDelay", attempt: 1, want: time.Second},
		{name: "backoff doubles", attempt: 3, want: 4 * time.Second},
		{name: "backoff stops at maxDelay", attempt: 10, want: 16 * time.Second},
		{name: "Retry-After seconds", attempt: 4, retryAfter: "3", want: 3 * time.Second},
		{name: "Retry-After zero", attempt: 1, retryAfter: "0", want: 0},
		{name: "Retry-After over maxDelay", attempt: 1, retryAfter: "120", want: 16 * time.Second},
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
			seen := map[time.Duration]bool{}
			for range 50 {
				got := rt.delay(tt.attempt, tt.retryAfter, now)
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
}
