package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// setProviderEnv sets the provider's environment variables to env for the rest of
// the test, and unsets the ones env leaves out. An empty value in env stays set.
func setProviderEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, name := range []string{"PERMITIO_API_KEY", "PERMITIO_API_URL", "PERMITIO_TIMEOUT"} {
		value, ok := env[name]
		if !ok {
			setOrUnsetEnv(t, name, "")
			continue
		}
		t.Setenv(name, value)
	}
}

// TestProviderSettingsPrecedence checks where each provider setting comes from: the
// provider block, else its environment variable, else its default. An empty
// environment variable counts as unset, and the provider does not read an
// environment variable whose setting the block sets.
func TestProviderSettingsPrecedence(t *testing.T) {
	const (
		blockKey = "block-key"
		envKey   = "env-key"
		blockURL = "https://block.example.com"
		envURL   = "http://env.example.com:8080"
	)
	block := PermitProviderModel{
		ApiKey:  types.StringValue(blockKey),
		ApiUrl:  types.StringValue(blockURL),
		Timeout: types.Int64Value(30),
	}
	keyOnly := PermitProviderModel{
		ApiKey:  types.StringValue(blockKey),
		ApiUrl:  types.StringNull(),
		Timeout: types.Int64Null(),
	}
	empty := PermitProviderModel{
		ApiKey:  types.StringNull(),
		ApiUrl:  types.StringNull(),
		Timeout: types.Int64Null(),
	}
	fromEnv := map[string]string{
		"PERMITIO_API_KEY": envKey,
		"PERMITIO_API_URL": envURL,
		"PERMITIO_TIMEOUT": "45",
	}
	blockSettings := providerSettings{apiKey: blockKey, apiURL: blockURL,
		timeout: 30 * time.Second}
	tests := []struct {
		name  string
		block PermitProviderModel
		env   map[string]string
		want  providerSettings
	}{
		{
			name: "block wins over environment", block: block, env: fromEnv,
			want: blockSettings,
		},
		{
			name: "environment when the block leaves the settings out", block: empty,
			env:  fromEnv,
			want: providerSettings{apiKey: envKey, apiURL: envURL, timeout: 45 * time.Second},
		},
		{
			name: "defaults when neither sets them", block: keyOnly,
			want: providerSettings{apiKey: blockKey, apiURL: DefaultApiUrl,
				timeout: DefaultTimeout},
		},
		{
			name: "empty environment variables count as unset", block: keyOnly,
			env: map[string]string{"PERMITIO_API_KEY": "", "PERMITIO_API_URL": "",
				"PERMITIO_TIMEOUT": ""},
			want: providerSettings{apiKey: blockKey, apiURL: DefaultApiUrl,
				timeout: DefaultTimeout},
		},
		{
			name: "block wins over empty environment variables", block: block,
			env: map[string]string{"PERMITIO_API_KEY": "", "PERMITIO_API_URL": "",
				"PERMITIO_TIMEOUT": ""},
			want: blockSettings,
		},
		{
			name: "largest timeout that fits in a time.Duration", block: keyOnly,
			env: map[string]string{"PERMITIO_TIMEOUT": strconv.FormatInt(maxTimeoutSeconds, 10)},
			want: providerSettings{apiKey: blockKey, apiURL: DefaultApiUrl,
				timeout: time.Duration(maxTimeoutSeconds) * time.Second},
		},
		{
			name:  "invalid environment variables are ignored when the block sets them",
			block: block,
			env: map[string]string{"PERMITIO_API_URL": "not a URL",
				"PERMITIO_TIMEOUT": "soon"},
			want: blockSettings,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setProviderEnv(t, tt.env)

			got, diags := resolveSettings(tt.block)

			if diags.HasError() {
				t.Fatalf("resolveSettings() diagnostics = %v, want none", diags)
			}
			if got != tt.want {
				t.Errorf("resolveSettings() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestProviderSettingsErrors checks that a missing API key and an environment
// variable the provider cannot use each give one error that names both the
// provider block argument and the environment variable.
func TestProviderSettingsErrors(t *testing.T) {
	keyOnly := PermitProviderModel{
		ApiKey:  types.StringValue("block-key"),
		ApiUrl:  types.StringNull(),
		Timeout: types.Int64Null(),
	}
	noKey := PermitProviderModel{
		ApiKey:  types.StringNull(),
		ApiUrl:  types.StringNull(),
		Timeout: types.Int64Null(),
	}
	emptyKey := PermitProviderModel{
		ApiKey:  types.StringValue(""),
		ApiUrl:  types.StringNull(),
		Timeout: types.Int64Null(),
	}
	missingKey := []string{"Missing Permit.io API key", "api_key", "PERMITIO_API_KEY"}
	badURL := []string{"PERMITIO_API_URL", "api_url", "absolute http or https URL"}
	badTimeout := []string{"PERMITIO_TIMEOUT", "timeout", "from 1 to 9223372036"}
	tests := []struct {
		name  string
		block PermitProviderModel
		env   map[string]string
		want  []string
	}{
		{name: "no API key anywhere", block: noKey, want: missingKey},
		{
			name: "empty PERMITIO_API_KEY", block: noKey,
			env: map[string]string{"PERMITIO_API_KEY": ""}, want: missingKey,
		},
		{
			name: "empty api_key in the block", block: emptyKey,
			env: map[string]string{"PERMITIO_API_KEY": "env-key"}, want: missingKey,
		},
		{
			name: "PERMITIO_API_URL without a scheme", block: keyOnly,
			env: map[string]string{"PERMITIO_API_URL": "api.permit.io"}, want: badURL,
		},
		{
			name: "PERMITIO_API_URL with another scheme", block: keyOnly,
			env: map[string]string{"PERMITIO_API_URL": "ftp://api.permit.io"}, want: badURL,
		},
		{
			name: "PERMITIO_API_URL without a host", block: keyOnly,
			env: map[string]string{"PERMITIO_API_URL": "https://"}, want: badURL,
		},
		{
			name: "PERMITIO_TIMEOUT of 0", block: keyOnly,
			env: map[string]string{"PERMITIO_TIMEOUT": "0"}, want: badTimeout,
		},
		{
			name: "negative PERMITIO_TIMEOUT", block: keyOnly,
			env: map[string]string{"PERMITIO_TIMEOUT": "-5"}, want: badTimeout,
		},
		{
			name: "PERMITIO_TIMEOUT with a unit", block: keyOnly,
			env: map[string]string{"PERMITIO_TIMEOUT": "10s"}, want: badTimeout,
		},
		{
			name: "PERMITIO_TIMEOUT too large for a time.Duration", block: keyOnly,
			env: map[string]string{"PERMITIO_TIMEOUT": "9223372037"}, want: badTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setProviderEnv(t, tt.env)

			_, diags := resolveSettings(tt.block)

			errs := diags.Errors()
			if len(errs) != 1 {
				t.Fatalf("resolveSettings() errors = %v, want exactly one", errs)
			}
			message := errs[0].Summary() + ": " + errs[0].Detail()
			for _, want := range tt.want {
				if !strings.Contains(message, want) {
					t.Errorf("error = %q, want it to contain %q", message, want)
				}
			}
		})
	}
}

// unusedAPI starts a server that fails the test on every request, for an API URL
// the provider must not use.
func unusedAPI(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s to an API URL the provider must not use",
			r.Method, r.URL.Path)
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// TestConfigureRequiresAPIKey checks that Configure fails, without sending a
// request, when no API key is set and PERMITIO_API_KEY is empty.
func TestConfigureRequiresAPIKey(t *testing.T) {
	setProviderEnv(t, map[string]string{
		"PERMITIO_API_KEY": "",
		"PERMITIO_API_URL": unusedAPI(t),
	})

	configureError(t, "api_key", "PERMITIO_API_KEY")
}

// TestConfigureRejectsUnknownSettings checks that Configure fails, without sending a
// request, when the provider block sets a setting to a value Terraform does not
// know yet, even though the setting's environment variable is set.
func TestConfigureRejectsUnknownSettings(t *testing.T) {
	tests := []struct {
		attribute, envVar string
		valueType         tftypes.Type
	}{
		{"api_url", "PERMITIO_API_URL", tftypes.String},
		{"api_key", "PERMITIO_API_KEY", tftypes.String},
		{"timeout", "PERMITIO_TIMEOUT", tftypes.Number},
	}
	for _, tt := range tests {
		t.Run(tt.attribute, func(t *testing.T) {
			setProviderEnv(t, map[string]string{
				"PERMITIO_API_KEY": "env-key",
				"PERMITIO_API_URL": unusedAPI(t),
				"PERMITIO_TIMEOUT": "5",
			})

			resp := configureBlock(t, map[string]tftypes.Value{
				tt.attribute: tftypes.NewValue(tt.valueType, tftypes.UnknownValue),
			})

			errs := resp.Diagnostics.Errors()
			if len(errs) != 1 {
				t.Fatalf("Configure() errors = %v, want exactly one", errs)
			}
			for _, want := range []string{tt.attribute, tt.envVar, "not known yet"} {
				if !strings.Contains(errs[0].Detail(), want) {
					t.Errorf("error detail = %q, want it to contain %q", errs[0].Detail(), want)
				}
			}
			if resp.ResourceData != nil || resp.DataSourceData != nil {
				t.Error("Configure() failed but set the client, want none")
			}
		})
	}
}

// TestProviderBlockOverridesEnvironment runs Terraform with a provider block that
// sets every setting while the environment variables name another API, another key
// and a timeout that is not a number: the requests go to the block's API with the
// block's key.
func TestProviderBlockOverridesEnvironment(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Tenants)
	setProviderEnv(t, map[string]string{
		"PERMITIO_API_KEY": "env-key",
		"PERMITIO_API_URL": unusedAPI(t),
		"PERMITIO_TIMEOUT": "soon",
	})
	config := fmt.Sprintf(`
provider "permitio" {
  api_url = %q
  api_key = %q
  timeout = 30
}

resource "permitio_tenant" "test" {
  key  = "acme"
  name = "Acme"
}
`, m.URL, mockpermit.APIKey)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{{
			Config: config,
			Check:  resource.TestCheckResourceAttr("permitio_tenant.test", "key", "acme"),
		}},
	})

	scopeRequests := m.Requests(http.MethodGet, apiKeyScopePath)
	if len(scopeRequests) == 0 {
		t.Fatal("the block's API got no API key scope request")
	}
	for _, r := range scopeRequests {
		if got, want := r.Header.Get("Authorization"), "Bearer "+mockpermit.APIKey; got != want {
			t.Errorf("scope request Authorization = %q, want %q", got, want)
		}
	}
}

// TestProviderBlockValidation checks that Terraform rejects an api_url that is not
// an absolute http or https URL and a timeout below 1 or too large for a
// time.Duration while validating the provider block, before the provider sends a
// request.
func TestProviderBlockValidation(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Tenants)
	tenant := `
resource "permitio_tenant" "test" {
  key  = "acme"
  name = "Acme"
}
`
	badTimeout := regexp.MustCompile(`timeout\s+value\s+must\s+be\s+between\s+1\s+and\s+9223372036`)
	tests := []struct {
		name     string
		argument string
		want     *regexp.Regexp
	}{
		{
			name: "api_url without a scheme", argument: `api_url = "api.permit.io"`,
			want: regexp.MustCompile(`Invalid Permit\.io API URL`),
		},
		{
			name: "api_url with another scheme", argument: `api_url = "ftp://api.permit.io"`,
			want: regexp.MustCompile(`Invalid Permit\.io API URL`),
		},
		{name: "timeout of 0", argument: `timeout = 0`, want: badTimeout},
		{
			name: "timeout too large for a time.Duration", argument: `timeout = 9223372037`,
			want: badTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      "provider \"permitio\" {\n  " + tt.argument + "\n}\n" + tenant,
					ExpectError: tt.want,
				}},
			})
		})
	}

	if got := m.Requests(http.MethodGet, apiKeyScopePath); len(got) != 0 {
		t.Errorf("API key scope requests = %d, want none: validation must fail first",
			len(got))
	}
}
