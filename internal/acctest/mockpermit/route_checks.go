package mockpermit

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
)

// httpSuffix ends the operation of a route for a request the provider builds
// itself with net/http.
const httpSuffix = " (HTTP)"

// CheckHTTPRoutes checks the routes in set for requests the provider builds itself
// with net/http. Only the provider package that builds such a request can call the
// function that sends it, so the package's tests call CheckHTTPRoutes, and the
// coverage tests of this package check that they do. calls maps the operation of
// each such route, "<pkg>.<function> (HTTP)", to a call of that function that sends
// its request to the base URL it is given. For each route, CheckHTTPRoutes starts a
// fake that serves only that route, runs the call, and fails the test unless the
// call sent exactly one request, the route received it, and the fake did not fail
// it. The call's arguments are placeholders: the request only has to reach the
// route, not succeed. It also fails the test when set has no such route, or when
// calls and the routes do not name the same operations.
func CheckHTTPRoutes(t *testing.T, set Routes,
	calls map[string]func(ctx context.Context, url string),
) {
	t.Helper()
	checked := map[string]bool{}
	for _, rt := range set {
		if !strings.HasSuffix(rt.operation, httpSuffix) {
			continue
		}
		checked[rt.operation] = true
		call, ok := calls[rt.operation]
		if !ok {
			t.Errorf("mockpermit: calls has no call for %s, the operation of the route %q",
				rt.operation, rt.pattern)
			continue
		}
		t.Run(rt.operation, func(t *testing.T) {
			checkRouteCall(t, rt, func(url string) { call(t.Context(), url) })
		})
	}
	if len(checked) == 0 {
		t.Errorf("mockpermit: CheckHTTPRoutes was given a route set with no route for a " +
			"request the provider builds itself")
	}
	for _, operation := range slices.Sorted(maps.Keys(calls)) {
		if !checked[operation] {
			t.Errorf("mockpermit: calls has a call for %s, which is not the operation of an "+
				"HTTP route in the set", operation)
		}
	}
}

// checkRouteCall starts a fake that serves only rt, runs call with the fake's URL,
// and fails the test unless call sent exactly one request besides the API key
// scope, rt received it, and the fake did not fail it.
func checkRouteCall(t *testing.T, rt route, call func(url string)) {
	t.Helper()
	rec := &errorRecorder{TB: t}
	m := New(rec, Routes{rt})

	call(m.URL)

	m.mu.Lock()
	var sent []string
	for _, request := range m.requests {
		if request.Path != scopePath {
			sent = append(sent, request.Method+" "+request.Path)
		}
	}
	hits := m.hits[rt.pattern]
	m.mu.Unlock()
	if len(sent) != 1 || hits != 1 {
		t.Errorf("%s sent %q; want one request, to %q", rt.operation, sent, rt.pattern)
	}
	if errs := rec.take(); len(errs) > 0 {
		t.Errorf("%s: the mock failed the request: %q", rt.operation, errs)
	}
}

// errorRecorder stands in for a test's testing.TB and records Errorf calls instead
// of failing the test, so a check can tell a request the fake failed from one it
// served, and a test can check that the mock fails the test it serves.
type errorRecorder struct {
	testing.TB
	mu         sync.Mutex
	testErrors []string
}

func (r *errorRecorder) Helper() {}

func (r *errorRecorder) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.testErrors = append(r.testErrors, fmt.Sprintf(format, args...))
}

func (r *errorRecorder) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	testErrors := r.testErrors
	r.testErrors = nil
	return testErrors
}
