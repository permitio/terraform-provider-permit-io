package provider

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
	client *permit.Client
}

//type ResourcesDataSourceModel struct {
//	Resources []resourceModel `tfsdk:"resources"`
//}

type actionsModel struct {
	Id types.String `tfsdk:"id"`
}

type ResourceDataSourceModel struct {
	ID             types.String            `tfsdk:"id"`
	OrganizationID types.String            `tfsdk:"organization_id"`
	ProjectID      types.String            `tfsdk:"project_id"`
	EnvironmentID  types.String            `tfsdk:"environment_id"`
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
	var data ResourceDataSourceModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	resource, err := d.client.Api.Resources.Get(ctx, data.ID.ValueString())
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to Read Resource",
			fmt.Sprintf("Unable to read resource: %s, Error: %s", data.ID.String(), err.Error()),
		)
		return
	}

	var (
		urn         types.String
		description types.String
		actions     map[string]actionsModel
	)

	if resource.Urn == nil {
		urn = types.StringNull()
	} else {
		urn = types.StringValue(*resource.Urn)
	}

	if resource.Description == nil {
		description = types.StringNull()
	} else {
		description = types.StringValue(*resource.Description)
	}

	if resource.Actions != nil {
		actions = make(map[string]actionsModel)
		for key, action := range *resource.Actions {
			actions[key] = actionsModel{
				Id: types.StringValue(action.Id),
			}
		}

	}

	state := ResourceDataSourceModel{
		ID:             types.StringValue(resource.Id),
		OrganizationID: types.StringValue(resource.OrganizationId),
		ProjectID:      types.StringValue(resource.ProjectId),
		EnvironmentID:  types.StringValue(resource.EnvironmentId),
		CreatedAt:      types.StringValue(resource.CreatedAt.String()),
		UpdatedAt:      types.StringValue(resource.UpdatedAt.String()),
		Key:            types.StringValue(resource.Key),
		Name:           types.StringValue(resource.Name),
		Urn:            urn,
		Description:    description,
		Actions:        actions,
	}

	// Set state
	diags := response.State.Set(ctx, &state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}
