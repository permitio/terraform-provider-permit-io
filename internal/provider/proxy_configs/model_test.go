package proxy_configs

import (
	"testing"

	"github.com/permitio/permit-golang/pkg/models"
)

// TestFromProxyConfigReadDefaultsTheAuthMechanism reads a proxy config that the
// API returns without auth_mechanism, and checks that it gets the API's default,
// Bearer, instead of crashing the provider.
func TestFromProxyConfigReadDefaultsTheAuthMechanism(t *testing.T) {
	var model proxyConfigModel

	model.fromProxyConfigRead(&models.ProxyConfigRead{Key: "billing", Secret: "token"})

	if got := model.AuthMechanism.ValueString(); got != "Bearer" {
		t.Errorf("auth_mechanism = %q, want %q", got, "Bearer")
	}
	if got := model.AuthSecret.Bearer.ValueString(); got != "token" {
		t.Errorf("auth_secret.bearer = %q, want %q", got, "token")
	}
}
