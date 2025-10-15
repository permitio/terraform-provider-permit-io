package tenants

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &TenantResource{}
	_ resource.ResourceWithConfigure = &TenantResource{}
)

func NewTenantResource() resource.Resource {
	return &TenantResource{}
}

type TenantResource struct {
	client tenantClient
}

func (r *TenantResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	permitClient := common.Configure(ctx, request, response)
	r.client = tenantClient{client: permitClient}
}

func (r *TenantResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_tenant"
}

func (r *TenantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := common.CreateBaseResourceSchema()

	attributes["last_action_at"] = schema.StringAttribute{
		MarkdownDescription: "Date and time when the tenant was last active (ISO_8601 format). In other words, this is the last time a permission check was done on a resource belonging to this tenant.",
		Computed:            true,
	}

	attributes["attributes"] = schema.StringAttribute{
		MarkdownDescription: "Arbitrary tenant attributes in JSON format that will be used to enforce attribute-based access control policies.",
		Optional:            true,
		Computed:            true,
	}

	resp.Schema = schema.Schema{
		Attributes:          attributes,
		MarkdownDescription: "Manages a Permit.io tenant. Tenants represent isolated groups or organizations within your application. See [the documentation](https://api.permit.io/v2/redoc#tag/Tenants) for more information about tenants.",
	}
}

func (r *TenantResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var plan tenantModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)

	if response.Diagnostics.HasError() {
		return
	}

	tenantRead, err := r.client.Create(ctx, plan)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to create tenant",
			fmt.Errorf("unable to create tenant: %w", err).Error(),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, tenantRead)...)
}

func (r *TenantResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var model tenantModel

	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	tenantRead, err := r.client.Read(ctx, model.Key.ValueString())

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read tenant",
			fmt.Errorf("unable to read tenant: %w", err).Error(),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, &tenantRead)...)
}

func (r *TenantResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	var plan tenantModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)

	if response.Diagnostics.HasError() {
		return
	}

	tenantRead, err := r.client.Update(ctx, plan)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to update tenant",
			fmt.Errorf("unable to update tenant: %w", err).Error(),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, tenantRead)...)
}

func (r *TenantResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var model tenantModel
	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, model.Key.ValueString())

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to delete tenant",
			fmt.Errorf("unable to delete tenant: %w", err).Error(),
		)
	}
}
