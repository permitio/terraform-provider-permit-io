package resource_instance_role_assignments

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
	"strings"
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
			"For tenant-level role assignments, use `permitio_role_assignment` instead. " +
			"Every argument forces replacement: changing one removes the assignment and " +
			"creates a new one.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique identifier of the role assignment",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the organization the role assignment belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the project the role assignment belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"environment_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the environment the role assignment belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"user": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The key of the user to assign the role to. " +
					common.UseKeyNote,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The key of the role to assign, a role of `resource`. " +
					common.KeyOnlyNote,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{common.KeyNotID("permitio_role")},
			},
			"tenant": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The key of the tenant the resource instance belongs to. " +
					common.UseKeyNote,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The key of the resource the instance belongs to, such as " +
					"`document`. " + common.KeyOnlyNote,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{common.KeyNotID("permitio_resource")},
			},
			"resource_instance": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The key of the resource instance, such as `handbook`. " +
					common.UseKeyNote,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the role assignment was created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
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
			common.APIErrorDetail("create", "resource instance role assignment",
				assignmentID(plan), err),
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
		if common.IsNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to read resource instance role assignment",
			common.APIErrorDetail("read", "resource instance role assignment",
				assignmentID(data), err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ResourceInstanceRoleAssignmentResource) Update(
	_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	common.AddReplaceOnlyUpdateError(&resp.Diagnostics, "resource instance role assignment")
}

func (r *ResourceInstanceRoleAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceInstanceRoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Delete(ctx, &state); err != nil && !common.IsNotFoundErr(err) {
		resp.Diagnostics.AddError(
			"Error deleting resource instance role assignment",
			common.APIErrorDetail("delete", "resource instance role assignment",
				assignmentID(state), err),
		)
	}
}

// assignmentID names an assignment in error messages the way its import ID does,
// as user:role:resource:resource_instance:tenant.
func assignmentID(model ResourceInstanceRoleAssignmentModel) string {
	return strings.Join([]string{
		model.User.ValueString(), model.Role.ValueString(), model.Resource.ValueString(),
		model.ResourceInstance.ValueString(), model.Tenant.ValueString(),
	}, ":")
}

// ImportState imports an assignment by the keys of its user, role, resource,
// resource instance and tenant.
func (r *ResourceInstanceRoleAssignmentResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	common.ImportState(ctx, "user:role:resource:resource_instance:tenant", req, resp)
}
