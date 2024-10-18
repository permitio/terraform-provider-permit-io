package user_attributes

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &UserAttributeResource{}
	_ resource.ResourceWithConfigure = &UserAttributeResource{}
)

func NewUserAttributeResource() resource.Resource {
	return &UserAttributeResource{}
}

type UserAttributeResource struct {
	client userAttributesClient
}

func (c *UserAttributeResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_user_attribute"
}

func (c *UserAttributeResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	permitClient := common.Configure(ctx, request, response)
	c.client = userAttributesClient{client: permitClient}
}

// Schema defines the schema for the user attribute resource.
func (c *UserAttributeResource) Schema(_ context.Context, _ resource.SchemaRequest, response *resource.SchemaResponse) {
	attributes := common.CreateBaseResourceSchema()

	// User attributes does not have a name attribute
	delete(attributes, "name")

	attributes["resource_id"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The ID of the User resource",
	}
	attributes["resource_key"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The key of the User resource, will always be `__user`",
	}
	attributes["key"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The key of the attribute",
	}
	attributes["type"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The type of the attribute",
		Validators: []validator.String{
			common.AttributeTypeValidator{},
		},
	}
	attributes["description"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The description of the attribute",
	}

	response.Schema = schema.Schema{
		Attributes:          attributes,
		MarkdownDescription: "See [the documentation](https://api.permit.io/v2/redoc#tag/User-Attributes/operation/create_user_attribute) for more information about User Attributes",
	}
}

func (c *UserAttributeResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var model userAttributeModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	reality, err := c.client.Create(ctx, model)

	if err != nil {
		response.Diagnostics.AddError(
			"Failed creating user attribute",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, reality)...)
}

func (c *UserAttributeResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var model userAttributeModel

	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	reality, err := c.client.Read(ctx, model.Key.ValueString())

	if err != nil {
		response.Diagnostics.AddError(
			"Failed reading user attribute",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, reality)...)
}

func (c *UserAttributeResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	var model userAttributeModel

	response.Diagnostics.Append(request.Plan.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	reality, err := c.client.Update(ctx, model.Id.ValueString(), model)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed updating user attribute",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, reality)...)
}

func (c *UserAttributeResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var model userAttributeModel

	response.Diagnostics.Append(request.State.Get(ctx, &model)...)

	if response.Diagnostics.HasError() {
		return
	}

	err := c.client.Delete(ctx, model.Key.ValueString())

	if err != nil {
		response.Diagnostics.AddError(
			"Failed deleting user attribute",
			err.Error(),
		)
		return
	}
}
