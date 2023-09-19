// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	permitConfig "github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/permit"
	conditionsetrules "github.com/permitio/terraform-provider-permit-io/internal/provider/conditionset_rules"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/conditionsets"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/resources"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/roles"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	DefaultApiUrl = "https://api.permit.io"
	PDPApiUrl     = "https://localhost:3000"
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
	ApiUrl types.String `tfsdk:"api_url"`
	ApiKey types.String `tfsdk:"api_key"`
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
	if !apiKeyExist {
		if config.ApiKey.IsNull() {
			resp.Diagnostics.AddError(
				"Missing Permit.io API Key",
				"The provider cannot create the Permit.io API client as there is an unknown configuration value for the Permit.io API Key."+
					"Either target apply the source of the value first, set the value statically in the configuration, or use the PERMITIO_API_KEY environment variable.")
		} else {
			apiKey = config.ApiKey.String()
		}
	}

	apiUrl, apiUrlExist := os.LookupEnv("PERMITIO_API_URL")
	if !apiUrlExist {
		if config.ApiUrl.IsNull() {
			apiUrl = DefaultApiUrl
		} else {
			apiUrl = config.ApiUrl.String()
		}
	}

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "permitio_api_url", apiUrl)
	ctx = tflog.SetField(ctx, "permitio_api_key", apiKey)
	ctx = tflog.MaskFieldValuesWithFieldKeys(ctx, "permitio_api_key")

	tflog.Debug(ctx, "Instantiating Permit.io client")
	clientConfig := permitConfig.NewConfigBuilder(apiKey).WithApiUrl(apiUrl).WithDebug(debug).Build()
	permitClient := permit.NewPermit(clientConfig)

	resp.DataSourceData = permitClient
	resp.ResourceData = permitClient

	tflog.Info(ctx, "Permit.io client configured", map[string]any{"success": true})
}

func (p *PermitProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewResourceResource,
		roles.NewRoleResource,
		conditionsets.NewUserSetResource,
		conditionsets.NewResourceSetResource,
		conditionsetrules.NewConditionSetRuleResource,
	}
}

func (p *PermitProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		resources.NewResourceDataSource,
		roles.NewRoleDataSource,
		conditionsets.NewConditionSetDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &PermitProvider{
			version: version,
		}
	}
}
