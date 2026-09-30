package proxy_configs

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/config"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/httpclient"
)

// TestHTTPRoutesMatchTheClient sends the update the client builds itself to a mock
// that serves only the route for it, and checks that the route receives it. The
// proxy config is a placeholder: the mock answers 404 for it.
func TestHTTPRoutesMatchTheClient(t *testing.T) {
	model := proxyConfigModel{
		Key:           types.StringValue("x"),
		Name:          types.StringValue("x"),
		AuthMechanism: types.StringValue("Bearer"),
		AuthSecret:    authSecretModel{Bearer: types.StringValue("x")},
	}

	mockpermit.CheckHTTPRoutes(t, mockpermit.ProxyConfigs,
		map[string]func(ctx context.Context, url string){
			"proxy_configs.update (HTTP)": func(ctx context.Context, url string) {
				client := proxyConfigClient{api: &config.API{
					HTTPClient: httpclient.New("test"), URL: url, Key: mockpermit.APIKey,
					ProjectID: mockpermit.ProjectID, EnvironmentID: mockpermit.EnvironmentID,
				}}
				_, _ = client.update(ctx, model, model)
			},
		})
}

// TestUpdateNeedsAConfiguredEnvironment checks that an update fails with a clear
// error, and sends nothing, before the provider is configured or with an API key
// that is not scoped to an environment.
func TestUpdateNeedsAConfiguredEnvironment(t *testing.T) {
	tests := map[string]struct {
		api  *config.API
		want error
	}{
		"not configured": {api: nil, want: errNotConfigured},
		"no environment": {
			api:  &config.API{HTTPClient: httpclient.New("test"), URL: "http://127.0.0.1:1"},
			want: errNoEnvironment,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			client := proxyConfigClient{api: tt.api}

			_, err := client.update(t.Context(), proxyConfigModel{}, proxyConfigModel{})

			if !errors.Is(err, tt.want) {
				t.Errorf("update error = %v, want %v", err, tt.want)
			}
		})
	}
}
