package mockpermit

import (
	"net/http"
	"strings"
	"testing"
)

// TestFailRequests checks that FailRequests serves the matching requests it is
// told to let through, answers the rest with the status and the API's 404 body,
// leaves other requests alone, and stops when told to. Stopping a fault that
// answered no request fails the test.
func TestFailRequests(t *testing.T) {
	rec := &errorRecorder{TB: t}
	m := New(rec, Tenants)
	send(t, m, http.MethodPost, tenantsPath, `{"key": "acme", "name": "Acme"}`, http.StatusOK)
	acme := tenantsPath + "/acme"

	stop := m.FailRequests(http.MethodGet, acme, http.StatusInternalServerError, 1)
	send(t, m, http.MethodGet, acme, "", http.StatusOK)
	for range 2 {
		got := send(t, m, http.MethodGet, acme, "", http.StatusInternalServerError)
		wantFields(t, got, `{"error_code": "NOT_FOUND", "message": "object not found"}`)
	}
	send(t, m, http.MethodGet, tenantsPath+"/beta", "", http.StatusNotFound)
	send(t, m, http.MethodPatch, acme, `{"name": "Acme Inc"}`, http.StatusOK)
	stop()
	send(t, m, http.MethodGet, acme, "", http.StatusOK)

	if got := rec.take(); len(got) != 0 {
		t.Errorf("FailRequests() of requests that were sent failed the test: %q", got)
	}
	if got := len(m.Requests(http.MethodGet, acme)); got != 4 {
		t.Errorf("the mock recorded %d GET %s requests, want 4", got, acme)
	}

	m.FailRequests(http.MethodDelete, acme, http.StatusForbidden, 0)()
	if got := rec.take(); len(got) != 1 || !strings.Contains(got[0], "no request was answered") {
		t.Errorf("stopping a fault that answered nothing: test errors = %q, want one", got)
	}
	wantStoredKeys(t, m, "tenants/acme")
}
