// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
				Optional: true,
				MarkdownDescription: "The base URL of the Permit.io API, an absolute `http` or " +
					"`https` URL. Defaults to `" + DefaultApiUrl + "`. Can also be set with the `" +
					apiURLEnvVar + "` environment variable; a value set here takes precedence.",
				Validators: []validator.String{apiURLValidator{}},
			},
			"api_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "An environment-level API key for the Permit.io API; the " +
					"provider manages the environment the key belongs to. Can also be set with " +
					"the `" + apiKeyEnvVar + "` environment variable; a value set here takes " +
					"precedence. The provider needs a key from one of the two.",
			},
			"timeout": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "The time limit, in seconds, for each request to the " +
					"Permit.io API, including its retries after a 429, 502, 503 or 504 response. " +
					"Must be greater than 0. Defaults to `10`. Can also be set with the `" +
					timeoutEnvVar + "` environment variable; a value set here takes precedence.",
				Validators: []validator.Int64{int64validator.Between(1, maxTimeoutSeconds)},
			},
		},
	}
}

// The environment variables that set the API key, API URL and timeout when the
// provider block leaves them out.
const (
	apiKeyEnvVar  = "PERMITIO_API_KEY"
	apiURLEnvVar  = "PERMITIO_API_URL"
	timeoutEnvVar = "PERMITIO_TIMEOUT"
)

// maxTimeoutSeconds is the largest timeout, in seconds, that fits in a
// time.Duration. A larger one would wrap to a negative Duration, which
// http.Client treats as no time limit.
const maxTimeoutSeconds = math.MaxInt64 / int64(time.Second)

func (p *PermitProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config PermitProviderModel
	tflog.Info(ctx, "Configuring Permit.io client")

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	for _, setting := range []struct {
		attribute, envVar string
		unknown           bool
	}{
		{"api_url", apiURLEnvVar, config.ApiUrl.IsUnknown()},
		{"api_key", apiKeyEnvVar, config.ApiKey.IsUnknown()},
		{"timeout", timeoutEnvVar, config.Timeout.IsUnknown()},
	} {
		if setting.unknown {
			resp.Diagnostics.AddAttributeError(path.Root(setting.attribute),
				"Unknown Permit.io provider "+setting.attribute,
				"The provider cannot create the Permit.io API client, because "+
					setting.attribute+" depends on a value that is not known yet. Apply the "+
					"source of the value first with -target, set "+setting.attribute+
					" to a known value, or leave "+setting.attribute+" out and set the "+
					setting.envVar+" environment variable.")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	settings, diags := resolveSettings(config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "permitio_api_url", settings.apiURL)
	ctx = tflog.SetField(ctx, "permitio_api_key", settings.apiKey)
	ctx = tflog.SetField(ctx, "permitio_timeout", settings.timeout.String())
	ctx = tflog.MaskFieldValuesWithFieldKeys(ctx, "permitio_api_key")

	tflog.Debug(ctx, "Instantiating Permit.io client")
	clientConfig := newClientConfig(settings.apiKey, settings.apiURL, settings.timeout, p.version)
	permitContext, err := resolveAPIKeyScope(ctx, clientConfig)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the Permit.io API key scope",
			err.Error()+"\n\n"+scopeErrorHint(err))
		return
	}
	permitClient := permit.NewPermit(clientConfig.WithContext(permitContext).Build())

	// Store config globally for resources that need direct HTTP access
	globalconfig.SetGlobalConfig(settings.apiURL, settings.apiKey)

	resp.DataSourceData = permitClient
	resp.ResourceData = permitClient

	tflog.Info(ctx, "Permit.io client configured", map[string]any{"success": true})
}

// providerSettings are the API key, API URL and timeout of the provider's client.
type providerSettings struct {
	apiKey  string
	apiURL  string
	timeout time.Duration
}

// resolveSettings takes each setting from the provider block in config, else from
// its environment variable, else from its default; an empty environment variable
// counts as unset. It checks the environment variables it reads, since the
// schema's validators check only the block, and it reports a missing API key.
// config must have no unknown values.
func resolveSettings(config PermitProviderModel) (providerSettings, diag.Diagnostics) {
	var diags diag.Diagnostics
	settings := providerSettings{apiURL: DefaultApiUrl, timeout: DefaultTimeout}

	if !config.ApiKey.IsNull() {
		settings.apiKey = config.ApiKey.ValueString()
	} else {
		settings.apiKey = os.Getenv(apiKeyEnvVar)
	}
	if settings.apiKey == "" {
		diags.AddAttributeError(path.Root("api_key"), "Missing Permit.io API key",
			"The provider needs an environment-level Permit.io API key. Set api_key in the "+
				"provider block, or leave api_key out and set the "+apiKeyEnvVar+
				" environment variable. Neither may be empty.")
	}

	if !config.ApiUrl.IsNull() {
		settings.apiURL = config.ApiUrl.ValueString()
	} else if value := os.Getenv(apiURLEnvVar); value != "" {
		if isAbsoluteHTTPURL(value) {
			settings.apiURL = value
		} else {
			diags.AddError("Invalid "+apiURLEnvVar+" environment variable",
				fmt.Sprintf("%s is %q, which is not an absolute http or https URL such as %s. "+
					"It sets api_url when the provider block leaves api_url out.",
					apiURLEnvVar, value, DefaultApiUrl))
		}
	}

	if !config.Timeout.IsNull() {
		settings.timeout = time.Duration(config.Timeout.ValueInt64()) * time.Second
	} else if value := os.Getenv(timeoutEnvVar); value != "" {
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err == nil && seconds >= 1 && seconds <= maxTimeoutSeconds {
			settings.timeout = time.Duration(seconds) * time.Second
		} else {
			diags.AddError("Invalid "+timeoutEnvVar+" environment variable",
				fmt.Sprintf("%s is %q, which is not a whole number of seconds from 1 to %d. "+
					"It sets timeout when the provider block leaves timeout out.",
					timeoutEnvVar, value, maxTimeoutSeconds))
		}
	}

	return settings, diags
}

// isAbsoluteHTTPURL reports whether value is an absolute http or https URL.
func isAbsoluteHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Host != ""
}

// apiURLValidator rejects an api_url that is not an absolute http or https URL.
type apiURLValidator struct{}

func (apiURLValidator) Description(context.Context) string {
	return "value must be an absolute http or https URL"
}

func (v apiURLValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (apiURLValidator) ValidateString(
	_ context.Context, req validator.StringRequest, resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if value := req.ConfigValue.ValueString(); !isAbsoluteHTTPURL(value) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Permit.io API URL",
			fmt.Sprintf("api_url is %q, which is not an absolute http or https URL such as %s.",
				value, DefaultApiUrl))
	}
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
