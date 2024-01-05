package relations

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &RelationResource{}
	_ resource.ResourceWithConfigure = &RelationResource{}
)

func NewRelationResource() resource.Resource {
	return &RelationResource{}
}

type RelationResource struct {
	client relationClient
}

func (c *RelationResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_relation"
}

func (c *RelationResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	permitClient := common.Configure(ctx, request, response)
	c.client = relationClient{client: permitClient}
}

func (c *RelationResource) Schema(_ context.Context, _ resource.SchemaRequest, response *resource.SchemaResponse) {
	attributes := common.CreateBaseResourceSchema()

	attributes["subject_resource"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The subject resource ID or key",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["object_resource"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The object resource ID or key",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}

	attributes["subject_resource_id"] = schema.StringAttribute{
		MarkdownDescription: "The subject resource ID",
		Computed:            true,
	}
	attributes["object_resource_id"] = schema.StringAttribute{
		MarkdownDescription: "The object resource ID",
		Computed:            true,
	}

	response.Schema = schema.Schema{
		Attributes:          attributes,
		MarkdownDescription: "See [the documentation](https://api.permit.io/v2/redoc#tag/Resource-Relations/operation/create_resource_relation) for more information about Relations",
	}
}

func (c *RelationResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var plan relationModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)

	if response.Diagnostics.HasError() {
		return
	}

	reality, err := c.client.Create(ctx, plan)

	if err != nil {
		response.Diagnostics.AddError(
			"Failed creating relation",
			fmt.Errorf("unable to create relation: %w", err).Error(),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, reality)...)
}

func (c *RelationResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var model relationModel

	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	reality, err := c.client.Read(ctx, model.ObjectResourceId.ValueString(), model.Key.ValueString())

	if err != nil {
		response.Diagnostics.AddError(
			"Failed reading relation",
			fmt.Errorf("unable to read relation %s/%s: %w", model.ObjectResourceId, model.Key, err).Error(),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, &reality)...)
}

func (c *RelationResource) Update(_ context.Context, _ resource.UpdateRequest, response *resource.UpdateResponse) {
	response.Diagnostics.AddError(
		"Unsupported operation",
		"resource relations must be replaced, and cannot be updated",
	)
}

func (c *RelationResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var model relationModel
	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	err := c.client.Delete(ctx, model.ObjectResource.ValueString(), model.Key.ValueString())

	if err != nil {
		response.Diagnostics.AddError(
			"Failed deleting relation",
			fmt.Errorf("unable to delete relation %s/%s: %w", model.ObjectResource.ValueString(), model.Key.ValueString(), err).Error(),
		)
		return
	}
}
