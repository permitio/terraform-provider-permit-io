package resources

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
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

type attributeTypeValidator struct{}

func (a attributeTypeValidator) Description(ctx context.Context) string {
	return "The type of the attribute in the resource."
}

func (a attributeTypeValidator) MarkdownDescription(ctx context.Context) string {
	return "The type of the attribute in the resource."
}

func (a attributeTypeValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsUnknown() {
		response.Diagnostics.AddError("Unable to read resource attribute type",
			fmt.Sprintf("Unable to read resource attribute type: %s", request.Path.String()),
		)
		return
	}
	if request.ConfigValue.IsNull() {
		response.Diagnostics.AddError("Invalid resource attribute type",
			fmt.Sprintf("Invalid null resource attribute type: %s", request.Path.String()),
		)
		return
	}

	value := request.ConfigValue.ValueString()
	if !models.AttributeType(value).IsValid() {
		response.Diagnostics.AddError("Invalid resource attribute type",
			fmt.Sprintf("Invalid resource attribute type: %s", value),
		)
		return
	}
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
			"created_at": schema.StringAttribute{
				Computed: true,
			},
			"updated_at": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"key": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				Required: true,
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
							Computed: true,
						},
						"name": schema.StringAttribute{
							Required: true,
						},
						"description": schema.StringAttribute{
							Optional: true,
						},
					},
				},
				Required: true,
			},
			"attributes": schema.MapNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								attributeTypeValidator{},
							},
						},
						"description": schema.StringAttribute{
							Optional: true,
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
