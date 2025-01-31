package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &ResourceResource{}
	_ resource.ResourceWithConfigure = &ResourceResource{}
)

// NewResourceResource is a helper function to simplify the provider implementation.
func NewResourceResource() resource.Resource {
	return &ResourceResource{}
}

// ResourceResource is the resource implementation.
type ResourceResource struct {
	ResourceClient
}

func (r *ResourceResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
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
func (r *ResourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource"
}

// Schema defines the schema for the resource.
func (r *ResourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "See [the documentation](https://api.permit.io/v2/redoc#tag/Resources/operation/create_resource) for more information about resources.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the resource",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the organization that owns the resource",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the project that owns the resource",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"environment_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the environment that owns the resource",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Timestamp when the resource was created",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Timestamp when the resource was last updated",
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A URL-friendly name of the resource (i.e: slug). You will be able to query later using this key instead of the id (UUID) of the resource.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the resource",
			},
			"urn": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The URN (Uniform Resource Name) of the resource",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "An optional longer description of what this resource respresents in your system",
			},
			"actions": schema.MapNestedAttribute{
				MarkdownDescription: "A actions definition block, typically contained within a resource type definition block.\n    The actions represents the ways you can interact with a protected resource.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"name": schema.StringAttribute{
							Required: true,
						},
						"description": schema.StringAttribute{
							Optional: true,
						},
					},
				},
				Required: true,
			},
			"attributes": schema.MapNestedAttribute{
				MarkdownDescription: "Attributes that each resource of this type defines, and can be used in your ABAC policies.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								common.AttributeTypeValidator{},
							},
						},
						"description": schema.StringAttribute{
							Optional: true,
						},
					},
				},
				Optional: true,
			},
		},
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *ResourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var (
		resourcePlan ResourceModel
	)

	diags := req.Plan.Get(ctx, &resourcePlan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.ResourceCreate(ctx, &resourcePlan); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create resource",
			fmt.Sprintf("Unable to create resource: %s", err),
		)
		return
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, resourcePlan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *ResourceResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var data ResourceModel

	response.Diagnostics.Append(request.State.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	state, err := r.ResourceRead(ctx, data)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to Read Resource",
			fmt.Sprintf("Unable to read resource: %s, Error: %s", data.Id.String(), err.Error()),
		)
		return
	}

	// Set state
	diags := response.State.Set(ctx, &state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *ResourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var (
		resourcePlan ResourceModel
	)
	diags := req.Plan.Get(ctx, &resourcePlan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Info(ctx, fmt.Sprintf("update %v", resourcePlan.Actions))

	if err := r.ResourceUpdate(ctx, &resourcePlan); err != nil {
		resp.Diagnostics.AddError(
			"Unable to update resource",
			fmt.Sprintf("Unable to update resource: %s", err),
		)
		return
	}
	diags = resp.State.Set(ctx, resourcePlan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *ResourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state ResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Api.Resources.Delete(ctx, state.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Resource",
			"Could not delete resource, unexpected error: "+err.Error(),
		)
		return
	}

}
