package users

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/permitio/permit-golang/pkg/permit"
)

var (
	_ datasource.DataSource              = &UserDataSource{}
	_ datasource.DataSourceWithConfigure = &UserDataSource{}
)

func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

type UserDataSource struct {
	client userClient
}

func (d *UserDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}
	client, ok := request.ProviderData.(*permit.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *permit.Client, got: %T.", request.ProviderData),
		)
		return
	}
	d.client = userClient{client: client}
}

func (d *UserDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *UserDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a user from the Permit.io directory.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique user ID",
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "User key identifier",
			},
			"email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "User's email address",
			},
			"first_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "User's first name",
			},
			"last_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "User's last name",
			},
			"attributes": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Custom user attributes as JSON string",
			},
			"organization_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Organization ID",
			},
			"project_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Project ID",
			},
			"environment_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Environment ID",
			},
		},
	}
}

func (d *UserDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data userModel
	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	state, err := d.client.Read(ctx, data.Key.ValueString())
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to Read User",
			fmt.Sprintf("Unable to read user with key %s: %s", data.Key.ValueString(), err.Error()),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}
