package resource_instance_role_assignments

import (
	"context"
	"fmt"
	"strings"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/permitio/permit-golang/pkg/permit"
)

var (
	_ resource.Resource                = &ResourceInstanceRoleAssignmentResource{}
	_ resource.ResourceWithConfigure   = &ResourceInstanceRoleAssignmentResource{}
	_ resource.ResourceWithImportState = &ResourceInstanceRoleAssignmentResource{}
)

func NewResourceInstanceRoleAssignmentResource() resource.Resource {
	return &ResourceInstanceRoleAssignmentResource{}
}

type ResourceInstanceRoleAssignmentResource struct {
	client resourceInstanceRoleAssignmentClient
}

func (r *ResourceInstanceRoleAssignmentResource) Configure(_ context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}
	permitClient, ok := request.ProviderData.(*permit.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *permit.Client, got: %T.", request.ProviderData),
		)
		return
	}
	r.client = resourceInstanceRoleAssignmentClient{client: permitClient}
}

func (r *ResourceInstanceRoleAssignmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource_instance_role_assignment"
}

func (r *ResourceInstanceRoleAssignmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a role to a user on a specific resource instance within a tenant. " +
			"This is for instance-level permissions (e.g., giving a user editor access to a specific document). " +
			"For tenant-level role assignments, use `permitio_role_assignment` instead.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique identifier of the role assignment",
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
			"user": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "User key to assign the role to",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Role key to assign",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Tenant key for scoped assignment",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Resource type (e.g., 'workspace', 'document')",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource_instance": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Resource instance key (e.g., 'ws-123', 'doc-456')",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (r *ResourceInstanceRoleAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceInstanceRoleAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Create(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create resource instance role assignment",
			fmt.Sprintf("Unable to assign role %s to user %s on resource %s instance %s in tenant %s: %s",
				plan.Role.ValueString(), plan.User.ValueString(), plan.Resource.ValueString(), plan.ResourceInstance.ValueString(), plan.Tenant.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ResourceInstanceRoleAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ResourceInstanceRoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, err := r.client.Read(ctx, data)
	if err != nil {
		// If the resource is not found, remove it from state (drift detection)
		if strings.Contains(err.Error(), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to read resource instance role assignment",
			fmt.Sprintf("Unable to read resource instance role assignment: %s", err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ResourceInstanceRoleAssignmentResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	panic("updating ResourceInstanceRoleAssignments is not implemented")
}

func (r *ResourceInstanceRoleAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceInstanceRoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Delete(ctx, &state); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting resource instance role assignment",
			fmt.Sprintf("Could not unassign role %s from user %s on resource %s instance %s in tenant %s: %s",
				state.Role.ValueString(), state.User.ValueString(), state.Resource.ValueString(), state.ResourceInstance.ValueString(), state.Tenant.ValueString(), err.Error()),
		)
	}
}

func (r *ResourceInstanceRoleAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Format: user:role:resource:resource_instance:tenant
	parts := strings.Split(req.ID, ":")
	if len(parts) != 5 {
		resp.Diagnostics.AddError(
			"Invalid import ID format",
			"Expected format: user:role:resource:resource_instance:tenant\n\n"+
				"Example: terraform import permitio_resource_instance_role_assignment.example \"user@example.com:editor:document:doc-123:default\"",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource_instance"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tenant"), parts[4])...)
}
