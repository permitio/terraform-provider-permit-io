// Package providerschema records a provider's schema as a Snapshot, saves it as
// JSON, and compares two snapshots, labelling each change that can break an
// existing configuration.
//
// A snapshot holds what Terraform sees in the provider schema (types, required,
// optional, computed, sensitive, descriptions, deprecation messages) and what it
// does not: the descriptions of each attribute's and block's plan modifiers,
// validators and default. A plan modifier is recorded as its Go type and its
// description, because the framework's RequiresReplaceIf takes any description:
// the type tells Diff that it can force replacement. Attributes and blocks are
// keyed by their dotted path, so the attribute "url" inside the nested attribute
// "mapping_rules" is "mapping_rules.url".
//
// CI decodes the base branch's snapshot file with the head branch's Decode, which
// rejects unknown fields. A change to the file format may therefore only add
// fields; renaming or removing a field takes two pull requests, one that stops
// writing it and one that removes it from the types.
package providerschema

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Snapshot is the schema of a provider, its resources and its data sources.
type Snapshot struct {
	Provider    Schema            `json:"provider"`
	Resources   map[string]Schema `json:"resources"`
	DataSources map[string]Schema `json:"data_sources"`
}

// Schema is the schema of the provider itself, of a resource or of a data source.
type Schema struct {
	Version            int64                `json:"version"`
	Description        string               `json:"description,omitempty"`
	DeprecationMessage string               `json:"deprecation_message,omitempty"`
	Attributes         map[string]Attribute `json:"attributes,omitempty"`
	Blocks             map[string]Block     `json:"blocks,omitempty"`
}

// Attribute is one attribute, at any nesting depth. Type is a Terraform type such
// as "set(string)", or "<nesting>_nested" for a nested attribute, whose own
// attributes are listed under its path.
type Attribute struct {
	Type               string   `json:"type"`
	Required           bool     `json:"required,omitempty"`
	Optional           bool     `json:"optional,omitempty"`
	Computed           bool     `json:"computed,omitempty"`
	Sensitive          bool     `json:"sensitive,omitempty"`
	WriteOnly          bool     `json:"write_only,omitempty"`
	Description        string   `json:"description,omitempty"`
	DeprecationMessage string   `json:"deprecation_message,omitempty"`
	Default            string   `json:"default,omitempty"`
	PlanModifiers      []string `json:"plan_modifiers,omitempty"`
	Validators         []string `json:"validators,omitempty"`
}

// Block is one nested block, at any nesting depth. Its attributes and blocks are
// listed under its path. A MaxItems of 0 means no limit.
type Block struct {
	Nesting            string   `json:"nesting"`
	MinItems           int64    `json:"min_items,omitempty"`
	MaxItems           int64    `json:"max_items,omitempty"`
	Description        string   `json:"description,omitempty"`
	DeprecationMessage string   `json:"deprecation_message,omitempty"`
	PlanModifiers      []string `json:"plan_modifiers,omitempty"`
	Validators         []string `json:"validators,omitempty"`
}

// Build takes the schema the provider serves over protocol 6, with no Terraform
// CLI and no network, and adds the plan modifiers, validators and defaults of each
// attribute and block from the provider's framework schemas.
func Build(ctx context.Context, p provider.Provider) (Snapshot, error) {
	server := providerserver.NewProtocol6(p)()
	resp, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("getting the provider schema: %w", err)
	}
	for _, diag := range resp.Diagnostics {
		if diag.Severity == tfprotov6.DiagnosticSeverityError {
			return Snapshot{}, fmt.Errorf("getting the provider schema: %s: %s",
				diag.Summary, diag.Detail)
		}
	}
	identities, err := server.GetResourceIdentitySchemas(ctx,
		&tfprotov6.GetResourceIdentitySchemasRequest{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("getting the resource identity schemas: %w", err)
	}
	for _, diag := range identities.Diagnostics {
		if diag.Severity == tfprotov6.DiagnosticSeverityError {
			return Snapshot{}, fmt.Errorf("getting the resource identity schemas: %s: %s",
				diag.Summary, diag.Detail)
		}
	}
	if err := checkRecordable(resp, identities); err != nil {
		return Snapshot{}, err
	}

	snapshot := Snapshot{
		Provider:    fromProtocol(resp.Provider),
		Resources:   map[string]Schema{},
		DataSources: map[string]Schema{},
	}
	for name, schema := range resp.ResourceSchemas {
		snapshot.Resources[name] = fromProtocol(schema)
	}
	for name, schema := range resp.DataSourceSchemas {
		snapshot.DataSources[name] = fromProtocol(schema)
	}
	if err := addFrameworkDetails(ctx, p, snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// checkRecordable rejects parts of a provider schema that a Snapshot has no place
// for, so that adding one fails the snapshot instead of leaving it out.
func checkRecordable(resp *tfprotov6.GetProviderSchemaResponse,
	identities *tfprotov6.GetResourceIdentitySchemasResponse) error {
	var found []string
	if resp.ProviderMeta != nil {
		found = append(found, "a provider_meta schema")
	}
	for kind, count := range map[string]int{
		"functions":           len(resp.Functions),
		"ephemeral resources": len(resp.EphemeralResourceSchemas),
		"list resources":      len(resp.ListResourceSchemas),
		"actions":             len(resp.ActionSchemas),
		"state stores":        len(resp.StateStoreSchemas),
		"resource identities": len(identities.IdentitySchemas),
	} {
		if count > 0 {
			found = append(found, kind)
		}
	}
	if len(found) > 0 {
		slices.Sort(found)
		return fmt.Errorf("the provider serves %s, which a schema snapshot does not "+
			"record yet; extend the providerschema package", strings.Join(found, ", "))
	}
	return nil
}

func fromProtocol(schema *tfprotov6.Schema) Schema {
	out := Schema{
		Version:            schema.Version,
		Description:        schema.Block.Description,
		DeprecationMessage: schema.Block.DeprecationMessage,
		Attributes:         map[string]Attribute{},
		Blocks:             map[string]Block{},
	}
	out.addBlockContents("", schema.Block)
	return out
}

func (s Schema) addBlockContents(prefix string, block *tfprotov6.SchemaBlock) {
	for _, attribute := range block.Attributes {
		s.addAttribute(prefix, attribute)
	}
	for _, nested := range block.BlockTypes {
		path := joinPath(prefix, nested.TypeName)
		s.Blocks[path] = Block{
			Nesting:            strings.ToLower(nested.Nesting.String()),
			MinItems:           nested.MinItems,
			MaxItems:           nested.MaxItems,
			Description:        nested.Block.Description,
			DeprecationMessage: nested.Block.DeprecationMessage,
		}
		s.addBlockContents(path, nested.Block)
	}
}

func (s Schema) addAttribute(prefix string, attribute *tfprotov6.SchemaAttribute) {
	path := joinPath(prefix, attribute.Name)
	out := Attribute{
		Required:           attribute.Required,
		Optional:           attribute.Optional,
		Computed:           attribute.Computed,
		Sensitive:          attribute.Sensitive,
		WriteOnly:          attribute.WriteOnly,
		Description:        attribute.Description,
		DeprecationMessage: attribute.DeprecationMessage,
	}
	if attribute.NestedType != nil {
		out.Type = strings.ToLower(attribute.NestedType.Nesting.String()) + "_nested"
		for _, child := range attribute.NestedType.Attributes {
			s.addAttribute(path, child)
		}
	} else {
		out.Type = typeString(attribute.Type)
	}
	s.Attributes[path] = out
}

// typeString writes a type the way Terraform configuration does. A type the
// framework does not give attributes falls back to its tftypes name.
func typeString(t tftypes.Type) string {
	switch typ := t.(type) {
	case tftypes.List:
		return "list(" + typeString(typ.ElementType) + ")"
	case tftypes.Set:
		return "set(" + typeString(typ.ElementType) + ")"
	case tftypes.Map:
		return "map(" + typeString(typ.ElementType) + ")"
	case tftypes.Object:
		fields := make([]string, 0, len(typ.AttributeTypes))
		for _, name := range slices.Sorted(maps.Keys(typ.AttributeTypes)) {
			fields = append(fields, name+"="+typeString(typ.AttributeTypes[name]))
		}
		return "object({" + strings.Join(fields, ", ") + "})"
	}
	switch {
	case t.Is(tftypes.String):
		return "string"
	case t.Is(tftypes.Number):
		return "number"
	case t.Is(tftypes.Bool):
		return "bool"
	case t.Is(tftypes.DynamicPseudoType):
		return "dynamic"
	}
	return t.String()
}

// addFrameworkDetails adds the plan modifiers, validators and defaults that the
// protocol schema leaves out, taken from the framework schema of the provider and
// of each resource and data source.
func addFrameworkDetails(ctx context.Context, p provider.Provider, snapshot Snapshot) error {
	var metadata provider.MetadataResponse
	p.Metadata(ctx, provider.MetadataRequest{}, &metadata)

	var providerSchema provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &providerSchema)
	if err := addDetails(ctx, snapshot.Provider, providerSchema.Schema); err != nil {
		return fmt.Errorf("provider: %w", err)
	}

	for _, newResource := range p.Resources(ctx) {
		r := newResource()
		var name resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: metadata.TypeName}, &name)
		var schema resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schema)
		target, ok := snapshot.Resources[name.TypeName]
		if !ok {
			return fmt.Errorf("resource %q is not in the protocol schema", name.TypeName)
		}
		if err := addDetails(ctx, target, schema.Schema); err != nil {
			return fmt.Errorf("resource %s: %w", name.TypeName, err)
		}
	}

	for _, newDataSource := range p.DataSources(ctx) {
		d := newDataSource()
		var name datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: metadata.TypeName}, &name)
		var schema datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schema)
		target, ok := snapshot.DataSources[name.TypeName]
		if !ok {
			return fmt.Errorf("data source %q is not in the protocol schema", name.TypeName)
		}
		if err := addDetails(ctx, target, schema.Schema); err != nil {
			return fmt.Errorf("data source %s: %w", name.TypeName, err)
		}
	}
	return nil
}

// addDetails walks a framework schema (the schema.Schema of the provider, a
// resource or a data source) and fills in the plan modifiers, validators and
// default of every attribute and block in target. The framework's attribute and
// block types differ per value type but share the field names PlanModifiers,
// Validators, Default, NestedObject, Attributes and Blocks, so the walk reads
// them by name. It fails unless it reaches every attribute and block in target.
func addDetails(ctx context.Context, target Schema, frameworkSchema any) error {
	w := detailWalker{ctx: ctx, target: target}
	if err := w.walk("", reflect.ValueOf(frameworkSchema)); err != nil {
		return err
	}
	if w.attributes != len(target.Attributes) || w.blocks != len(target.Blocks) {
		return fmt.Errorf("the framework schema has %d attributes and %d blocks, the "+
			"protocol schema %d and %d", w.attributes, w.blocks,
			len(target.Attributes), len(target.Blocks))
	}
	return nil
}

type detailWalker struct {
	ctx                context.Context
	target             Schema
	attributes, blocks int
}

// walk visits the Attributes and Blocks maps of container, a framework schema,
// nested object, single nested attribute or single nested block.
func (w *detailWalker) walk(prefix string, container reflect.Value) error {
	for name, value := range fieldMap(container, "Attributes") {
		path := joinPath(prefix, name)
		attribute, ok := w.target.Attributes[path]
		if !ok {
			return fmt.Errorf("framework attribute %q is not in the protocol schema", path)
		}
		attribute.PlanModifiers = w.describeAll(value, "PlanModifiers", w.describePlanModifier)
		attribute.Validators = w.describeAll(value, "Validators", w.describe)
		attribute.Default = w.describe(fieldByName(value, "Default"))
		if err := w.walkNested(path, value, &attribute.PlanModifiers,
			&attribute.Validators); err != nil {
			return err
		}
		w.target.Attributes[path] = attribute
		w.attributes++
	}
	for name, value := range fieldMap(container, "Blocks") {
		path := joinPath(prefix, name)
		block, ok := w.target.Blocks[path]
		if !ok {
			return fmt.Errorf("framework block %q is not in the protocol schema", path)
		}
		block.PlanModifiers = w.describeAll(value, "PlanModifiers", w.describePlanModifier)
		block.Validators = w.describeAll(value, "Validators", w.describe)
		if err := w.walkNested(path, value, &block.PlanModifiers,
			&block.Validators); err != nil {
			return err
		}
		w.target.Blocks[path] = block
		w.blocks++
	}
	return nil
}

// walkNested visits what is nested in an attribute or block. A list, set or map
// nests a NestedObject, whose own plan modifiers and validators are recorded on
// the attribute or block with a "nested object: " prefix. A single nested
// attribute or block holds its Attributes and Blocks itself.
func (w *detailWalker) walkNested(path string, value reflect.Value, planModifiers,
	validators *[]string) error {
	if object := fieldByName(value, "NestedObject"); object.IsValid() {
		for _, description := range w.describeAll(object, "PlanModifiers",
			w.describePlanModifier) {
			*planModifiers = append(*planModifiers, "nested object: "+description)
		}
		for _, description := range w.describeAll(object, "Validators", w.describe) {
			*validators = append(*validators, "nested object: "+description)
		}
		slices.Sort(*planModifiers)
		slices.Sort(*validators)
		return w.walk(path, object)
	}
	return w.walk(path, value)
}

// describer is the Description method shared by the framework's plan modifiers,
// validators and defaults.
type describer interface {
	Description(context.Context) string
}

// describeAll describes each element of the named slice field, sorted so that the
// snapshot does not depend on declaration order.
func (w *detailWalker) describeAll(value reflect.Value, field string,
	describe func(reflect.Value) string) []string {
	list := fieldByName(value, field)
	if !list.IsValid() || list.Kind() != reflect.Slice || list.Len() == 0 {
		return nil
	}
	descriptions := make([]string, 0, list.Len())
	for i := range list.Len() {
		descriptions = append(descriptions, describe(list.Index(i)))
	}
	slices.Sort(descriptions)
	return descriptions
}

// describePlanModifier returns a plan modifier's Go type and description, such as
// "stringplanmodifier.requiresReplaceIfModifier: If the value of this attribute
// changes, Terraform will destroy and recreate the resource.", or only its type
// when it has no description ("<nil>" for a nil plan modifier).
func (w *detailWalker) describePlanModifier(value reflect.Value) string {
	typeName := fmt.Sprintf("%T", value.Interface())
	description := w.describe(value)
	if description == "" || description == typeName {
		return typeName
	}
	return typeName + ": " + description
}

// describe returns the description of a plan modifier, validator or default, or
// its Go type when it has no description, and "" for a missing one.
func (w *detailWalker) describe(value reflect.Value) string {
	if !value.IsValid() || (value.Kind() == reflect.Interface && value.IsNil()) {
		return ""
	}
	item := value.Interface()
	if d, ok := item.(describer); ok {
		if description := d.Description(w.ctx); description != "" {
			return description
		}
	}
	return fmt.Sprintf("%T", item)
}

// fieldByName returns the named field of a struct, looking through interfaces and
// pointers, or the zero Value when there is no such field.
func fieldByName(value reflect.Value, name string) reflect.Value {
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	return value.FieldByName(name)
}

// fieldMap returns the entries of the named map field keyed by string.
func fieldMap(value reflect.Value, name string) map[string]reflect.Value {
	field := fieldByName(value, name)
	if !field.IsValid() || field.Kind() != reflect.Map {
		return nil
	}
	entries := make(map[string]reflect.Value, field.Len())
	iter := field.MapRange()
	for iter.Next() {
		entries[iter.Key().String()] = iter.Value()
	}
	return entries
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// Encode writes a snapshot as indented JSON with sorted keys and a final newline.
func Encode(snapshot Snapshot) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		return nil, fmt.Errorf("encoding the schema snapshot: %w", err)
	}
	return buf.Bytes(), nil
}

// Decode reads a snapshot written by Encode. It rejects empty input, unknown
// fields, trailing data and a snapshot with no resources and no data sources, so
// a truncated or unrelated file is not mistaken for a provider without a schema.
func Decode(data []byte) (Snapshot, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Snapshot{}, errors.New("the input is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var snapshot Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("not a provider schema snapshot: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Snapshot{}, errors.New("not a provider schema snapshot: data after the snapshot")
	}
	if len(snapshot.Resources) == 0 && len(snapshot.DataSources) == 0 {
		return Snapshot{}, errors.New("the snapshot has no resources and no data sources")
	}
	return snapshot, nil
}
