package resources

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/permit"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

var (
	_ datasource.DataSource              = &ResourceDataSource{}
	_ datasource.DataSourceWithConfigure = &ResourceDataSource{}
)

func NewResourceDataSource() datasource.DataSource {
	return &ResourceDataSource{}
}

type ResourceDataSource struct {
	ResourceClient
}
type actionsModel struct {
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

type ResourceModel struct {
	Id             types.String            `tfsdk:"id"`
	OrganizationId types.String            `tfsdk:"organization_id"`
	ProjectId      types.String            `tfsdk:"project_id"`
	EnvironmentId  types.String            `tfsdk:"environment_id"`
	CreatedAt      types.String            `tfsdk:"created_at"`
	UpdatedAt      types.String            `tfsdk:"updated_at"`
	Key            types.String            `tfsdk:"key"`
	Name           types.String            `tfsdk:"name"`
	Urn            types.String            `tfsdk:"urn"`
	Description    types.String            `tfsdk:"description"`
	Actions        map[string]actionsModel `tfsdk:"actions"`
}

func (d *ResourceDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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
	d.client = client
}

func (d *ResourceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource"
}

// Schema defines the schema for the data source.
func (d *ResourceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"organization_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"project_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"environment_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"updated_at": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"key": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"name": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"urn": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"actions": schema.MapNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Required: true,
						},
					},
				},
				Optional: true,
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *ResourceDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data ResourceModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	state, err := d.ResourceRead(ctx, data)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to Read Resource",
			fmt.Sprintf("Unable to read resource: %s, Error: %s", data.Id.String(), err.Error()),
		)
		return
	}

	// Set state
	diags := response.State.Set(ctx, &state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}
