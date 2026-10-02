package mockpermit

import (
	"net/http"
	"strings"
	"testing"
)

// TestTenantPatchReplacesAttributesWhole checks that a tenant PATCH with attributes
// replaces them whole, that attributes {} clear them, and that a PATCH without them
// keeps them, as the API does. The fake stores and answers an integer beyond 2^53
// with the digits it was sent.
func TestTenantPatchReplacesAttributesWhole(t *testing.T) {
	m := New(t, Tenants)
	const acme = tenantsPath + "/acme"
	wantStoredAttributes := func(want string) {
		t.Helper()
		if err := m.CheckStoredJSON("tenants/acme", "attributes", want)(nil); err != nil {
			t.Error(err)
		}
	}
	send(t, m, http.MethodPost, tenantsPath, `{"key": "acme", "name": "Acme",
		"attributes": {"tier": "gold", "account": 9007199254740993}}`, http.StatusOK)

	wantStoredAttributes(`{"account":9007199254740993,"tier":"gold"}`)
	if _, body := call(t, m, http.MethodGet, acme, bearer, ""); !strings.Contains(body,
		`"account":9007199254740993`) {
		t.Errorf("GET %s = %s, want the account 9007199254740993 as sent", acme, body)
	}

	send(t, m, http.MethodPatch, acme, `{"name": "Acme Inc"}`, http.StatusOK)
	wantStoredAttributes(`{"account":9007199254740993,"tier":"gold"}`)

	send(t, m, http.MethodPatch, acme, `{"attributes": {"seats": 25}}`, http.StatusOK)
	wantStoredAttributes(`{"seats":25}`)

	wantFields(t, send(t, m, http.MethodPatch, acme, `{"attributes": {}}`, http.StatusOK),
		`{"name": "Acme Inc", "attributes": {}}`)
	wantStoredAttributes(`{}`)
}

// TestCheckStoredJSON checks that CheckStoredJSON fails on a stored value other than
// want, on a number rounded on the way, and on an object or field the fake does not
// hold.
func TestCheckStoredJSON(t *testing.T) {
	m := New(t, Tenants)
	send(t, m, http.MethodPost, tenantsPath, `{"key": "acme", "name": "Acme",
		"attributes": {"account": 9007199254740993}}`, http.StatusOK)

	tests := []struct {
		name, stored, field, want, wantError string
	}{
		{name: "same value", stored: "tenants/acme", field: "attributes",
			want: `{"account":9007199254740993}`},
		{name: "rounded number", stored: "tenants/acme", field: "attributes",
			want: `{"account":9007199254740992}`, wantError: "want"},
		{name: "same value, other spacing", stored: "tenants/acme", field: "attributes",
			want: `{"account": 9007199254740993}`, wantError: "want"},
		{
			name: "missing object", stored: "tenants/beta", field: "attributes", want: `{}`,
			wantError: `holds no tenants/beta with the field attributes; ` +
				`it holds ["tenants/acme"]`,
		},
		{name: "missing field", stored: "tenants/acme", field: "tags", want: `{}`,
			wantError: "holds no tenants/acme with the field tags"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.CheckStoredJSON(tt.stored, tt.field, tt.want)(nil)
			if tt.wantError == "" {
				if err != nil {
					t.Errorf("CheckStoredJSON() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("CheckStoredJSON() = %v, want an error containing %q", err,
					tt.wantError)
			}
		})
	}
}
