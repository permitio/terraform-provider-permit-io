package proxy_configs

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"strings"
)

var (
	_ resource.Resource              = &proxyConfigResource{}
	_ resource.ResourceWithConfigure = &proxyConfigResource{}
)

func NewProxyConfigResource() resource.Resource {
	return &proxyConfigResource{}
}

type proxyConfigResource struct {
	client proxyConfigClient
}

func (c *proxyConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_proxy_config"
}

func (c *proxyConfigResource) Configure(_ context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	permitClient, ok := request.ProviderData.(*permit.Client)

	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *permit.Client, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)
		return
	}

	c.client = proxyConfigClient{client: permitClient}

}

func (c *proxyConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "See [the documentation](https://api.permit.io/v2/redoc#tag/Proxy-Config/operation/create_proxy_config) for more information about proxy configs.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the proxy config",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the organization that owns the proxy config",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the project that owns the proxy config",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"environment_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the environment that owns the proxy config",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				MarkdownDescription: "Proxy Config is set to enable the Permit Proxy to make proxied requests as part of the Frontend AuthZ.\n\n",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the proxy config, for example: 'Stripe API",
				Required:            true,
			},
			"auth_mechanism": schema.StringAttribute{
				MarkdownDescription: "Default: \"Bearer\"\nEnum: \"Bearer\" \"Basic\" \"Headers\"\nProxy config auth mechanism will define the authentication mechanism that will be used to authenticate the request.\n\nBearer injects the secret into the Authorization header as a Bearer token,\n\nBasic injects the secret into the Authorization header as a Basic user:password,\n\nHeaders injects plain headers into the request.",
				Required:            true,
				Validators: []validator.String{
					authMechanismValidator{},
				},
			},
			"auth_secret": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "Proxy config secret is set to enable the Permit Proxy to make proxied requests to the backend service.",
				Attributes: map[string]schema.Attribute{
					"bearer": schema.StringAttribute{
						Optional: true,
					},
					"basic": schema.StringAttribute{
						Optional: true,
					},
					"headers": schema.MapAttribute{
						Optional:    true,
						ElementType: types.StringType,
					},
				},
			},
			"mapping_rules": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "Proxy config mapping rules will include the rules that will be used to map the request to the backend service by a URL and a http method.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"url": schema.StringAttribute{
							Required: true,
						},
						"http_method": schema.StringAttribute{
							Required: true,
						},
						"resource": schema.StringAttribute{
							Required: true,
						},
						"action": schema.StringAttribute{
							Optional: true,
						},
						"priority": schema.Int64Attribute{
							Optional: true,
						},
						"headers": schema.MapAttribute{
							Optional:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

func (c *proxyConfigResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.Conflicting(
			path.MatchRoot("auth_secret").AtName("basic"),
			path.MatchRoot("auth_secret").AtName("bearer"),
			path.MatchRoot("auth_secret").AtName("headers"),
		),
	}
}

func (c *proxyConfigResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data proxyConfigModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if strings.EqualFold(data.AuthMechanism.ValueString(), string(models.BASIC)) && data.AuthSecret.Basic.IsNull() {
		resp.Diagnostics.AddError("auth_mechanism was set to `basic` but auth_secret.basic is not set", "")
		return
	}

	if strings.EqualFold(data.AuthMechanism.ValueString(), string(models.BEARER)) && data.AuthSecret.Bearer.IsNull() {
		resp.Diagnostics.AddError("auth_mechanism was set to `bearer` but auth_secret.bearer is not set", "")
		return
	}

	if strings.EqualFold(data.AuthMechanism.ValueString(), string(models.HEADERS)) && data.AuthSecret.Headers.IsNull() {
		resp.Diagnostics.AddError("auth_mechanism was set to `headers` but auth_secret.headers is not set", "")
		return
	}
}

func (c *proxyConfigResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var (
		model proxyConfigModel
	)

	response.Diagnostics.Append(request.Plan.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	created, err := c.client.create(ctx, model)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to create proxy config",
			fmt.Sprintf("Unable to create resource: %s", err),
		)
		return
	}

	// Set state to fully populated data
	response.Diagnostics.Append(response.State.Set(ctx, created)...)

	if response.Diagnostics.HasError() {
		return
	}
}

func (c *proxyConfigResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var model proxyConfigModel

	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	read, err := c.client.read(ctx, model)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to Read Condition Set",
			fmt.Sprintf("Unable to read condition set: %s, Error: %s", read.Id.String(), err.Error()),
		)
		return
	}

	// Set state
	response.Diagnostics.Append(response.State.Set(ctx, &read)...)

	if response.Diagnostics.HasError() {
		return
	}
}

func (c *proxyConfigResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	var model proxyConfigModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	proxyConfig, err := c.client.update(ctx, model)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to update resource",
			fmt.Sprintf("Unable to update resource: %s", err),
		)
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, proxyConfig)...)
	if response.Diagnostics.HasError() {
		return
	}
}

func (c *proxyConfigResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var model proxyConfigModel
	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	err := c.client.delete(ctx, model)

	if err != nil {
		response.Diagnostics.AddError(
			"Error deleting Proxy Config",
			fmt.Sprintf("Could not delete Proxy Config, unexpected error: %s", err.Error()),
		)
		return
	}
}
