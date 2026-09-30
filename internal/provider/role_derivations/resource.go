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
		MarkdownDescription: "The key or ID of the resource that `to_role` belongs to. " +
			"Users get `to_role` on instances of this resource.",
		Required: true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["role"] = schema.StringAttribute{
		MarkdownDescription: "The key of a role on `on_resource`. Users who have this role on an " +
			"`on_resource` instance get `to_role` on the linked `resource` instances.",
		Required: true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["on_resource"] = schema.StringAttribute{
		MarkdownDescription: "The key of the related resource that `role` belongs to.",
		Required:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["to_role"] = schema.StringAttribute{
		MarkdownDescription: "The key of the role on `resource` that users get through this derivation.",
		Required:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["linked_by"] = schema.StringAttribute{
		MarkdownDescription: "The key of the relation that links `on_resource` instances to " +
			"`resource` instances.",
		Required: true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}

	resp.Schema = schema.Schema{
		Attributes: attributes,
		MarkdownDescription: "Grants `to_role` on `resource` to every user who has `role` on an " +
			"`on_resource` instance linked by the `linked_by` relation. For example, " +
			"`role = \"manager\"`, `on_resource = \"folder\"`, `to_role = \"editor\"`, " +
			"`resource = \"file\"` and `linked_by = \"parent\"` make folder managers editors of " +
			"the files in their folders. See [the documentation](" +
			"https://api.permit.io/v2/redoc#tag/Implicit-Grants/operation/create_implicit_grant) " +
			"for more information on role derivations.",
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
		detail := common.APIErrorDetail("create", "role derivation", derivationID(plan), err)
		if common.IsNotFoundErr(err) {
			detail += fmt.Sprintf(
				"\n\nCheck that to_role %q is a role on resource %q, role %q is a role on "+
					"on_resource %q, and linked_by %q is a relation between the two resources.",
				plan.ToRole.ValueString(), plan.Resource.ValueString(), plan.Role.ValueString(),
				plan.OnResource.ValueString(), plan.LinkedByRelation.ValueString(),
			)
		}
		response.Diagnostics.AddError("Unable to create role derivation", detail)
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
		// The derivation or its role was deleted outside Terraform; drop it so it is recreated.
		if common.IsNotFoundErr(err) {
			response.State.RemoveResource(ctx)
			return
		}
		response.Diagnostics.AddError(
			"Unable to read role derivation",
			common.APIErrorDetail("read", "role derivation", derivationID(model), err),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, &reality)...)
}

func (r *RoleDerivationResource) Update(
	_ context.Context, _ resource.UpdateRequest, response *resource.UpdateResponse,
) {
	common.AddReplaceOnlyUpdateError(&response.Diagnostics, "role derivation")
}

func (r *RoleDerivationResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var model roleDerivationModel
	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, model)

	if err != nil && !common.IsNotFoundErr(err) {
		response.Diagnostics.AddError(
			"Failed deleting role derivation",
			common.APIErrorDetail("delete", "role derivation", derivationID(model), err),
		)
	}
}

// derivationID names a role derivation in error messages by the role it derives
// from and the role it grants, each as resource:role.
func derivationID(model roleDerivationModel) string {
	return fmt.Sprintf("%s:%s to %s:%s",
		model.OnResource.ValueString(), model.Role.ValueString(),
		model.Resource.ValueString(), model.ToRole.ValueString())
}
