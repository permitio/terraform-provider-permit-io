// Package mockpermit is a stateful fake of the Permit API for offline provider
// tests. It serves only the routes a test asks for, keeps the objects the provider
// creates in memory, and records every request so a test can assert what went over
// the wire. A request to a route the fake does not serve, or with an empty or dot
// path segment, fails the test. So does a request whose effect on the real API is
// unconfirmed, such as a resource PATCH that leaves out an action: the fake does
// not guess.
package mockpermit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

// The API key the provider sends and the scope the fake reports for it. The SDK
// asks for the scope before its first call and puts the project and environment
// IDs into every later path.
const (
	APIKey         = "mockpermit-test-key"
	OrganizationID = "mock-org"
	ProjectID      = "mock-project"
	EnvironmentID  = "mock-env"
)

// timestamp is the creation, update and last-action time of every object, so
// responses do not change between runs.
const timestamp = "2026-01-01T00:00:00Z"

// scopePath is where the SDK asks for the API key's scope.
const scopePath = "/v2/api-key/scope"

// Request is one HTTP request the fake received.
type Request struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

// CheckJSONBody returns an error unless the request body is the same JSON value as
// want. Object key order and whitespace do not matter; missing, extra or changed
// fields do.
func (r Request) CheckJSONBody(want string) error {
	var gotValue, wantValue any
	if err := json.Unmarshal(r.Body, &gotValue); err != nil {
		return fmt.Errorf("%s %s: request body %q is not JSON: %w", r.Method, r.Path, r.Body, err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		return fmt.Errorf("want %q is not JSON: %w", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		return fmt.Errorf("%s %s: request body = %s, want %s",
			r.Method, r.Path, compactJSON(gotValue), compactJSON(wantValue))
	}
	return nil
}

// compactJSON writes a decoded JSON value on one line with sorted object keys, so
// two bodies in an error message line up.
func compactJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%#v (not encodable: %v)", value, err)
	}
	return string(encoded)
}

// route is one API operation the fake serves. The pattern is an http.ServeMux
// pattern with a method, written with the path parameter names the SDK uses. The
// operation is the permit-golang call that sends the request, written as
// Group.Method on the client's Api, such as "Resources.Create". The coverage tests
// check each operation against the SDK and against the provider's call sites.
type route struct {
	pattern   string
	operation string
	handle    func(s *Server, w http.ResponseWriter, r *http.Request)
}

// Routes is a set of operations to pass to New, one set per Permit object type.
type Routes []route

// Server is a running fake Permit API.
type Server struct {
	// URL is the base URL of the fake, without a trailing slash.
	URL string

	t        testing.TB
	mu       sync.Mutex
	requests []Request
	hits     map[string]int
	// objects maps a collection name to its objects by key.
	objects map[string]map[string]map[string]any
	lastID  int
}

// New starts a fake Permit API that serves the API key scope and the given route
// sets, and points the provider at it by setting PERMITIO_API_URL and
// PERMITIO_API_KEY for the rest of the test. Because it sets environment
// variables, a test that calls New cannot run in parallel. The server stops when
// the test ends.
func New(t testing.TB, sets ...Routes) *Server {
	t.Helper()
	s := &Server{
		t:       t,
		hits:    map[string]int{},
		objects: map[string]map[string]map[string]any{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.unknownRoute)
	// The SDK asks for the scope on its own, so this route names no operation.
	s.handle(mux, route{pattern: "GET " + scopePath, handle: (*Server).getAPIKeyScope})
	for _, set := range sets {
		for _, rt := range set {
			s.handle(mux, rt)
		}
	}
	srv := httptest.NewServer(s.recordRequests(mux))
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	t.Setenv("PERMITIO_API_URL", srv.URL)
	t.Setenv("PERMITIO_API_KEY", APIKey)
	return s
}

// Requests returns the requests received so far with this method and path, oldest
// first.
func (s *Server) Requests(method, urlPath string) []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	var matching []Request
	for _, r := range s.requests {
		if r.Method == method && r.Path == urlPath {
			matching = append(matching, r)
		}
	}
	return matching
}

// AssertAllRoutesHit fails the test for every route the fake serves that received
// no request, so a test cannot pass without exercising the operations it set up.
func (s *Server) AssertAllRoutesHit() {
	s.t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var unhit []string
	for pattern, hits := range s.hits {
		if hits == 0 {
			unhit = append(unhit, pattern)
		}
	}
	if len(unhit) > 0 {
		slices.Sort(unhit)
		s.t.Errorf("mockpermit: routes never called: %q", unhit)
	}
}

// AssertRoutesHit fails the test for every route in sets that received no request.
// A test that also serves the route sets of objects it only depends on, such as
// the resource a resource set is on, names the sets it exercises here, so it does
// not have to change the other objects just to call their routes. It fails the
// test when sets has no routes or names a route the fake does not serve.
func (s *Server) AssertRoutesHit(sets ...Routes) {
	s.t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var unhit, unserved []string
	checked := 0
	for _, set := range sets {
		for _, rt := range set {
			checked++
			hits, served := s.hits[rt.pattern]
			switch {
			case !served:
				unserved = append(unserved, rt.pattern)
			case hits == 0:
				unhit = append(unhit, rt.pattern)
			}
		}
	}
	if checked == 0 {
		s.t.Errorf("mockpermit: AssertRoutesHit was given no routes to check")
	}
	if len(unserved) > 0 {
		slices.Sort(unserved)
		s.t.Errorf("mockpermit: AssertRoutesHit names routes the mock does not serve: %q; "+
			"pass their route sets to New", unserved)
	}
	if len(unhit) > 0 {
		slices.Sort(unhit)
		s.t.Errorf("mockpermit: routes never called: %q", unhit)
	}
}

// StoredKeys returns "collection/key" for every object the fake holds, sorted. It
// is empty after a destroy that removed everything the test created.
func (s *Server) StoredKeys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for collection, objects := range s.objects {
		for key := range objects {
			keys = append(keys, collection+"/"+key)
		}
	}
	slices.Sort(keys)
	return keys
}

func (s *Server) handle(mux *http.ServeMux, rt route) {
	s.hits[rt.pattern] = 0
	mux.HandleFunc(rt.pattern, func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[rt.pattern]++
		s.mu.Unlock()
		if !s.inScope(w, r) {
			return
		}
		rt.handle(s, w, r)
	})
}

// recordRequests records every request, then checks its API key and that its path
// is in canonical form before passing it on to the route table.
func (s *Server) recordRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.t.Errorf("mockpermit: %s %s: reading the request body: %v", r.Method, r.URL.Path, err)
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "unreadable request body")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		s.mu.Lock()
		s.requests = append(s.requests, Request{
			Method:   r.Method,
			Path:     r.URL.Path,
			RawQuery: r.URL.RawQuery,
			Header:   r.Header.Clone(),
			Body:     body,
		})
		s.mu.Unlock()

		if got, want := r.Header.Get("Authorization"), "Bearer "+APIKey; got != want {
			s.t.Errorf("mockpermit: %s %s: Authorization header = %q, want %q",
				r.Method, r.URL.Path, got, want)
			s.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid API key")
			return
		}
		if escaped := r.URL.EscapedPath(); escaped != cleanPath(escaped) {
			s.t.Errorf("mockpermit: %s %s: the path has an empty or dot segment, usually an "+
				"empty key or ID in the path", r.Method, escaped)
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "path is not canonical")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// cleanPath returns p the way http.ServeMux cleans a path before routing it. The
// mux answers a path that cleaning changes with a redirect to the cleaned path, so
// without a check first, .../tenants//users would be served as .../tenants/users.
func cleanPath(p string) string {
	cleaned := path.Clean(p)
	if strings.HasSuffix(p, "/") && cleaned != "/" {
		cleaned += "/"
	}
	return cleaned
}

func (s *Server) unknownRoute(w http.ResponseWriter, r *http.Request) {
	s.t.Errorf("mockpermit: no route for %s %s; add it to the mock's route table if the "+
		"provider should call it", r.Method, r.URL.Path)
	s.writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "no route in mockpermit")
}

// inScope fails the test when a path names a project or environment other than the
// one the API key scope reported.
func (s *Server) inScope(w http.ResponseWriter, r *http.Request) bool {
	for name, want := range map[string]string{"proj_id": ProjectID, "env_id": EnvironmentID} {
		if got := r.PathValue(name); got != "" && got != want {
			s.t.Errorf("mockpermit: %s %s: %s = %q, want %q from the API key scope",
				r.Method, r.URL.Path, name, got, want)
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", name+" not found")
			return false
		}
	}
	return true
}

func (s *Server) getAPIKeyScope(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"organization_id": OrganizationID,
		"project_id":      ProjectID,
		"environment_id":  EnvironmentID,
	})
}

// decodeObject reads the request body as a JSON object. On failure it fails the
// test, answers 422 like the API does, and returns false.
func (s *Server) decodeObject(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	var object map[string]any
	err := json.NewDecoder(r.Body).Decode(&object)
	if err == nil && object == nil {
		err = errors.New("the body is null")
	}
	if err != nil {
		s.t.Errorf("mockpermit: %s %s: request body is not a JSON object: %v",
			r.Method, r.URL.Path, err)
		s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
			"request body is not a JSON object")
		return nil, false
	}
	return object, true
}

// knownFieldsOnly answers 422 and returns false when the request body has a field
// that is not in known. The API's request models allow no other fields, and it
// rejects a body with one the same way.
func (s *Server) knownFieldsOnly(w http.ResponseWriter, body map[string]any,
	known ...string,
) bool {
	var unknown []string
	for field := range body {
		if !slices.Contains(known, field) {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) == 0 {
		return true
	}
	slices.Sort(unknown)
	s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
		fmt.Sprintf("unknown fields %q", unknown))
	return false
}

// objectIfGiven answers 422 and returns false when the request body has the field
// and it is not a JSON object.
func (s *Server) objectIfGiven(w http.ResponseWriter, body map[string]any, field string) bool {
	if _, given := body[field]; !given {
		return true
	}
	if _, ok := body[field].(map[string]any); ok {
		return true
	}
	s.writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY",
		field+" is not an object")
	return false
}

// refuseUnconfirmed fails the test and answers 501 for a request whose effect on
// the API is unconfirmed, so that the fake does not guess it. what says what the
// request does.
func (s *Server) refuseUnconfirmed(w http.ResponseWriter, r *http.Request, what string) {
	s.t.Errorf("mockpermit: %s %s: %s, and what the API does then is unconfirmed, so the "+
		"fake does not model it", r.Method, r.URL.Path, what)
	s.writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "not modelled in mockpermit")
}

// collection returns the objects stored under name by key, creating the collection
// on first use. The caller holds s.mu.
func (s *Server) collection(name string) map[string]map[string]any {
	objects, ok := s.objects[name]
	if !ok {
		objects = map[string]map[string]any{}
		s.objects[name] = objects
	}
	return objects
}

// ObjectID returns the ID, in UUID form, that the fake gives the nth object it
// creates. Nested objects, such as a resource's actions, get IDs too.
func ObjectID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}

// newID returns a fresh object ID. The caller holds s.mu.
func (s *Server) newID() string {
	s.lastID++
	return ObjectID(s.lastID)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		s.t.Errorf("mockpermit: writing a %d response: %v", status, err)
	}
}

// writeError answers with a body shaped like the Permit API's error body.
func (s *Server) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, map[string]string{
		"error_code": code,
		"title":      http.StatusText(status),
		"message":    message,
	})
}
