package group_resource_instance_role_assignments

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/config"
)

var (
	_ resource.Resource                = &GroupResourceInstanceRoleAssignmentResource{}
	_ resource.ResourceWithConfigure   = &GroupResourceInstanceRoleAssignmentResource{}
	_ resource.ResourceWithImportState = &GroupResourceInstanceRoleAssignmentResource{}
)

func NewGroupResourceInstanceRoleAssignmentResource() resource.Resource {
	return &GroupResourceInstanceRoleAssignmentResource{}
}

type GroupResourceInstanceRoleAssignmentResource struct {
	client groupResourceInstanceRoleAssignmentClient
}

func (r *GroupResourceInstanceRoleAssignmentResource) Configure(_ context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	if _, ok := request.ProviderData.(*permit.Client); !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *permit.Client, got: %T.", request.ProviderData),
		)
		return
	}

	// The requests go through the connection the provider's Configure stored with
	// the SDK client, not through the SDK.
	r.client = groupResourceInstanceRoleAssignmentClient{api: config.GetAPI()}
}

func (r *GroupResourceInstanceRoleAssignmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_resource_instance_role_assignment"
}

func (r *GroupResourceInstanceRoleAssignmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a role to a group on a specific resource instance within a tenant. " +
			"This uses the Permit.io Groups API to manage group-level permissions on resource instances. " +
			"For user-specific assignments, use `permitio_resource_instance_role_assignment` " +
			"instead. Every argument forces replacement: changing one removes the assignment and " +
			"creates a new one.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The key of the group, the same as `group`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"group": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Group key to assign the role to",
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
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Tenant key for scoped assignment",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *GroupResourceInstanceRoleAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan GroupResourceInstanceRoleAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Create(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create group resource instance role assignment",
			common.APIErrorDetail("create", "group resource instance role assignment",
				assignmentID(plan), err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *GroupResourceInstanceRoleAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data GroupResourceInstanceRoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, err := r.client.Read(ctx, data)
	if err != nil {
		if common.IsNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to read group resource instance role assignment",
			common.APIErrorDetail("read", "group resource instance role assignment",
				assignmentID(data), err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupResourceInstanceRoleAssignmentResource) Update(
	_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	common.AddReplaceOnlyUpdateError(&resp.Diagnostics, "group resource instance role assignment")
}

func (r *GroupResourceInstanceRoleAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GroupResourceInstanceRoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Delete(ctx, &state); err != nil && !common.IsNotFoundErr(err) {
		resp.Diagnostics.AddError(
			"Error deleting group resource instance role assignment",
			common.APIErrorDetail("delete", "group resource instance role assignment",
				assignmentID(state), err),
		)
	}
}

// assignmentID names an assignment in error messages the way its import ID does,
// as group:role:resource:resource_instance:tenant.
func assignmentID(model GroupResourceInstanceRoleAssignmentModel) string {
	return strings.Join([]string{
		model.Group.ValueString(), model.Role.ValueString(), model.Resource.ValueString(),
		model.ResourceInstance.ValueString(), model.Tenant.ValueString(),
	}, ":")
}

// ImportState imports an assignment by the keys of its group, role, resource,
// resource instance and tenant.
func (r *GroupResourceInstanceRoleAssignmentResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	common.ImportState(ctx, "group:role:resource:resource_instance:tenant", req, resp)
}
