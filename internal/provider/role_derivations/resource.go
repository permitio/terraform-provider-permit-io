package role_derivations

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &RoleDerivationResource{}
	_ resource.ResourceWithConfigure = &RoleDerivationResource{}
)

func NewRoleDerivationResource() resource.Resource {
	return &RoleDerivationResource{}
}

type RoleDerivationResource struct {
	client apiClient
}

func (r *RoleDerivationResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	permitClient := common.Configure(ctx, request, response)
	r.client = apiClient{client: permitClient}
}

func (r *RoleDerivationResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_role_derivation"
}

func (r *RoleDerivationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := make(map[string]schema.Attribute)
	attributes["resource"] = schema.StringAttribute{
		MarkdownDescription: "Either the unique id of the resource, or the URL-friendly key of the resource that you want to create role derivation for.",
		Required:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["role"] = schema.StringAttribute{
		MarkdownDescription: "The role that the user will derive.",
		Required:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["on_resource"] = schema.StringAttribute{
		MarkdownDescription: "The resource that the user will derive the role on.",
		Required:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["to_role"] = schema.StringAttribute{
		MarkdownDescription: "The role that you want to create role derivation for.",
		Required:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["linked_by"] = schema.StringAttribute{
		MarkdownDescription: "The relation that links the resource to the role.",
		Required:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}

	resp.Schema = schema.Schema{
		Attributes:          attributes,
		MarkdownDescription: "See [the documentation](https://api.permit.io/v2/redoc#tag/Implicit-Grants/operation/create_implicit_grant) for more information on role derivations.",
	}
}

func (r *RoleDerivationResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var plan roleDerivationModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)

	if response.Diagnostics.HasError() {
		return
	}

	roleRead, err := r.client.Create(ctx, plan)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to create role derivation",
			fmt.Errorf("unable to create role derivation: %w", err).Error(),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, roleRead)...)
}

func (r *RoleDerivationResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var model roleDerivationModel

	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	reality, err := r.client.Read(ctx, model)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read role derivation",
			fmt.Errorf("unable to read role derivation: %w", err).Error(),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, &reality)...)
}

func (r *RoleDerivationResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	panic("")
}

func (r *RoleDerivationResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var model roleDerivationModel
	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, model)

	if err != nil {
		response.Diagnostics.AddError(
			"Failed deleting role derivation",
			fmt.Errorf("unable to delete role derivation: %w", err).Error(),
		)
	}
}
