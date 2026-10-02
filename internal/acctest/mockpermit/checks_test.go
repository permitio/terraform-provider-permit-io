package mockpermit

import (
	"net/http"
	"strings"
	"testing"
)

func TestCheckRequests(t *testing.T) {
	m := New(t, Tenants)
	const (
		acme = `{"key": "acme", "name": "Acme"}`
		beta = `{"key": "beta", "name": "Beta"}`
	)
	send(t, m, http.MethodPost, tenantsPath, acme, http.StatusOK)
	send(t, m, http.MethodPost, tenantsPath, beta, http.StatusOK)

	tests := []struct {
		name      string
		method    string
		want      []string
		wantError string
	}{
		{name: "same bodies, same order", method: http.MethodPost, want: []string{acme, beta}},
		{name: "same bodies, other order", method: http.MethodPost, want: []string{beta, acme}},
		{name: "no requests, none wanted", method: http.MethodPatch},
		{
			name: "fewer bodies than requests", method: http.MethodPost, want: []string{acme},
			wantError: "got 2 requests, want 1",
		},
		{
			name: "requests where none are wanted", method: http.MethodPost,
			wantError: "got 2 requests, want 0",
		},
		{
			name: "one body differs", method: http.MethodPost,
			want:      []string{acme, `{"key": "beta", "name": "Other"}`},
			wantError: `no request has the body {"key": "beta", "name": "Other"}`,
		},
		{
			name: "the same body twice", method: http.MethodPost, want: []string{acme, acme},
			wantError: "no request has the body " + acme,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.CheckRequests(tt.method, tenantsPath, tt.want...)(nil)
			if tt.wantError == "" {
				if err != nil {
					t.Errorf("CheckRequests() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("CheckRequests() = %v, want an error containing %q", err, tt.wantError)
			}
		})
	}
}

func TestCheckEmpty(t *testing.T) {
	m := New(t, Tenants)
	if err := m.CheckEmpty(nil); err != nil {
		t.Errorf("CheckEmpty() on a new mock = %v, want nil", err)
	}

	send(t, m, http.MethodPost, tenantsPath, `{"key": "acme", "name": "Acme"}`, http.StatusOK)

	if err := m.CheckEmpty(nil); err == nil || !strings.Contains(err.Error(), "tenants/acme") {
		t.Errorf("CheckEmpty() with a tenant stored = %v, want an error naming tenants/acme", err)
	}

	send(t, m, http.MethodDelete, tenantsPath+"/acme", "", http.StatusNoContent)

	if err := m.CheckEmpty(nil); err != nil {
		t.Errorf("CheckEmpty() after the delete = %v, want nil", err)
	}
}

func TestCheckStored(t *testing.T) {
	m := New(t, Tenants, Users)
	m.AddUser(`{"key": "alice"}`)
	send(t, m, http.MethodPost, tenantsPath, `{"key": "acme", "name": "Acme"}`, http.StatusOK)

	if err := m.CheckStored("users/alice")(nil); err == nil ||
		!strings.Contains(err.Error(), "tenants/acme") {
		t.Errorf("CheckStored(users/alice) with a tenant stored too = %v, want an error "+
			"naming tenants/acme", err)
	}
	if err := m.CheckStored("users/alice", "users/bob")(nil); err == nil {
		t.Errorf("CheckStored(users/alice, users/bob) without bob = nil, want an error")
	}
	if err := m.CheckStored("users/alice", "tenants/acme")(nil); err != nil {
		t.Errorf("CheckStored() in any order = %v, want nil", err)
	}

	send(t, m, http.MethodDelete, tenantsPath+"/acme", "", http.StatusNoContent)

	if err := m.CheckStored("users/alice")(nil); err != nil {
		t.Errorf("CheckStored(users/alice) after the delete = %v, want nil", err)
	}
}
