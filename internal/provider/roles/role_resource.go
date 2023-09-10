package roles

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/permit"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &RoleResource{}
	_ resource.ResourceWithConfigure = &RoleResource{}
)

// NewRoleResource is a helper function to simplify the provider implementation.
func NewRoleResource() resource.Resource {
	return &RoleResource{}
}

// RoleResource is the resource implementation.
type RoleResource struct {
	RoleClient
}

func (r *RoleResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}
	client, ok := request.ProviderData.(*permit.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *permit.Client, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)
		return
	}
	r.client = client
}

// Metadata returns the resource type name.
func (r *RoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

// Schema defines the schema for the resource.
func (r *RoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"environment_id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"key": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"permissions": schema.ListAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Optional:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"extends": schema.ListAttribute{
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				Computed: true,
				Optional: true,
			},
		},
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *RoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var (
		rolePlan RoleModel
	)
	diags := req.Plan.Get(ctx, &rolePlan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	err, diags := r.RoleCreate(ctx, &rolePlan)
	if err != nil || diags.HasError() {
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to create role",
				fmt.Sprintf("Unable to create role: %s", err),
			)
		} else {
			resp.Diagnostics.Append(diags...)
		}
		return
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, rolePlan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *RoleResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var (
		data RoleModel
	)

	response.Diagnostics.Append(request.State.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}
	state, err, diags := r.RoleRead(ctx, data)
	if err != nil || diags.HasError() {
		if err != nil {
			response.Diagnostics.AddError(
				"Unable to create role",
				fmt.Sprintf("Unable to create role: %s", err),
			)
		} else {
			response.Diagnostics.Append(diags...)
		}
		return
	}

	// Set state
	diags = response.State.Set(ctx, &state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *RoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var (
		resourcePlan RoleModel
	)
	diags := req.Plan.Get(ctx, &resourcePlan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err, diags := r.RoleUpdate(ctx, &resourcePlan)
	if err != nil || diags.HasError() {
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to create role",
				fmt.Sprintf("Unable to create role: %s", err),
			)
		} else {
			resp.Diagnostics.Append(diags...)
		}
		return
	}

	diags = resp.State.Set(ctx, resourcePlan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *RoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RoleModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Api.Roles.Delete(ctx, state.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Role",
			"Could not delete role, unexpected error: "+err.Error(),
		)
		return
	}

}
