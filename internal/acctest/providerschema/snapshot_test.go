package providerschema

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	pschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testProvider serves one resource and one data source whose schemas use every
// kind of attribute, block, plan modifier, validator and default the snapshot
// records.
type testProvider struct {
	resourceSchema rschema.Schema
}

func (testProvider) Metadata(_ context.Context, _ provider.MetadataRequest,
	resp *provider.MetadataResponse) {
	resp.TypeName = "example"
}

func (testProvider) Schema(_ context.Context, _ provider.SchemaRequest,
	resp *provider.SchemaResponse) {
	resp.Schema = pschema.Schema{Attributes: map[string]pschema.Attribute{
		"api_key": pschema.StringAttribute{
			Optional:    true,
			Sensitive:   true,
			Description: "The API key.",
			Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
		},
	}}
}

func (testProvider) Configure(context.Context, provider.ConfigureRequest,
	*provider.ConfigureResponse) {
}

func (p testProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		func() resource.Resource { return testResource{schema: p.resourceSchema} },
	}
}

func (testProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{func() datasource.DataSource { return testDataSource{} }}
}

type testResource struct {
	schema rschema.Schema
}

func (testResource) Metadata(_ context.Context, req resource.MetadataRequest,
	resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_thing"
}

func (r testResource) Schema(_ context.Context, _ resource.SchemaRequest,
	resp *resource.SchemaResponse) {
	resp.Schema = r.schema
}

func (testResource) Create(context.Context, resource.CreateRequest, *resource.CreateResponse) {}
func (testResource) Read(context.Context, resource.ReadRequest, *resource.ReadResponse)       {}
func (testResource) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {}
func (testResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {}

type testDataSource struct{}

func (testDataSource) Metadata(_ context.Context, req datasource.MetadataRequest,
	resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lookup"
}

func (testDataSource) Schema(_ context.Context, _ datasource.SchemaRequest,
	resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		MarkdownDescription: "Looks a thing up.",
		Attributes: map[string]dschema.Attribute{
			"key":  dschema.StringAttribute{Required: true},
			"tags": dschema.ListAttribute{Computed: true, ElementType: types.StringType},
		},
	}
}

func (testDataSource) Read(context.Context, datasource.ReadRequest, *datasource.ReadResponse) {}

// undescribedCheck is a validator and plan modifier with no description, so the
// snapshot names its Go type. That name sorts after "nested object: ", which
// shows whether an attribute's own entries and its nested object's are sorted
// together.
type undescribedCheck struct{}

func (undescribedCheck) Description(context.Context) string         { return "" }
func (undescribedCheck) MarkdownDescription(context.Context) string { return "" }
func (undescribedCheck) ValidateObject(context.Context, validator.ObjectRequest,
	*validator.ObjectResponse) {
}
func (undescribedCheck) ValidateList(context.Context, validator.ListRequest,
	*validator.ListResponse) {
}
func (undescribedCheck) PlanModifyList(context.Context, planmodifier.ListRequest,
	*planmodifier.ListResponse) {
}

func testResourceSchema() rschema.Schema {
	return rschema.Schema{
		Version:             2,
		MarkdownDescription: "A thing.",
		DeprecationMessage:  "Use example_other.",
		Attributes: map[string]rschema.Attribute{
			"id": rschema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key": rschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The key.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"enabled": rschema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"old":     rschema.StringAttribute{Optional: true, DeprecationMessage: "Use key."},
			"secret":  rschema.StringAttribute{Optional: true, WriteOnly: true},
			"payload": rschema.DynamicAttribute{Optional: true},
			"meta": rschema.MapAttribute{
				Optional: true,
				ElementType: types.ObjectType{AttrTypes: map[string]attr.Type{
					"b": types.NumberType,
					"a": types.ListType{ElemType: types.BoolType},
				}},
			},
			"rules": rschema.ListNestedAttribute{
				Optional:      true,
				PlanModifiers: []planmodifier.List{undescribedCheck{}},
				Validators:    []validator.List{undescribedCheck{}},
				NestedObject: rschema.NestedAttributeObject{
					Attributes: map[string]rschema.Attribute{
						"url": rschema.StringAttribute{Required: true},
						"priority": rschema.Int64Attribute{
							Optional:   true,
							Validators: []validator.Int64{int64validator.AtLeast(0)},
						},
					},
					PlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
					Validators:    []validator.Object{undescribedCheck{}},
				},
			},
			"auth": rschema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]rschema.Attribute{
					"token": rschema.StringAttribute{Optional: true, Sensitive: true},
				},
			},
		},
		Blocks: map[string]rschema.Block{
			"timeouts": rschema.SingleNestedBlock{
				Attributes: map[string]rschema.Attribute{
					"create": rschema.StringAttribute{Optional: true},
				},
			},
			"step": rschema.ListNestedBlock{
				PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
				Validators:    []validator.List{listvalidator.SizeAtLeast(1)},
				NestedObject: rschema.NestedBlockObject{
					Attributes: map[string]rschema.Attribute{
						"name": rschema.StringAttribute{Required: true},
					},
					Blocks: map[string]rschema.Block{
						"input": rschema.SetNestedBlock{
							NestedObject: rschema.NestedBlockObject{
								Attributes: map[string]rschema.Attribute{
									"value": rschema.StringAttribute{Optional: true},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestBuild(t *testing.T) {
	const (
		listRequiresReplace = "listplanmodifier.requiresReplaceIfModifier: If the value of " +
			"this attribute changes, Terraform will destroy and recreate the resource."
		objectUseStateForUnknown = "objectplanmodifier.useStateForUnknownModifier: Once set, " +
			"the value of this attribute in state will not change."
	)
	got, err := Build(t.Context(), testProvider{resourceSchema: testResourceSchema()})
	if err != nil {
		t.Fatal(err)
	}
	want := Snapshot{
		Provider: Schema{
			Attributes: map[string]Attribute{
				"api_key": {Type: "string", Optional: true, Sensitive: true,
					Description: "The API key.",
					Validators:  []string{"string length must be at least 1"}},
			},
			Blocks: map[string]Block{},
		},
		Resources: map[string]Schema{"example_thing": {
			Version:            2,
			Description:        "A thing.",
			DeprecationMessage: "Use example_other.",
			Attributes: map[string]Attribute{
				"id": {Type: "string", Computed: true,
					PlanModifiers: []string{useStateForUnknown}},
				"key": {Type: "string", Required: true, Description: "The key.",
					PlanModifiers: []string{requiresReplace, useStateForUnknown},
					Validators:    []string{"string length must be at least 1"}},
				"enabled": {Type: "bool", Optional: true, Computed: true,
					Default: "value defaults to true"},
				"old":     {Type: "string", Optional: true, DeprecationMessage: "Use key."},
				"secret":  {Type: "string", Optional: true, WriteOnly: true},
				"payload": {Type: "dynamic", Optional: true},
				"meta":    {Type: "map(object({a=list(bool), b=number}))", Optional: true},
				"rules": {Type: "list_nested", Optional: true,
					PlanModifiers: []string{
						"nested object: " + objectUseStateForUnknown,
						"providerschema.undescribedCheck",
					},
					Validators: []string{
						"nested object: providerschema.undescribedCheck",
						"providerschema.undescribedCheck",
					}},
				"rules.url": {Type: "string", Required: true},
				"rules.priority": {Type: "number", Optional: true,
					Validators: []string{"value must be at least 0"}},
				"auth":             {Type: "single_nested", Optional: true},
				"auth.token":       {Type: "string", Optional: true, Sensitive: true},
				"timeouts.create":  {Type: "string", Optional: true},
				"step.name":        {Type: "string", Required: true},
				"step.input.value": {Type: "string", Optional: true},
			},
			Blocks: map[string]Block{
				"timeouts": {Nesting: "single"},
				"step": {Nesting: "list",
					PlanModifiers: []string{listRequiresReplace},
					Validators:    []string{"list must contain at least 1 elements"}},
				"step.input": {Nesting: "set"},
			},
		}},
		DataSources: map[string]Schema{"example_lookup": {
			Description: "Looks a thing up.",
			Attributes: map[string]Attribute{
				"key":  {Type: "string", Required: true},
				"tags": {Type: "list(string)", Computed: true},
			},
			Blocks: map[string]Block{},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := Encode(got)
		wantJSON, _ := Encode(want)
		t.Errorf("Build() =\n%s\nwant\n%s", gotJSON, wantJSON)
	}
}

// TestBuildLabelsRequiresReplaceIf adds a RequiresReplaceIf whose description does
// not mention replacement and checks that Diff labels it breaking by its Go type.
func TestBuildLabelsRequiresReplaceIf(t *testing.T) {
	base, err := Build(t.Context(), testProvider{resourceSchema: testResourceSchema()})
	if err != nil {
		t.Fatal(err)
	}
	headSchema := testResourceSchema()
	headSchema.Attributes["old"] = rschema.StringAttribute{
		Optional:           true,
		DeprecationMessage: "Use key.",
		PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIf(
			func(context.Context, planmodifier.StringRequest,
				*stringplanmodifier.RequiresReplaceIfFuncResponse) {
			},
			"Changing it makes a new object.", "Changing it makes a new object.")},
	}
	head, err := Build(t.Context(), testProvider{resourceSchema: headSchema})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, change := range Diff(base, head) {
		got = append(got, change.String())
	}
	want := []string{`BREAKING      resource example_thing: attribute "old" now forces ` +
		"replacement: stringplanmodifier.requiresReplaceIfModifier: Changing it makes a " +
		"new object."}
	if !slices.Equal(got, want) {
		t.Errorf("Diff() =\n\t%s\nwant\n\t%s", strings.Join(got, "\n\t"),
			strings.Join(want, "\n\t"))
	}
}

func TestDescribePlanModifier(t *testing.T) {
	modifiers := reflect.ValueOf([]planmodifier.List{
		listplanmodifier.UseStateForUnknown(),
		undescribedCheck{},
		nil,
	})
	want := []string{
		"listplanmodifier.useStateForUnknownModifier: Once set, the value of this " +
			"attribute in state will not change.",
		"providerschema.undescribedCheck",
		"<nil>",
	}
	w := detailWalker{ctx: t.Context()}
	var got []string
	for i := range modifiers.Len() {
		got = append(got, w.describePlanModifier(modifiers.Index(i)))
	}
	if !slices.Equal(got, want) {
		t.Errorf("describePlanModifier() =\n\t%s\nwant\n\t%s", strings.Join(got, "\n\t"),
			strings.Join(want, "\n\t"))
	}
}

// testProviderWithFunction serves a provider function, which a snapshot does not
// record.
type testProviderWithFunction struct {
	testProvider
}

func (testProviderWithFunction) Functions(context.Context) []func() function.Function {
	return []func() function.Function{func() function.Function { return testFunction{} }}
}

type testFunction struct{}

func (testFunction) Metadata(_ context.Context, _ function.MetadataRequest,
	resp *function.MetadataResponse) {
	resp.Name = "echo"
}

func (testFunction) Definition(_ context.Context, _ function.DefinitionRequest,
	resp *function.DefinitionResponse) {
	resp.Definition = function.Definition{Return: function.StringReturn{}}
}

func (testFunction) Run(context.Context, function.RunRequest, *function.RunResponse) {}

// testProviderWithIdentity serves a resource with an identity schema, which a
// snapshot does not record.
type testProviderWithIdentity struct {
	testProvider
	identity identityschema.Schema
}

func (p testProviderWithIdentity) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{func() resource.Resource {
		return testResourceWithIdentity{testResource{schema: p.resourceSchema}, p.identity}
	}}
}

type testResourceWithIdentity struct {
	testResource
	identity identityschema.Schema
}

func (r testResourceWithIdentity) IdentitySchema(_ context.Context,
	_ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = r.identity
}

func TestBuildErrors(t *testing.T) {
	invalid := testResourceSchema()
	invalid.Attributes["broken"] = rschema.StringAttribute{}
	identity := func(attribute identityschema.Attribute) testProviderWithIdentity {
		return testProviderWithIdentity{
			testProvider: testProvider{resourceSchema: testResourceSchema()},
			identity: identityschema.Schema{
				Attributes: map[string]identityschema.Attribute{"key": attribute},
			},
		}
	}
	tests := []struct {
		name     string
		provider provider.Provider
		want     string
	}{
		{
			name:     "invalid schema",
			provider: testProvider{resourceSchema: invalid},
			want:     "must have Required, Optional, or Computed set",
		},
		{
			name:     "unrecorded provider function",
			provider: testProviderWithFunction{testProvider{resourceSchema: testResourceSchema()}},
			want:     "the provider serves functions, which a schema snapshot does not record",
		},
		{
			name:     "unrecorded resource identity",
			provider: identity(identityschema.StringAttribute{RequiredForImport: true}),
			want: "the provider serves resource identities, which a schema snapshot does " +
				"not record",
		},
		{
			name:     "invalid resource identity",
			provider: identity(identityschema.ListAttribute{RequiredForImport: true}),
			want: "getting the resource identity schemas: Invalid Attribute Implementation: " +
				"When validating the schema",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Build(t.Context(), tt.provider)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Build() error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

// TestCheckRecordable sets each part of a provider schema that a snapshot does
// not record, in turn, and checks that the snapshot is refused and names it.
func TestCheckRecordable(t *testing.T) {
	const notRecorded = ", which a schema snapshot does not record yet"
	tests := []struct {
		name       string
		schema     *tfprotov6.GetProviderSchemaResponse
		identities *tfprotov6.GetResourceIdentitySchemasResponse
		want       string
	}{
		{
			name: "only resources and data sources",
			schema: &tfprotov6.GetProviderSchemaResponse{
				ResourceSchemas:   map[string]*tfprotov6.Schema{"example_thing": {}},
				DataSourceSchemas: map[string]*tfprotov6.Schema{"example_lookup": {}},
			},
		},
		{
			name:   "provider_meta",
			schema: &tfprotov6.GetProviderSchemaResponse{ProviderMeta: &tfprotov6.Schema{}},
			want:   "the provider serves a provider_meta schema" + notRecorded,
		},
		{
			name: "functions",
			schema: &tfprotov6.GetProviderSchemaResponse{
				Functions: map[string]*tfprotov6.Function{"echo": {}},
			},
			want: "the provider serves functions" + notRecorded,
		},
		{
			name: "ephemeral resources",
			schema: &tfprotov6.GetProviderSchemaResponse{
				EphemeralResourceSchemas: map[string]*tfprotov6.Schema{"example_token": {}},
			},
			want: "the provider serves ephemeral resources" + notRecorded,
		},
		{
			name: "list resources",
			schema: &tfprotov6.GetProviderSchemaResponse{
				ListResourceSchemas: map[string]*tfprotov6.Schema{"example_thing": {}},
			},
			want: "the provider serves list resources" + notRecorded,
		},
		{
			name: "actions",
			schema: &tfprotov6.GetProviderSchemaResponse{
				ActionSchemas: map[string]*tfprotov6.ActionSchema{"example_run": {}},
			},
			want: "the provider serves actions" + notRecorded,
		},
		{
			name: "state stores",
			schema: &tfprotov6.GetProviderSchemaResponse{
				StateStoreSchemas: map[string]*tfprotov6.Schema{"example_store": {}},
			},
			want: "the provider serves state stores" + notRecorded,
		},
		{
			name: "resource identities",
			identities: &tfprotov6.GetResourceIdentitySchemasResponse{
				IdentitySchemas: map[string]*tfprotov6.ResourceIdentitySchema{
					"example_thing": {},
				},
			},
			want: "the provider serves resource identities" + notRecorded,
		},
		{
			name: "several, in name order",
			schema: &tfprotov6.GetProviderSchemaResponse{
				ProviderMeta:  &tfprotov6.Schema{},
				Functions:     map[string]*tfprotov6.Function{"echo": {}},
				ActionSchemas: map[string]*tfprotov6.ActionSchema{"example_run": {}},
			},
			want: "the provider serves a provider_meta schema, actions, functions" + notRecorded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, identities := tt.schema, tt.identities
			if schema == nil {
				schema = &tfprotov6.GetProviderSchemaResponse{}
			}
			if identities == nil {
				identities = &tfprotov6.GetResourceIdentitySchemasResponse{}
			}
			err := checkRecordable(schema, identities)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("checkRecordable() error = %v, want none", err)
			case tt.want != "" && (err == nil || err.Error() != tt.want+
				"; extend the providerschema package"):
				t.Fatalf("checkRecordable() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestAddDetailsErrors gives addDetails a protocol schema with one attribute or
// block more or fewer than the framework schema, and checks that it fails.
func TestAddDetailsErrors(t *testing.T) {
	framework := rschema.Schema{
		Attributes: map[string]rschema.Attribute{"a": rschema.StringAttribute{Optional: true}},
		Blocks:     map[string]rschema.Block{"b": rschema.SingleNestedBlock{}},
	}
	target := func(attributes, blocks []string) Schema {
		schema := Schema{Attributes: map[string]Attribute{}, Blocks: map[string]Block{}}
		for _, path := range attributes {
			schema.Attributes[path] = Attribute{Type: "string", Optional: true}
		}
		for _, path := range blocks {
			schema.Blocks[path] = Block{Nesting: "single"}
		}
		return schema
	}
	tests := []struct {
		name   string
		target Schema
		want   string
	}{
		{name: "same attributes and blocks", target: target([]string{"a"}, []string{"b"})},
		{
			name:   "an attribute the framework schema lacks",
			target: target([]string{"a", "extra"}, []string{"b"}),
			want: "the framework schema has 1 attributes and 1 blocks, the protocol schema " +
				"2 and 1",
		},
		{
			name:   "a block the framework schema lacks",
			target: target([]string{"a"}, []string{"b", "extra"}),
			want: "the framework schema has 1 attributes and 1 blocks, the protocol schema " +
				"1 and 2",
		},
		{
			name:   "an attribute the protocol schema lacks",
			target: target(nil, []string{"b"}),
			want:   `framework attribute "a" is not in the protocol schema`,
		},
		{
			name:   "a block the protocol schema lacks",
			target: target([]string{"a"}, nil),
			want:   `framework block "b" is not in the protocol schema`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := addDetails(t.Context(), tt.target, framework)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("addDetails() error = %v, want none", err)
			case tt.want != "" && (err == nil || err.Error() != tt.want):
				t.Fatalf("addDetails() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestAddFrameworkDetailsErrors removes a resource, data source or attribute from
// a built snapshot and checks that adding the framework details to it fails and
// says where.
func TestAddFrameworkDetailsErrors(t *testing.T) {
	p := testProvider{resourceSchema: testResourceSchema()}
	built, err := Build(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(*Snapshot)
		want   string
	}{
		{name: "unchanged", change: func(*Snapshot) {}},
		{
			name:   "provider attribute missing",
			change: func(s *Snapshot) { delete(s.Provider.Attributes, "api_key") },
			want:   `provider: framework attribute "api_key" is not in the protocol schema`,
		},
		{
			name:   "resource missing",
			change: func(s *Snapshot) { delete(s.Resources, "example_thing") },
			want:   `resource "example_thing" is not in the protocol schema`,
		},
		{
			name: "resource attribute missing",
			change: func(s *Snapshot) {
				delete(s.Resources["example_thing"].Attributes, "key")
			},
			want: `resource example_thing: framework attribute "key" is not in the ` +
				"protocol schema",
		},
		{
			name:   "data source missing",
			change: func(s *Snapshot) { delete(s.DataSources, "example_lookup") },
			want:   `data source "example_lookup" is not in the protocol schema`,
		},
		{
			name: "data source attribute missing",
			change: func(s *Snapshot) {
				delete(s.DataSources["example_lookup"].Attributes, "tags")
			},
			want: `data source example_lookup: framework attribute "tags" is not in the ` +
				"protocol schema",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := clone(t, built)
			tt.change(&snapshot)
			err := addFrameworkDetails(t.Context(), p, snapshot)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("addFrameworkDetails() error = %v, want none", err)
			case tt.want != "" && (err == nil || err.Error() != tt.want):
				t.Fatalf("addFrameworkDetails() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestFromProtocolBlock records a block's limits, description and deprecation
// from a protocol schema, since the framework's blocks never set min or max items.
func TestFromProtocolBlock(t *testing.T) {
	got := fromProtocol(&tfprotov6.Schema{Block: &tfprotov6.SchemaBlock{
		BlockTypes: []*tfprotov6.SchemaNestedBlock{{
			TypeName: "step",
			Nesting:  tfprotov6.SchemaNestedBlockNestingModeList,
			MinItems: 1,
			MaxItems: 3,
			Block: &tfprotov6.SchemaBlock{
				Description:        "A step.",
				DeprecationMessage: "Use stage.",
			},
		}},
	}})
	want := Block{
		Nesting:            "list",
		MinItems:           1,
		MaxItems:           3,
		Description:        "A step.",
		DeprecationMessage: "Use stage.",
	}
	if !reflect.DeepEqual(got.Blocks, map[string]Block{"step": want}) {
		t.Errorf("fromProtocol() blocks = %+v, want step: %+v", got.Blocks, want)
	}
}

func TestEncodeDecode(t *testing.T) {
	snapshot := baseFixture()
	data, err := Encode(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "}\n") || !strings.Contains(string(data), "\n  \"") {
		t.Errorf("Encode() is not indented JSON ending in a newline:\n%s", data)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, snapshot) {
		t.Errorf("Decode(Encode()) = %+v, want %+v", decoded, snapshot)
	}
	again, err := Encode(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(data) {
		t.Errorf("encoding is not stable:\n%s\nthen\n%s", data, again)
	}
}

func TestDecodeRejects(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "the input is empty"},
		{name: "whitespace", input: " \n", want: "the input is empty"},
		{name: "truncated", input: `{"resources": {`, want: "not a provider schema snapshot"},
		{name: "not an object", input: `[1]`, want: "not a provider schema snapshot"},
		{
			name:  "unknown field",
			input: `{"resources": {"a": {"attributes": {"id": {"typ": "string"}}}}}`,
			want:  `unknown field "typ"`,
		},
		{
			name:  "trailing data",
			input: `{"resources": {"a": {}}} {}`,
			want:  "data after the snapshot",
		},
		{name: "null", input: "null", want: "no resources and no data sources"},
		{
			name:  "no resources or data sources",
			input: `{"provider": {"version": 0}, "resources": {}, "data_sources": {}}`,
			want:  "no resources and no data sources",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode([]byte(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Decode(%q) error = %v, want one containing %q", tt.input, err, tt.want)
			}
		})
	}
}
