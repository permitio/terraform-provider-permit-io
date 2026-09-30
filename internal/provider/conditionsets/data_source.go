package conditionsets

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
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

// conditionSetDataSourceModel is a user set or a resource set that the
// permitio_condition_set data source looks up by key.
type conditionSetDataSourceModel struct {
	Id             types.String `tfsdk:"id"`
	OrganizationId types.String `tfsdk:"organization_id"`
	ProjectId      types.String `tfsdk:"project_id"`
	EnvironmentId  types.String `tfsdk:"environment_id"`
	Key            types.String `tfsdk:"key"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Type           types.String `tfsdk:"type"`
	Resource       types.String `tfsdk:"resource"`
	Conditions     types.String `tfsdk:"conditions"`
	ParentId       types.String `tfsdk:"parent_id"`
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

// Schema defines the schema for the data source. Only key selects the condition
// set. The other settable attributes are accepted so that older configurations
// keep working, and hold the values read from Permit.
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
				Optional: true,
				Computed: true,
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"type": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"resource": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"conditions": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"parent_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the parent condition set, if any.",
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *ConditionSetDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var config conditionSetDataSourceModel

	response.Diagnostics.Append(request.Config.Get(ctx, &config)...)
	if response.Diagnostics.HasError() {
		return
	}

	key := config.Key.ValueString()
	conditionSet, err := d.client.client.Api.ConditionSets.Get(ctx, key)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read condition set",
			common.APIErrorDetail("read", "condition set", key, err),
		)
		return
	}

	state, err := newConditionSetDataSourceModel(conditionSet)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read condition set",
			common.APIErrorDetail("read", "condition set", key, err),
		)
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func newConditionSetDataSourceModel(
	conditionSet *models.ConditionSetRead,
) (conditionSetDataSourceModel, error) {
	conditions, err := json.Marshal(conditionSet.Conditions)
	if err != nil {
		return conditionSetDataSourceModel{}, fmt.Errorf("encoding the conditions: %w", err)
	}

	setType := types.StringNull()
	if conditionSet.Type != nil {
		setType = types.StringValue(string(*conditionSet.Type))
	}

	resource := types.StringNull()
	if conditionSet.Resource != nil {
		resource = types.StringValue(conditionSet.Resource.Key)
	}

	parentId := types.StringNull()
	if conditionSet.ParentId != nil {
		encoded, err := json.Marshal(conditionSet.ParentId)
		if err != nil {
			return conditionSetDataSourceModel{}, fmt.Errorf("encoding the parent_id: %w", err)
		}
		var id string
		if err := json.Unmarshal(encoded, &id); err != nil {
			return conditionSetDataSourceModel{}, fmt.Errorf("decoding the parent_id %s: %w",
				encoded, err)
		}
		parentId = types.StringValue(id)
	}

	return conditionSetDataSourceModel{
		Id:             types.StringValue(conditionSet.Id),
		OrganizationId: types.StringValue(conditionSet.OrganizationId),
		ProjectId:      types.StringValue(conditionSet.ProjectId),
		EnvironmentId:  types.StringValue(conditionSet.EnvironmentId),
		Key:            types.StringValue(conditionSet.Key),
		Name:           types.StringValue(conditionSet.Name),
		Description:    types.StringPointerValue(conditionSet.Description),
		Type:           setType,
		Resource:       resource,
		Conditions:     types.StringValue(string(conditions)),
		ParentId:       parentId,
	}, nil
}
