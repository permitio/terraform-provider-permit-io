package conditionsets

import (
	"context"
	"fmt"
	"github.com/permitio/permit-golang/pkg/permit"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

var (
	_ datasource.DataSource              = &ConditionSetDataSource{}
	_ datasource.DataSourceWithConfigure = &ConditionSetDataSource{}
)

func NewConditionSetDataSource() datasource.DataSource {
	return &ConditionSetDataSource{}
}

type ConditionSetDataSource struct {
	client ConditionSetClient
}

func (d *ConditionSetDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

	d.client = ConditionSetClient{client: client}
}

func (d *ConditionSetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_condition_set"
}

// Schema defines the schema for the data source.
func (d *ConditionSetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"organization_id": schema.StringAttribute{
				Computed: true,
			},
			"project_id": schema.StringAttribute{
				Computed: true,
			},
			"environment_id": schema.StringAttribute{
				Computed: true,
			},
			"key": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"type": schema.StringAttribute{
				Required: true,
			},
			"resource": schema.StringAttribute{
				Optional: true,
			},
			"conditions": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *ConditionSetDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data ConditionSetModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)

	if response.Diagnostics.HasError() {
		return
	}

	state, err := d.client.Read(ctx, data)

	if err != nil {
		response.Diagnostics.AddError(
			"Unable to Read Resource",
			fmt.Sprintf("Unable to read resource: %s, Error: %s", data.Id.String(), err.Error()),
		)
		return
	}

	diags := response.State.Set(ctx, &state)

	response.Diagnostics.Append(diags...)

	if response.Diagnostics.HasError() {
		return
	}
}
