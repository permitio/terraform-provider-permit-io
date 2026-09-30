// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/permitio/permit-golang/pkg/api"
	permitConfig "github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/openapi"
	"github.com/permitio/permit-golang/pkg/permit"
	conditionsetrules "github.com/permitio/terraform-provider-permit-io/internal/provider/conditionset_rules"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/conditionsets"
	globalconfig "github.com/permitio/terraform-provider-permit-io/internal/provider/config"
	group_resource_instance_role_assignments "github.com/permitio/terraform-provider-permit-io/internal/provider/group_resource_instance_role_assignments"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/httpclient"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/proxy_configs"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/relations"
	resource_instance_role_assignments "github.com/permitio/terraform-provider-permit-io/internal/provider/resource_instance_role_assignments"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/resource_instances"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/resources"
	role_assignments "github.com/permitio/terraform-provider-permit-io/internal/provider/role_assignments"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/role_derivations"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/roles"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/tenants"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/user_attributes"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/users"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	DefaultApiUrl  = "https://api.permit.io"
	PDPApiUrl      = "https://localhost:3000"
	DefaultTimeout = 10 * time.Second
)

// Ensure PermitProvider satisfies various provider interfaces.
var _ provider.Provider = &PermitProvider{}

// PermitProvider defines the provider implementation.
type PermitProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// PermitProviderModel describes the provider data model.
type PermitProviderModel struct {
	ApiUrl  types.String `tfsdk:"api_url"`
	ApiKey  types.String `tfsdk:"api_key"`
	Timeout types.Int64  `tfsdk:"timeout"`
}

func (p *PermitProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "permitio"
	resp.Version = p.version
}

func (p *PermitProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The URL of Permit.io API",
				// TODO: Add validation for URL
			},
			"api_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				// TODO: Add support in more API key levels
				MarkdownDescription: "The API key for Permit.io API (Required)",
			},
			"timeout": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Timeout for the requests to Permit.io API - default is 10 seconds",
			},
		},
	}
}

func (p *PermitProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config PermitProviderModel
	tflog.Info(ctx, "Configuring Permit.io client")

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if config.ApiUrl.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Unknown Permit.io API URL",
			"The provider cannot create the Permit.io API client as there is an unknown configuration value for the Permit.io API URL. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the PERMITIO_API_URL environment variable.",
		)
	}

	if config.ApiKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Unknown Permit.io API Key",
			"The provider cannot create the Permit.io API client as there is an unknown configuration value for the Permit.io API Key. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the PERMITIO_API_KEY environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	debug := os.Getenv("PERMITIO_DEBUG") == "true"
	apiKey, apiKeyExist := os.LookupEnv("PERMITIO_API_KEY")
	tflog.Debug(ctx, "API Key exists in env var 'PERMITIO_API_KEY': "+strconv.FormatBool(apiKeyExist))
	if !apiKeyExist {
		if config.ApiKey.IsNull() {
			resp.Diagnostics.AddError(
				"Missing Permit.io API Key",
				"The provider cannot create the Permit.io API client as there is an unknown configuration value for the Permit.io API Key."+
					"Either target apply the source of the value first, set the value statically in the configuration, or use the PERMITIO_API_KEY environment variable.")
		} else {
			apiKey = config.ApiKey.ValueString()
		}
	}

	apiUrl, apiUrlExist := os.LookupEnv("PERMITIO_API_URL")
	if !apiUrlExist {
		if config.ApiUrl.IsNull() {
			apiUrl = DefaultApiUrl
		} else {
			apiUrl = config.ApiUrl.ValueString()
		}
	}

	var timeout int64
	timeoutStr, timeoutExist := os.LookupEnv("PERMITIO_TIMEOUT")
	if timeoutExist {
		timeoutInt, err := strconv.ParseInt(timeoutStr, 10, 64)
		if err != nil {
			tflog.Debug(ctx, "Error parsing timeout from env var 'PERMITIO_TIMEOUT': "+err.Error())
			resp.Diagnostics.AddAttributeError(
				path.Root("timeout"),
				"Timeout is not a valid integer",
				"The provider cannot create the Permit.io API client as the timeout value is not a valid integer.",
			)
			return
		}
		timeout = timeoutInt * int64(time.Second)
	} else {
		if config.Timeout.IsNull() {
			timeout = int64(DefaultTimeout)
		} else {
			timeout = config.Timeout.ValueInt64() * int64(time.Second)
		}
	}

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "permitio_api_url", apiUrl)
	ctx = tflog.SetField(ctx, "permitio_api_key", apiKey)
	ctx = tflog.SetField(ctx, "permitio_timeout", timeout)
	ctx = tflog.MaskFieldValuesWithFieldKeys(ctx, "permitio_api_key")

	tflog.Debug(ctx, "Instantiating Permit.io client")
	clientConfig := newClientConfig(apiKey, apiUrl, time.Duration(timeout), p.version).
		WithDebug(debug)
	permitContext, err := resolveAPIKeyScope(ctx, clientConfig)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the Permit.io API key scope",
			err.Error()+"\n\n"+scopeErrorHint(err))
		return
	}
	permitClient := permit.NewPermit(clientConfig.WithContext(permitContext).Build())

	// Store config globally for resources that need direct HTTP access
	globalconfig.SetGlobalConfig(apiUrl, apiKey)

	resp.DataSourceData = permitClient
	resp.ResourceData = permitClient

	tflog.Info(ctx, "Permit.io client configured", map[string]any{"success": true})
}

// newClientConfig returns the SDK configuration for the Permit API at apiURL. Its
// HTTP client retries rate-limited and failed requests and names this provider
// version in the User-Agent. The client is set before the timeout because
// WithTimeout sets the timeout on the client the configuration has.
func newClientConfig(
	apiKey, apiURL string, timeout time.Duration, version string,
) *permitConfig.PermitConfig {
	return permitConfig.NewConfigBuilder(apiKey).
		WithApiUrl(apiURL).
		WithHTTPClient(httpclient.New(version)).
		WithTimeout(timeout)
}

// apiKeyScopePath is where the Permit API reports an API key's scope.
const apiKeyScopePath = "/v2/api-key/scope"

// resolveAPIKeyScope asks the Permit API which organization, project and
// environment the API key belongs to. The SDK needs the answer for every call.
// Unless it is given one, the SDK asks on the first call, and again on every call
// that starts before an answer arrives, with no lock around where it keeps the
// answer, and it reports a failure without the API's status or message.
func resolveAPIKeyScope(
	ctx context.Context, cfg *permitConfig.PermitConfig,
) (*permitConfig.PermitContext, error) {
	client := openapi.NewAPIClient(api.NewClientConfig(cfg))
	scope, httpResp, err := client.APIKeysApi.GetApiKeyScope(ctx).Execute()
	if err != nil {
		var apiErr *openapi.GenericOpenAPIError
		if httpResp != nil && httpResp.StatusCode >= http.StatusMultipleChoices &&
			errors.As(err, &apiErr) {
			err = &apiStatusError{
				code:    httpResp.StatusCode,
				status:  httpResp.Status,
				message: apiErrorMessage(apiErr.Body()),
			}
		}
		return nil, fmt.Errorf("GET %s: %w", apiKeyScopePath, err)
	}
	return permitConfig.NewPermitContext(permitConfig.GetApiKeyLevel(scope),
		scope.GetProjectId(), scope.GetEnvironmentId()), nil
}

// apiStatusError is an error answer of the Permit API.
type apiStatusError struct {
	code    int
	status  string
	message string
}

func (e *apiStatusError) Error() string {
	return fmt.Sprintf("the Permit.io API answered %s: %s", e.status, e.message)
}

// scopeErrorHint says what to check when reading the API key scope failed with err.
func scopeErrorHint(err error) string {
	var statusErr *apiStatusError
	if errors.As(err, &statusErr) {
		switch {
		case statusErr.code == http.StatusUnauthorized || statusErr.code == http.StatusForbidden:
			return "The Permit.io API rejected the API key. Check the api_key argument or " +
				"the PERMITIO_API_KEY environment variable."
		case statusErr.code == http.StatusTooManyRequests ||
			statusErr.code >= http.StatusInternalServerError:
			return "The Permit.io API is busy or unavailable. Try again later, or raise " +
				"the timeout argument or the PERMITIO_TIMEOUT environment variable so " +
				"that the provider retries for longer."
		}
	}
	return "Check the api_url argument or the PERMITIO_API_URL environment variable, " +
		"and the api_key argument or the PERMITIO_API_KEY environment variable."
}

// maxErrorBodyBytes caps how much of an error body without a message, such as the
// HTML page of a proxy, goes into an error.
const maxErrorBodyBytes = 512

// apiErrorMessage returns the message of a Permit API error body, or else the start
// of the body.
func apiErrorMessage(body []byte) string {
	var apiError struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Message != "" {
		return apiError.Message
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "(empty response body)"
	}
	if len(trimmed) > maxErrorBodyBytes {
		return strings.ToValidUTF8(trimmed[:maxErrorBodyBytes], "") + "... (truncated)"
	}
	return trimmed
}

func (p *PermitProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewResourceResource,
		roles.NewRoleResource,
		conditionsets.NewUserSetResource,
		conditionsets.NewResourceSetResource,
		conditionsetrules.NewConditionSetRuleResource,
		proxy_configs.NewProxyConfigResource,
		relations.NewRelationResource,
		role_derivations.NewRoleDerivationResource,
		tenants.NewTenantResource,
		user_attributes.NewUserAttributeResource,
		role_assignments.NewRoleAssignmentResource,
		resource_instances.NewResourceInstanceResource,
		resource_instance_role_assignments.NewResourceInstanceRoleAssignmentResource,
		group_resource_instance_role_assignments.NewGroupResourceInstanceRoleAssignmentResource,
	}
}

func (p *PermitProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		resources.NewResourceDataSource,
		roles.NewRoleDataSource,
		conditionsets.NewConditionSetDataSource,
		users.NewUserDataSource,
		user_attributes.NewUserAttributeDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &PermitProvider{
			version: version,
		}
	}
}
