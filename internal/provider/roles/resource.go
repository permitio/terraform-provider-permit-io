package roles

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &RoleResource{}
	_ resource.ResourceWithConfigure = &RoleResource{}
)

func NewRoleResource() resource.Resource {
	return &RoleResource{}
}

type RoleResource struct {
	client roleClient
}

func (r *RoleResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	permitClient := common.Configure(ctx, request, response)
	r.client = roleClient{client: permitClient}
}

func (r *RoleResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_role"
}

func (r *RoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := common.CreateBaseResourceSchema()
	attributes["permissions"] = schema.SetAttribute{
		ElementType:         types.StringType,
		MarkdownDescription: "list of action keys that define what actions this resource role is permitted to do",
		Computed:            true,
		Optional:            true,
		PlanModifiers: []planmodifier.Set{
			setplanmodifier.UseStateForUnknown(),
		},
	}
	attributes["extends"] = schema.SetAttribute{
		MarkdownDescription: "list of role keys that define what roles this role extends. In other words: this role will automatically inherit all the permissions of the given roles in this list.",
		ElementType:         types.StringType,
		PlanModifiers: []planmodifier.Set{
			setplanmodifier.UseStateForUnknown(),
		},
		Computed: true,
		Optional: true,
	}
	attributes["resource"] = schema.StringAttribute{
		MarkdownDescription: "The unique resource key that the role belongs to.",
		Optional:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["resource_id"] = schema.StringAttribute{
		MarkdownDescription: "The unique resource ID that the role belongs to.",
		Computed:            true,
	}

	resp.Schema = schema.Schema{
		Attributes:          attributes,
		MarkdownDescription: "See [the documentation](https://api.permit.io/v2/redoc#tag/Resources/operation/create_resource) for more information about roles.\n You can also read about Resource Roles [here](https://api.permit.io/v2/redoc#tag/Resource-Roles/operation/create_resource_role).",
	}
}

func (r *RoleResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var plan roleModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)

	if response.Diagnostics.HasError() {
		return
	}

	roleRead, err := r.client.Create(ctx, plan)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to create role",
			fmt.Errorf("unable to create role: %w", err).Error(),
		)
	}

	response.Diagnostics.Append(response.State.Set(ctx, roleRead)...)
}

func (r *RoleResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var model roleModel

	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	roleRead, err := r.client.Read(
		ctx,
		model.Key.ValueString(),
		model.Resource.ValueStringPointer())

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read role",
			fmt.Errorf("unable to read role: %w", err).Error(),
		)
	}

	response.Diagnostics.Append(response.State.Set(ctx, &roleRead)...)
}

func (r *RoleResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	var plan roleModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)

	if response.Diagnostics.HasError() {
		return
	}

	roleRead, err := r.client.Update(ctx, plan)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to update role",
			fmt.Errorf("unable to update role: %w", err).Error(),
		)
	}

	response.Diagnostics.Append(response.State.Set(ctx, roleRead)...)
}

func (r *RoleResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var model roleModel
	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, model.Key.ValueString(), model.Resource.ValueStringPointer())

	if err != nil {
		response.Diagnostics.AddError(
			"Failed deleting relation",
			fmt.Errorf("unable to delete role %s: %w", model.Key.ValueString(), err).Error(),
		)
		return
	}
}
