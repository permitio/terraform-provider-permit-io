package resources

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"

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

type attributeModel struct {
	Type        types.String `tfsdk:"type"`
	Description types.String `tfsdk:"description"`
}

type attributesModel map[string]attributeModel

func newAttributesModelsFromSDK(sdkAttributes *map[string]models.AttributeBlockRead) attributesModel {
	var attributes attributesModel
	if sdkAttributes == nil || *sdkAttributes == nil {
		return attributes
	}
	attributes = make(attributesModel)
	for key, attribute := range *sdkAttributes {
		attributes[key] = attributeModel{
			Type:        types.StringValue(string(attribute.Type)),
			Description: types.StringPointerValue(attribute.Description),
		}
	}
	return attributes
}

// newAttributesModelsFromSDKWithPlan converts SDK attributes to the Terraform model,
// preserving the plan's null vs empty map distinction when the API returns an empty map.
func newAttributesModelsFromSDKWithPlan(sdkAttributes *map[string]models.AttributeBlockRead, planAttributes attributesModel) attributesModel {
	result := newAttributesModelsFromSDK(sdkAttributes)
	// If API returned an empty map but the plan had null (user didn't specify attributes),
	// preserve null to avoid "was null, but now empty map" inconsistency.
	if len(result) == 0 && planAttributes == nil {
		return nil
	}
	return result
}

func (a attributesModel) toSDK() map[string]models.AttributeBlockEditable {
	var attributes map[string]models.AttributeBlockEditable
	if a == nil {
		return attributes
	}
	attributes = make(map[string]models.AttributeBlockEditable)
	for key, attribute := range a {
		attributes[key] = models.AttributeBlockEditable{
			Type:        models.AttributeType(attribute.Type.ValueString()),
			Description: attribute.Description.ValueStringPointer(),
		}
	}
	return attributes
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
	Attributes     attributesModel         `tfsdk:"attributes"`
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
		MarkdownDescription: "Reads a resource by its key, with its actions and attributes, " +
			"such as one that Terraform does not manage. Only `key` selects the resource; " +
			"every other attribute holds what Permit returns.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID Permit assigns to the resource.",
			},
			"organization_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the organization the resource belongs to.",
			},
			"project_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the project the resource belongs to.",
			},
			"environment_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the environment the resource belongs to.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the resource was created.",
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the resource was last updated.",
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The key of the resource to read.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The name of the resource. " + common.LookupOnlyInputNote,
			},
			"urn": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The URN of the resource, or null when it has none. " +
					common.LookupOnlyInputNote,
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The description of the resource, or null when it has " +
					"none. " + common.LookupOnlyInputNote,
			},
			"actions": schema.MapNestedAttribute{
				MarkdownDescription: "The actions of the resource, by action key. " +
					common.LookupOnlyInputNote,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The ID Permit assigns to the action.",
						},
						"name": schema.StringAttribute{
							Optional:            true,
							Computed:            true,
							MarkdownDescription: "The name of the action.",
						},
						"description": schema.StringAttribute{
							Optional:            true,
							Computed:            true,
							MarkdownDescription: "The description of the action, or null.",
						},
					},
				},
				Optional: true,
				Computed: true,
			},
			"attributes": schema.MapNestedAttribute{
				MarkdownDescription: "The attributes of the resource, by attribute key. " +
					common.LookupOnlyInputNote,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required: true,
							MarkdownDescription: "The type of the attribute: `bool`, " +
								"`number`, `string`, `time`, `array` or `json`.",
							Validators: []validator.String{
								common.AttributeTypeValidator{},
							},
						},
						"description": schema.StringAttribute{
							Optional:            true,
							Computed:            true,
							MarkdownDescription: "The description of the attribute, or null.",
						},
					},
				},
				Optional: true,
				Computed: true,
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
		lookup := data.Key.ValueString()
		if data.Key.IsNull() {
			lookup = data.Id.ValueString()
		}
		response.Diagnostics.AddError(
			"Unable to Read Resource",
			common.APIErrorDetail("read", "resource", lookup, err),
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
