package resource_instances

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
	"strings"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &ResourceInstanceResource{}
	_ resource.ResourceWithConfigure   = &ResourceInstanceResource{}
	_ resource.ResourceWithImportState = &ResourceInstanceResource{}
)

func NewResourceInstanceResource() resource.Resource {
	return &ResourceInstanceResource{}
}

type ResourceInstanceResource struct {
	client resourceInstanceClient
}

func (r *ResourceInstanceResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	permitClient := common.Configure(ctx, request, response)
	r.client = resourceInstanceClient{client: permitClient}
}

func (r *ResourceInstanceResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_resource_instance"
}

func (r *ResourceInstanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := common.CreateBaseResourceSchema()
	delete(attributes, "name")
	delete(attributes, "description")

	attributes["resource"] = schema.StringAttribute{
		MarkdownDescription: "The resource type key that this instance belongs to.",
		Required:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["resource_id"] = schema.StringAttribute{
		MarkdownDescription: "The unique resource type ID.",
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseNonNullStateForUnknown(),
		},
	}
	attributes["tenant"] = schema.StringAttribute{
		MarkdownDescription: "The tenant key for multi-tenant enforcement.",
		Optional:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["attributes"] = schema.StringAttribute{
		MarkdownDescription: "Arbitrary resource instance attributes in JSON format that will be used to enforce attribute-based access control policies.",
		Optional:            true,
		Computed:            true,
	}

	resp.Schema = schema.Schema{
		Attributes:          attributes,
		MarkdownDescription: "Manages a Permit.io resource instance. Resource instances represent specific objects of a resource type (e.g., a specific document, project, or folder). See [the documentation](https://api.permit.io/v2/redoc#tag/Resource-Instances) for more information.",
	}
}

func (r *ResourceInstanceResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var plan resourceInstanceModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)

	if response.Diagnostics.HasError() {
		return
	}

	instanceRead, err := r.client.Create(ctx, plan)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to create resource instance",
			common.APIErrorDetail("create", "resource instance", instanceID(plan), err),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, instanceRead)...)
}

func (r *ResourceInstanceResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var model resourceInstanceModel

	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	instanceRead, err := r.client.Read(ctx, model.Key.ValueString(), model.Resource.ValueString())

	if err != nil {
		if common.IsNotFoundErr(err) {
			response.State.RemoveResource(ctx)
			return
		}
		response.Diagnostics.AddError(
			"Unable to read resource instance",
			common.APIErrorDetail("read", "resource instance", instanceID(model), err),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, &instanceRead)...)
}

func (r *ResourceInstanceResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	var plan resourceInstanceModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)

	if response.Diagnostics.HasError() {
		return
	}

	instanceRead, err := r.client.Update(ctx, plan)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to update resource instance",
			common.APIErrorDetail("update", "resource instance", instanceID(plan), err),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, instanceRead)...)
}

func (r *ResourceInstanceResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var model resourceInstanceModel
	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, model.Key.ValueString(), model.Resource.ValueString())

	if err != nil && !common.IsNotFoundErr(err) {
		response.Diagnostics.AddError(
			"Unable to delete resource instance",
			common.APIErrorDetail("delete", "resource instance", instanceID(model), err),
		)
		return
	}
}

// instanceID names a resource instance in error messages the way its import ID
// does, as resource:key.
func instanceID(model resourceInstanceModel) string {
	return model.Resource.ValueString() + ":" + model.Key.ValueString()
}

// ImportState implements resource.ResourceWithImportState.
func (r *ResourceInstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: resource_key:instance_key
	idParts := strings.SplitN(req.ID, ":", 2)

	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID Format",
			"Expected format: resource_key:instance_key\n\n"+
				"Example: terraform import permitio_resource_instance.example \"document:doc-123\"",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), idParts[1])...)
}
