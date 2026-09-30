package mockpermit

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// TestDeleteStored checks that DeleteStored removes exactly the objects it names,
// so that the API then answers 404 for them, and fails the test on a name that is
// not stored or on no names at all.
func TestDeleteStored(t *testing.T) {
	rec := &errorRecorder{TB: t}
	m := New(rec, Tenants, Users)
	m.AddUser(`{"key": "alice"}`)
	send(t, m, http.MethodPost, tenantsPath, `{"key": "acme", "name": "Acme"}`, http.StatusOK)
	send(t, m, http.MethodPost, tenantsPath, `{"key": "beta", "name": "Beta"}`, http.StatusOK)

	m.DeleteStored("tenants/acme", "users/alice")

	if got := rec.take(); len(got) != 0 {
		t.Errorf("DeleteStored() of stored objects failed the test: %q", got)
	}
	wantStoredKeys(t, m, "tenants/beta")
	send(t, m, http.MethodGet, tenantsPath+"/acme", "", http.StatusNotFound)

	for _, tt := range []struct {
		name    string
		stored  []string
		wantErr string
	}{
		{"deleted already", []string{"tenants/acme"}, "tenants/acme is not stored"},
		{"unknown collection", []string{"groups/acme"}, "groups/acme is not stored"},
		{"no slash", []string{"tenants"}, "tenants is not stored"},
		{"nothing", nil, "given nothing to delete"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m.DeleteStored(tt.stored...)

			got := rec.take()
			if len(got) != 1 || !strings.Contains(got[0], tt.wantErr) {
				t.Errorf("test errors = %q, want one containing %q", got, tt.wantErr)
			}
			if keys := m.StoredKeys(); !slices.Equal(keys, []string{"tenants/beta"}) {
				t.Errorf("StoredKeys() = %q, want only tenants/beta left", keys)
			}
		})
	}
}

// TestSetStored checks that SetStored changes the fields it is given on the object
// it names, so that the API then answers with them, and fails the test on an
// object that is not stored or on no fields at all.
func TestSetStored(t *testing.T) {
	rec := &errorRecorder{TB: t}
	m := New(rec, Tenants)
	send(t, m, http.MethodPost, tenantsPath, `{"key": "acme", "name": "Acme"}`, http.StatusOK)

	m.SetStored("tenants/acme", map[string]any{"description": "Set in the UI"})

	if got := rec.take(); len(got) != 0 {
		t.Errorf("SetStored() of a stored object failed the test: %q", got)
	}
	want := `{"key": "acme", "name": "Acme", "description": "Set in the UI"}`
	wantFields(t, send(t, m, http.MethodGet, tenantsPath+"/acme", "", http.StatusOK), want)

	for _, tt := range []struct {
		name    string
		stored  string
		fields  map[string]any
		wantErr string
	}{
		{"not stored", "tenants/beta", map[string]any{"name": "B"}, "tenants/beta is not stored"},
		{"no fields", "tenants/acme", nil, "given no fields to set on tenants/acme"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m.SetStored(tt.stored, tt.fields)

			got := rec.take()
			if len(got) != 1 || !strings.Contains(got[0], tt.wantErr) {
				t.Errorf("test errors = %q, want one containing %q", got, tt.wantErr)
			}
			wantStoredKeys(t, m, "tenants/acme")
			wantFields(t, send(t, m, http.MethodGet, tenantsPath+"/acme", "", http.StatusOK),
				want)
		})
	}
}
