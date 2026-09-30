package mockpermit

import (
	"net/http"
	"slices"
)

// fault is a set of requests that FailRequests answers with an error.
type fault struct {
	method string
	path   string
	status int
	// after is how many matching requests are served before the fault answers.
	after  int
	seen   int
	failed int
}

// FailRequests makes the fake answer requests with this method and path with
// status instead of serving them, until the returned function is called. When
// after is n, the first n matching requests are served as usual, so that a test
// can fail the read of an object that shares its path with an object read before
// it. Each answer has the body of the API's 404, whose error code and message say
// "not found", so a provider that decides from the body or the message instead of
// the status is caught. The returned function fails the test when the fault
// answered no request, so a test cannot pass without reaching it.
func (s *Server) FailRequests(method, urlPath string, status, after int) (stop func()) {
	s.t.Helper()
	f := &fault{method: method, path: urlPath, status: status, after: after}
	s.mu.Lock()
	s.faults = append(s.faults, f)
	s.mu.Unlock()
	return func() {
		s.t.Helper()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.faults = slices.DeleteFunc(s.faults, func(other *fault) bool { return other == f })
		if f.failed == 0 {
			s.t.Errorf("mockpermit: FailRequests(%s %s, %d): no request was answered with "+
				"the error; %d matching requests were served", method, urlPath, status, f.seen)
		}
	}
}

// failRequest answers r with the error of the first fault that matches it and
// returns true, or returns false when no fault answers it.
func (s *Server) failRequest(w http.ResponseWriter, r *http.Request) bool {
	s.mu.Lock()
	status := 0
	for _, f := range s.faults {
		if f.method != r.Method || f.path != r.URL.Path {
			continue
		}
		f.seen++
		if f.seen > f.after {
			f.failed++
			status = f.status
		}
		break
	}
	s.mu.Unlock()
	if status == 0 {
		return false
	}
	s.writeError(w, status, "NOT_FOUND", "object not found")
	return true
}
