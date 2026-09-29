package user_attributes

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/permitio/permit-golang/pkg/permit"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &UserAttributeDataSource{}
	_ datasource.DataSourceWithConfigure = &UserAttributeDataSource{}
)

func NewUserAttributeDataSource() datasource.DataSource {
	return &UserAttributeDataSource{}
}

type UserAttributeDataSource struct {
	client userAttributesClient
}

func (d *UserAttributeDataSource) Configure(_ context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}
	client, ok := request.ProviderData.(*permit.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *permit.Client, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)
		return
	}
	d.client = userAttributesClient{client: client}
}

func (d *UserAttributeDataSource) Metadata(_ context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_user_attribute"
}

func (d *UserAttributeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Fetches a user attribute by key. See [the documentation](https://api.permit.io/v2/redoc#tag/User-Attributes/operation/get_user_attribute) for more information about User Attributes",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The attribute ID. This is a unique identifier for the attribute.",
			},
			"organization_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The organization ID. This is a unique identifier for the organization.",
			},
			"project_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The project ID. This is a unique identifier for the project.",
			},
			"environment_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The environment ID. This is a unique identifier for the environment.",
			},
			"resource_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the User resource",
			},
			"resource_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The key of the User resource, will always be `__user`",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The creation timestamp. This is a timestamp for when the object was created.",
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The update timestamp. This is a timestamp for when the object was last updated.",
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The key of the attribute",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of the attribute",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The description of the attribute",
			},
		},
	}
}

func (d *UserAttributeDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data userAttributeModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	state, err := d.client.Read(ctx, data.Key.ValueString())
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read user attribute",
			fmt.Sprintf("Unable to read user attribute with key %s: %s", data.Key.ValueString(), err.Error()),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}
