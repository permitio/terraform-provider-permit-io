package conditionsetrules

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
	"strings"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &ConditionSetRuleResource{}
	_ resource.ResourceWithConfigure   = &ConditionSetRuleResource{}
	_ resource.ResourceWithImportState = &ConditionSetRuleResource{}
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
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the organization that owns the condition set rule",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the project that owns the condition set rule",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
				},
			},
			"environment_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique id of the environment that owns the condition set rule",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseNonNullStateForUnknown(),
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
			common.APIErrorDetail("create", "condition set rule", ruleID(plan), err),
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
		// If the rule no longer exists in Permit, drop it from state so it is recreated.
		if common.IsNotFoundErr(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to Read Condition Set Rule",
			common.APIErrorDetail("read", "condition set rule", ruleID(data), err),
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

// Update reports an error: every attribute of a rule forces replacement, so
// Terraform never calls it.
func (c *ConditionSetRuleResource) Update(
	_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse,
) {
	common.AddReplaceOnlyUpdateError(&resp.Diagnostics, "condition set rule")
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

	if err != nil && !common.IsNotFoundErr(err) {
		resp.Diagnostics.AddError(
			"Error Deleting Condition Set Rule",
			common.APIErrorDetail("delete", "condition set rule", ruleID(state), err),
		)
		return
	}
}

// ruleID names a condition set rule in error messages the way its import ID does,
// as user_set,permission,resource_set.
func ruleID(model ConditionSetRuleModel) string {
	return strings.Join([]string{
		model.UserSet.ValueString(),
		model.Permission.ValueString(),
		model.ResourceSet.ValueString(),
	}, ",")
}

// ImportState implements resource.ResourceWithImportState.
func (c *ConditionSetRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expected format: user_set,permission,resource_set
	idParts := strings.Split(req.ID, ",")

	if len(idParts) != 3 {
		resp.Diagnostics.AddError(
			"Invalid Import ID Format",
			fmt.Sprintf("Expected import ID format: 'user_set,permission,resource_set'. Got %d parts, expected 3.\n\n"+
				"Example: terraform import permitio_condition_set_rule.example \"admins,document:read,sensitive-docs\"",
				len(idParts)),
		)
		return
	}

	userSet := strings.TrimSpace(idParts[0])
	permission := strings.TrimSpace(idParts[1])
	resourceSet := strings.TrimSpace(idParts[2])

	if userSet == "" || permission == "" || resourceSet == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID Format",
			"Import ID contains empty values. All three parts must be non-empty.\n\n"+
				fmt.Sprintf("Got: user_set='%s', permission='%s', resource_set='%s'\n\n"+
					"Example: terraform import permitio_condition_set_rule.example \"admins,document:read,sensitive-docs\"",
					userSet, permission, resourceSet),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_set"), userSet)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("permission"), permission)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource_set"), resourceSet)...)
}
