package roles

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

var (
	_ datasource.DataSource              = &RoleDataSource{}
	_ datasource.DataSourceWithConfigure = &RoleDataSource{}
)

func NewRoleDataSource() datasource.DataSource {
	return &RoleDataSource{}
}

type RoleDataSource struct {
	client roleClient
}

func (d *RoleDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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
	d.client.client = client
}

func (d *RoleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

// Schema defines the schema for the data source.
func (d *RoleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a role by its key: a top-level role, or with `resource` " +
			"a role on that resource. Only `key` and `resource` select the role; every other " +
			"attribute holds what Permit returns.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID Permit assigns to the role.",
			},
			"organization_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the organization the role belongs to.",
			},
			"project_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the project the role belongs to.",
			},
			"environment_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the environment the role belongs to.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the role was created.",
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the role was last updated.",
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The key of the role to read.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The name of the role. " + common.LookupOnlyInputNote,
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The description of the role, or null when it has none. " +
					common.LookupOnlyInputNote,
			},
			"permissions": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				MarkdownDescription: "The permissions the role grants: " +
					"`resource_key:action_key` pairs for a top-level role, such as " +
					"`document:read`, and action keys of `resource` for a resource role, such " +
					"as `read`. " + common.LookupOnlyInputNote,
			},
			"extends": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				MarkdownDescription: "The keys of the roles whose permissions this role " +
					"inherits. " + common.LookupOnlyInputNote,
			},
			"resource": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The key of the resource the role belongs to. Set it to " +
					"read a resource role; leave it out to read a top-level role, and it is " +
					"then null.",
			},
			"resource_id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The ID of the resource the role belongs to, or null for " +
					"a top-level role.",
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *RoleDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data roleModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	roleRead, err := d.client.Read(ctx, data.Key.ValueString(), data.Resource.ValueStringPointer())

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read role",
			common.APIErrorDetail("read", "role", roleID(data), err),
		)
		return
	}

	// Set state
	diags := response.State.Set(ctx, &roleRead)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}
