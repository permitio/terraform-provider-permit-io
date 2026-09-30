// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	permitConfig "github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/httpclient"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.

var permitProvider = New("test")()

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"permitio": providerserver.NewProtocol6WithError(permitProvider),
}

const (
	providerConfig = `
		provider "permitio" {
		}
		`
)

// testAccKeyPrefix starts every key an acceptance test creates. The random suffix
// from acctest.RandomWithPrefix keeps runs apart; the prefix makes objects left
// behind by cancelled runs easy to find. User attribute keys start with "tfacc_"
// instead of "tfacc-" (see testAccUserAttributeKey), so a cleanup that looks for
// leftovers must match both.
const testAccKeyPrefix = "tfacc"

// testAccPreCheck stops an acceptance test before it touches Terraform when the
// API key is missing, instead of failing later with a less obvious error.
func testAccPreCheck(tb testing.TB) {
	tb.Helper()
	if os.Getenv("PERMITIO_API_KEY") == "" {
		tb.Fatal("PERMITIO_API_KEY must be set to run acceptance tests")
	}
}

// testAccPermitClient builds an SDK client that tests use to check or change Permit
// state outside Terraform.
func testAccPermitClient() (*permit.Client, error) {
	cfg, err := testAccPermitConfig()
	if err != nil {
		return nil, err
	}
	return permit.NewPermit(cfg), nil
}

// testAccPermitConfig reads PERMITIO_API_KEY, PERMITIO_API_URL and PERMITIO_TIMEOUT
// the way the provider does, and builds the provider's HTTP client, so checks made
// outside Terraform get the same API, retries and time to wait out a 429 as the
// provider under test.
func testAccPermitConfig() (permitConfig.PermitConfig, error) {
	apiURL := os.Getenv("PERMITIO_API_URL")
	if apiURL == "" {
		apiURL = DefaultApiUrl
	}
	timeout := DefaultTimeout
	if value, ok := os.LookupEnv("PERMITIO_TIMEOUT"); ok {
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return permitConfig.PermitConfig{}, fmt.Errorf(
				"PERMITIO_TIMEOUT must be a whole number of seconds, got %q: %w", value, err)
		}
		timeout = time.Duration(seconds) * time.Second
	}
	return newClientConfig(os.Getenv("PERMITIO_API_KEY"), apiURL, timeout, "test").Build(), nil
}

// fatalRecorder stands in for a test's testing.TB and records a call to Fatal
// instead of stopping the test.
type fatalRecorder struct {
	testing.TB
	fatal string
}

func (r *fatalRecorder) Fatal(args ...any) { r.fatal = fmt.Sprint(args...) }

func TestPreCheckRequiresAPIKey(t *testing.T) {
	t.Setenv("PERMITIO_API_KEY", "")
	missing := &fatalRecorder{TB: t}
	testAccPreCheck(missing)
	if !strings.Contains(missing.fatal, "PERMITIO_API_KEY must be set") {
		t.Errorf("testAccPreCheck with PERMITIO_API_KEY empty: Fatal(%q), want a Fatal "+
			"naming PERMITIO_API_KEY", missing.fatal)
	}

	t.Setenv("PERMITIO_API_KEY", "some-key")
	set := &fatalRecorder{TB: t}
	testAccPreCheck(set)
	if set.fatal != "" {
		t.Errorf("testAccPreCheck with PERMITIO_API_KEY set: Fatal(%q), want no Fatal", set.fatal)
	}
}

func TestPermitConfigReadsProviderEnv(t *testing.T) {
	t.Setenv("PERMITIO_API_KEY", "some-key")
	tests := []struct {
		name        string
		url         string
		timeout     string
		wantURL     string
		wantTimeout time.Duration
	}{
		{
			name: "defaults", wantURL: DefaultApiUrl, wantTimeout: DefaultTimeout,
		},
		{
			name: "PERMITIO_API_URL and PERMITIO_TIMEOUT", url: "http://127.0.0.1:1",
			timeout: "20", wantURL: "http://127.0.0.1:1", wantTimeout: 20 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setOrUnsetEnv(t, "PERMITIO_API_URL", tt.url)
			setOrUnsetEnv(t, "PERMITIO_TIMEOUT", tt.timeout)

			cfg, err := testAccPermitConfig()

			if err != nil {
				t.Fatalf("testAccPermitConfig() error = %v", err)
			}
			if got := cfg.GetApiUrl(); got != tt.wantURL {
				t.Errorf("API URL = %q, want %q", got, tt.wantURL)
			}
			if got := cfg.GetToken(); got != "some-key" {
				t.Errorf("API key = %q, want %q", got, "some-key")
			}
			if got := cfg.GetHTTPClient().Timeout; got != tt.wantTimeout {
				t.Errorf("HTTP client timeout = %s, want %s", got, tt.wantTimeout)
			}
			transport := cfg.GetHTTPClient().Transport
			if _, ok := transport.(*httpclient.Transport); !ok {
				t.Errorf("HTTP client transport = %T, want the provider's retrying "+
					"*httpclient.Transport", transport)
			}
		})
	}

	t.Run("malformed PERMITIO_TIMEOUT", func(t *testing.T) {
		t.Setenv("PERMITIO_TIMEOUT", "20s")
		if _, err := testAccPermitClient(); err == nil ||
			!strings.Contains(err.Error(), "PERMITIO_TIMEOUT") {
			t.Fatalf("testAccPermitClient() error = %v, want one naming PERMITIO_TIMEOUT", err)
		}
	})
}

// setOrUnsetEnv sets key to value for the rest of the test, or unsets it when value
// is empty; t.Setenv restores the old value either way.
func setOrUnsetEnv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
	if value == "" {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unsetting %s: %v", key, err)
		}
	}
}
