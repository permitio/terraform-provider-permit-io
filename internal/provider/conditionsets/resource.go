package conditionsets

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &UserSetResource{}
	_ resource.ResourceWithConfigure   = &UserSetResource{}
	_ resource.Resource                = &ResourceSetResource{}
	_ resource.ResourceWithConfigure   = &ResourceSetResource{}
	_ resource.ResourceWithModifyPlan  = &ResourceSetResource{}
	_ resource.ResourceWithImportState = &UserSetResource{}
	_ resource.ResourceWithImportState = &ResourceSetResource{}
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

// ModifyPlan replaces a resource set when its resource changes, as the API cannot
// move a set to another resource. The configuration may name the resource by ID
// or by key, so a change between the ID and the key of the set's own resource
// updates the set in place: a replacement would also delete the rules on the set.
func (c *ResourceSetResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var key, prior, planned types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("key"), &key)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("resource"), &prior)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("resource"), &planned)...)
	if resp.Diagnostics.HasError() || planned.Equal(prior) {
		return
	}
	if planned.IsUnknown() || c.client.client == nil {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("resource"))
		return
	}
	same, err := c.client.NamesSetResource(ctx, key.ValueString(), planned.ValueString())
	if err != nil && !common.IsNotFoundErr(err) {
		resp.Diagnostics.AddError(
			"Unable to read resource set",
			common.APIErrorDetail("read", "resource set", key.ValueString(), err),
		)
		return
	}
	if !same {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("resource"))
	}
}

func (c *UserSetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := c.baseAttributes()

	resp.Schema = schema.Schema{
		MarkdownDescription: "See the [our documentation](https://api.permit.io/v2/redoc#tag/Condition-Sets/operation/create_condition_set) for more information on condition sets.",
		Attributes:          attributes,
	}
}

func (c *conditionSetResource) baseAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "A unique id by which Permit will identify the condition set. The key will be used as the generated rego rule name.\n\n",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseNonNullStateForUnknown(),
			},
		},
		"organization_id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The id of the organization to which the condition set belongs.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseNonNullStateForUnknown(),
			},
		},
		"project_id": schema.StringAttribute{
			MarkdownDescription: "The id of the project to which the condition set belongs.",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseNonNullStateForUnknown(),
			},
		},
		"environment_id": schema.StringAttribute{
			MarkdownDescription: "The id of the environment to which the condition set belongs.",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseNonNullStateForUnknown(),
			},
		},
		"key": schema.StringAttribute{
			MarkdownDescription: "A unique id by which Permit will identify the condition set. The key will be used as the generated rego rule name.",
			Required:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "A descriptive name for the set, i.e: 'US based employees' or 'Users behind VPN'",
			Required:            true,
		},
		"description": schema.StringAttribute{
			MarkdownDescription: "an optional longer description of the set. Removing it clears the description in Permit.",
			Optional:            true,
		},
		"conditions": schema.StringAttribute{
			MarkdownDescription: "a boolean expression that consists of multiple conditions, with and/or logic. A JSON object, such as `jsonencode({ allOf = [...] })`. Differences in whitespace and key order from the object Permit returns do not show as changes.",
			Required:            true,
			CustomType:          jsontypes.NormalizedType{},
		},
		"parent_id": schema.StringAttribute{
			MarkdownDescription: "The parent condition set id. Allows creating a nested condition set hierarchy. A plan that removes it from a set that has a parent fails: the provider cannot detach a set from its parent in place. To remove it from such a set, run `terraform taint` on the set and apply: Terraform replaces the set, which deletes its condition set rules. `terraform apply -replace` fails with the same error.",
			Optional:            true,
			PlanModifiers: []planmodifier.String{
				refuseParentRemoval{},
			},
		},
	}
}

// refuseParentRemoval is the plan modifier of parent_id that fails a plan that
// removes the parent of a set that has one. The API detaches a set only when an
// update sends parent_id as null, and the Go SDK leaves a null parent_id out of
// the request, so the apply would keep the parent (PER-16604).
type refuseParentRemoval struct{}

func (refuseParentRemoval) Description(context.Context) string {
	return "Fails a plan that removes the parent of a condition set that has one."
}

func (m refuseParentRemoval) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (refuseParentRemoval) PlanModifyString(_ context.Context, req planmodifier.StringRequest,
	resp *planmodifier.StringResponse,
) {
	if req.Plan.Raw.IsNull() || req.StateValue.IsNull() || !req.PlanValue.IsNull() {
		return
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Cannot remove the parent of a condition set",
		fmt.Sprintf("The condition set has the parent %s. The Permit API detaches a set "+
			"from its parent only when an update sends parent_id as null, which the "+
			"Permit Go SDK the provider uses cannot send, so an apply would keep the "+
			"parent. Keep parent_id in the configuration, detach the set outside "+
			"Terraform and then remove parent_id, or mark the set for replacement with "+
			"terraform taint and then apply, which creates it again without a parent; "+
			"Permit deletes the set's condition set rules when it deletes the set. "+
			"terraform apply -replace does not help: its plan still starts from the "+
			"set that has the parent, and fails with this error.", req.StateValue))
}

// modelSource is a plan or a state, which get reads a condition set from.
type modelSource interface {
	Get(ctx context.Context, target any) diag.Diagnostics
}

// get reads the attributes of this type of condition set from source into model.
// A user set has no resource attribute, so it leaves model.Resource null.
func (c *conditionSetResource) get(ctx context.Context, source modelSource,
	model *ConditionSetModel,
) diag.Diagnostics {
	if c.conditionSetType == models.USERSET {
		return source.Get(ctx, &model.userSetModel)
	}
	return source.Get(ctx, model)
}

// set writes the attributes of this type of condition set from model into state.
func (c *conditionSetResource) set(ctx context.Context, state *tfsdk.State,
	model ConditionSetModel,
) diag.Diagnostics {
	if c.conditionSetType == models.USERSET {
		return state.Set(ctx, model.userSetModel)
	}
	return state.Set(ctx, model)
}

func (c *conditionSetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var (
		plan ConditionSetModel
	)

	diags := c.get(ctx, req.Plan, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.client.Create(ctx, c.conditionSetType, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create condition set",
			common.APIErrorDetail("create", "condition set", plan.Key.ValueString(), err),
		)
		return
	}

	// Set state to fully populated data
	diags = c.set(ctx, &resp.State, plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (c *conditionSetResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var data ConditionSetModel

	response.Diagnostics.Append(c.get(ctx, request.State, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	state, err := c.client.Read(ctx, c.conditionSetType, data)

	if err != nil {
		if common.IsNotFoundErr(err) {
			response.State.RemoveResource(ctx)
			return
		}
		var wrongType wrongTypeError
		if errors.As(err, &wrongType) {
			response.Diagnostics.AddError("Condition set of another type", wrongType.Error())
			return
		}
		response.Diagnostics.AddError(
			"Unable to Read Condition Set",
			common.APIErrorDetail("read", "condition set", data.Key.ValueString(), err),
		)
		return
	}

	// Set state
	diags := c.set(ctx, &response.State, state)
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

	diags := c.get(ctx, req.Plan, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := c.client.Update(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Unable to update condition set",
			common.APIErrorDetail("update", "condition set", plan.Key.ValueString(), err),
		)
		return
	}
	diags = c.set(ctx, &resp.State, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (c *conditionSetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state ConditionSetModel
	diags := c.get(ctx, req.State, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := c.client.Delete(ctx, state.Key.ValueString())

	if err != nil && !common.IsNotFoundErr(err) {
		resp.Diagnostics.AddError(
			"Error Deleting Condition Set",
			common.APIErrorDetail("delete", "condition set", state.Key.ValueString(), err),
		)
		return
	}
}

// ImportState imports a user set or a resource set by its key. An imported resource
// set names its resource by key. Read fails on a set of the other type, so a
// resource set cannot be imported as a user set, or the reverse.
func (c *conditionSetResource) ImportState(
	ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse,
) {
	common.ImportState(ctx, "key", request, response)
}
