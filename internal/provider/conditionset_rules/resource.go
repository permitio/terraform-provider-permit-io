package conditionsetrules

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/permitio/permit-golang/pkg/permit"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &ConditionSetRuleResource{}
	_ resource.ResourceWithConfigure = &ConditionSetRuleResource{}
)

func NewConditionSetRuleResource() resource.Resource {
	return &ConditionSetRuleResource{}
}

type ConditionSetRuleResource struct {
	client ConditionSetRuleClient
}

func (c *ConditionSetRuleResource) Configure(_ context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
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

	c.client = ConditionSetRuleClient{client: permitClient}
}

func (c *ConditionSetRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	// should be completely implemented in ResourceSet/UserSet
	resp.TypeName = req.ProviderTypeName + "_condition_set_rule"
}

func (c *ConditionSetRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "See [our documentation](https://api.permit.io/v2/redoc#tag/Condition-Set-Rules) for more information on condition sets rules.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the condition set rule",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the organization that owns the condition set rule",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the project that owns the condition set rule",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"environment_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the environment that owns the condition set rule",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_set": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The userset that will be given permission, i.e: all the users matching this rule will be given the specified permission",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"permission": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The permission that will be granted to the userset on the resourceset. The permission can be either a resource action id, or {resource_key}:{action_key}, i.e: the \"permission name\".",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource_set": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The resourceset that represents the resources that are granted for access, i.e: all the resources matching this rule can be accessed by the userset to perform the granted permission",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (c *ConditionSetRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var (
		plan ConditionSetRuleModel
	)

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.client.Create(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create condition set rule",
			fmt.Sprintf("Unable to create condition set rule: %s", err),
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

func (c *ConditionSetRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ConditionSetRuleModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, err := c.client.Read(ctx, data)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Condition Set Rule",
			fmt.Sprintf("Unable to read condition set rule: %s, Error: %s", data.Id.String(), err.Error()),
		)
		return
	}

	// Set state
	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (c *ConditionSetRuleResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// rules cannot be updated, only replaced - this should never be called
	panic("updating ConditionSetRules is not implemented")
}

// Delete deletes the resource and removes the Terraform state on success.
func (c *ConditionSetRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state ConditionSetRuleModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := c.client.Delete(ctx, &state)

	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Condition Set Rule",
			"Could not delete condition set rule, unexpected error: "+err.Error(),
		)
		return
	}
}
