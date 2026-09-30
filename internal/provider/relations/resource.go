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

	// The Permit API has no operation that updates a relation, so a new name or
	// description replaces it.
	for _, name := range []string{"name", "description"} {
		attribute, ok := attributes[name].(schema.StringAttribute)
		if !ok {
			response.Diagnostics.AddError("Invalid relation schema",
				fmt.Sprintf("The base schema's %s is a %T, not a string attribute. "+
					"Please report this issue to the provider developers.",
					name, attributes[name]))
			return
		}
		attribute.PlanModifiers = append(attribute.PlanModifiers,
			stringplanmodifier.RequiresReplace())
		attributes[name] = attribute
	}

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
			common.APIErrorDetail("create", "relation", relationID(plan), err),
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
		if common.IsNotFoundErr(err) {
			response.State.RemoveResource(ctx)
			return
		}
		response.Diagnostics.AddError(
			"Failed reading relation",
			common.APIErrorDetail("read", "relation", relationID(model), err),
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

	if err != nil && !common.IsNotFoundErr(err) {
		response.Diagnostics.AddError(
			"Failed deleting relation",
			common.APIErrorDetail("delete", "relation", relationID(model), err),
		)
		return
	}
}

// relationID names a relation in error messages by its object resource and key.
func relationID(model relationModel) string {
	return model.ObjectResource.ValueString() + "/" + model.Key.ValueString()
}
