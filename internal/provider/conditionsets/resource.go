package conditionsets

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &conditionSetResource{}
	_ resource.ResourceWithConfigure = &conditionSetResource{}
)

func NewResourceSetResource() resource.Resource {
	return &ResourceSetResource{conditionSetResource{conditionSetType: models.RESOURCESET}}
}

func NewUserSetResource() resource.Resource {
	return &UserSetResource{conditionSetResource{conditionSetType: models.USERSET}}
}

type conditionSetResource struct {
	client           ConditionSetClient
	conditionSetType models.ConditionSetType
}

type UserSetResource struct {
	conditionSetResource
}

type ResourceSetResource struct {
	conditionSetResource
}

func (c *UserSetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_set"
}

func (c *ResourceSetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource_set"
}

func (c *conditionSetResource) Configure(_ context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
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

	c.client = ConditionSetClient{client: permitClient}
}

func (c *conditionSetResource) Metadata(_ context.Context, _ resource.MetadataRequest, _ *resource.MetadataResponse) {
	// should be completely implemented in ResourceSet/UserSet
	panic("not implemented")
}

func (c *ResourceSetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := c.baseAttributes()
	attributes["resource"] = schema.StringAttribute{
		Required: true,
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "See the [our documentation](https://api.permit.io/v2/redoc#tag/Condition-Sets/operation/create_condition_set) for more information on condition sets.",
		Attributes:          attributes,
	}
}

func (c *UserSetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := c.baseAttributes()

	resp.Schema = schema.Schema{
		MarkdownDescription: "See the [our documentation](https://api.permit.io/v2/redoc#tag/Condition-Sets/operation/create_condition_set) for more information on condition sets.",
		Attributes:          attributes,
	}
}

func (c *conditionSetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	panic("not implemented")
}

func (c *conditionSetResource) baseAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "A unique id by which Permit will identify the condition set. The key will be used as the generated rego rule name.\n\n",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"organization_id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The id of the organization to which the condition set belongs.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"project_id": schema.StringAttribute{
			MarkdownDescription: "The id of the project to which the condition set belongs.",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"environment_id": schema.StringAttribute{
			MarkdownDescription: "The id of the environment to which the condition set belongs.",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"key": schema.StringAttribute{
			MarkdownDescription: "A unique id by which Permit will identify the condition set. The key will be used as the generated rego rule name.",
			Required:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "A descriptive name for the set, i.e: 'US based employees' or 'Users behind VPN'",
			Required:            true,
		},
		"description": schema.StringAttribute{
			MarkdownDescription: "an optional longer description of the set",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"conditions": schema.StringAttribute{
			MarkdownDescription: "a boolean expression that consists of multiple conditions, with and/or logic.",
			Required:            true,
		},
		"resource": schema.StringAttribute{
			MarkdownDescription: "The resource id to which the condition set applies. This is only required for resource sets.",
			Optional:            true,
		},
		"parent_id": schema.StringAttribute{
			MarkdownDescription: "The parent condition set id. Allows creating a nested condition set hierarchy.",
			Optional:            true,
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
	}
}

func (c *conditionSetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var (
		plan ConditionSetModel
	)

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.client.Create(ctx, c.conditionSetType, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create resource",
			fmt.Sprintf("Unable to create resource: %s", err),
		)
		return
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (c *conditionSetResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var data ConditionSetModel

	response.Diagnostics.Append(request.State.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	state, err := c.client.Read(ctx, data)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to Read Condition Set",
			fmt.Sprintf("Unable to read condition set: %s, Error: %s", data.Id.String(), err.Error()),
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
func (c *conditionSetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var (
		plan ConditionSetModel
	)

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.client.Update(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Unable to update resource",
			fmt.Sprintf("Unable to update resource: %s", err),
		)
		return
	}
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (c *conditionSetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state ConditionSetModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := c.client.Delete(ctx, state.Key.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Condition Set",
			"Could not delete resource, unexpected error: "+err.Error(),
		)
		return
	}
}
