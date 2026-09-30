// Package httpclient builds the HTTP client the provider sends Permit API requests
// with. The client retries requests the API rate-limited or could not serve for a
// moment, and names the provider in the User-Agent header.
package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// The retry limits of the client New returns. The client's Timeout bounds all the
// attempts of one request together, so with the provider's default timeout of 10
// seconds a request stops retrying after about four attempts.
const (
	maxAttempts = 6
	baseDelay   = time.Second
	maxDelay    = 16 * time.Second
)

// UserAgent is the User-Agent header the provider sends: the provider name and
// its version.
func UserAgent(version string) string {
	return "terraform-provider-permitio/" + version
}

// New returns an HTTP client for Permit API requests that sends them through a
// Transport over http.DefaultTransport, with the User-Agent of this provider
// version. Its Timeout is zero; the caller sets it.
func New(version string) *http.Client {
	return &http.Client{Transport: &Transport{
		UserAgent:   UserAgent(version),
		MaxAttempts: maxAttempts,
		BaseDelay:   baseDelay,
		MaxDelay:    maxDelay,
	}}
}

// Transport is an http.RoundTripper that sets the User-Agent header and sends a
// request again when an attempt fails in a way that a later attempt may not:
//   - 429 Too Many Requests, for every method;
//   - 502, 503 and 504, for methods that are safe to repeat, never for POST or
//     PATCH, since the API may have made the change before it failed;
//   - a connection that could not be opened, for every method, since the API
//     never saw the request;
//   - a connection that broke before the response, for methods that are safe to
//     repeat.
//
// It waits between attempts for the response's Retry-After, or else for an
// exponential backoff, capped at MaxDelay and spread by up to a quarter so that
// parallel requests do not retry in step. It returns the last failed response
// instead of waiting past the request context's deadline, which includes the
// http.Client's Timeout, and it does not retry a request whose body it cannot
// replay.
type Transport struct {
	// Base sends each attempt. It is http.DefaultTransport when nil.
	Base http.RoundTripper
	// UserAgent replaces the User-Agent header of every request.
	UserAgent string
	// MaxAttempts is how many times a request is sent at most, the first time
	// included.
	MaxAttempts int
	// BaseDelay is the wait before the first retry when the response has no
	// Retry-After; it doubles for every retry after that.
	BaseDelay time.Duration
	// MaxDelay caps every wait, Retry-After included.
	MaxDelay time.Duration
}

// RoundTrip sends req, and sends it again as the Transport's rules allow.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	for attempt := 1; ; attempt++ {
		attemptReq, err := t.attemptRequest(req, attempt)
		if err != nil {
			return nil, err
		}
		resp, err := base.RoundTrip(attemptReq)
		reason := retryReason(req, resp, err)
		if reason == "" || attempt >= t.MaxAttempts {
			return resp, err
		}
		wait := t.delay(attempt, resp, time.Now())
		if !canResend(req, wait) {
			return resp, err
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		tflog.Info(req.Context(), "Retrying a Permit API request", map[string]any{
			"method":  req.Method,
			"reason":  reason,
			"attempt": attempt + 1,
			"wait":    wait.String(),
		})
		if err := sleep(req.Context(), wait); err != nil {
			return nil, err
		}
	}
}

// attemptRequest returns the request to send for attempt: a copy of req with the
// User-Agent set, and from the second attempt on a fresh copy of its body. A
// RoundTripper must not change the request it is given.
func (t *Transport) attemptRequest(req *http.Request, attempt int) (*http.Request, error) {
	next := req.Clone(req.Context())
	next.Header.Set("User-Agent", t.UserAgent)
	if attempt > 1 && req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("replaying the %s request body for a retry: %w",
				req.Method, err)
		}
		next.Body = body
	}
	return next, nil
}

// retryReason says why the outcome of an attempt is worth another one, or returns
// "" when it is not.
func retryReason(req *http.Request, resp *http.Response, err error) string {
	if req.Context().Err() != nil {
		return ""
	}
	if err != nil {
		if connectionError(req.Method, err) {
			return err.Error()
		}
		return ""
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return resp.Status
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		if safeToRepeat(req.Method) {
			return resp.Status
		}
	}
	return ""
}

// connectionError reports whether err means that the connection failed: it could
// not be opened, which is worth retrying for every method because the API never
// saw the request, or, for a method that is safe to repeat, it broke before the
// response arrived. A host name that does not resolve is not worth retrying.
func connectionError(method string, err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	return safeToRepeat(method) && (errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF))
}

// safeToRepeat reports whether sending a request with method twice has the same
// effect on the API as sending it once.
func safeToRepeat(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut,
		http.MethodDelete:
		return true
	}
	return false
}

// delay is the wait before retry number attempt: the response's Retry-After when
// it has one, otherwise BaseDelay doubled for every earlier retry. The wait is
// capped at MaxDelay and then spread by up to a quarter.
func (t *Transport) delay(attempt int, resp *http.Response, now time.Time) time.Duration {
	var wait time.Duration
	retryAfter := false
	if resp != nil {
		wait, retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"), now)
	}
	if !retryAfter {
		wait = t.BaseDelay
		for range attempt - 1 {
			if wait >= t.MaxDelay {
				break
			}
			wait *= 2
		}
	}
	wait = min(wait, t.MaxDelay)
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
// replayable, and the wait must end before the request's deadline, so that the
// caller gets the failed response instead of a timeout.
func canResend(req *http.Request, wait time.Duration) bool {
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return false
	}
	deadline, ok := req.Context().Deadline()
	return !ok || time.Now().Add(wait).Before(deadline)
}

func sleep(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
