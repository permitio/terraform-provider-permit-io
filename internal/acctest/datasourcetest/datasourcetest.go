// Package datasourcetest checks, without Terraform or the Permit API, that a data
// source's schema and the Go model its Read decodes the configuration into agree.
package datasourcetest

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// filling says which attributes a configuration sets.
type filling string

const (
	everyAttribute    filling = "sets every attribute"
	requiredOnly      filling = "leaves out every attribute that is not required"
	requiredAndNested filling = "sets the nested attributes and leaves out every other " +
		"attribute that is not required"
)

// CheckConfigDecodes decodes configurations of d into model, as Read does with
// request.Config.Get, and fails the test on any diagnostic. model is a pointer to
// the struct Read decodes into. There are three configurations: one sets every
// attribute of the schema, nested attributes included; one leaves out every
// attribute that is not required; and one sets the nested attributes but leaves
// out every other attribute that is not required, inside them too. An attribute
// the struct lacks, a field the schema lacks, a field of the wrong type, or a
// field that cannot hold null for an attribute a configuration may leave out each
// make Get fail, and so the test.
func CheckConfigDecodes(t testing.TB, d datasource.DataSource, model any) {
	t.Helper()
	ctx := t.Context()
	var resp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("getting the data source schema: %v", resp.Diagnostics)
	}
	objectType, ok := resp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok || len(objectType.AttributeTypes) == 0 {
		t.Fatal("DID NOT RUN: the data source schema has no attributes")
	}

	for _, fill := range []filling{everyAttribute, requiredOnly, requiredAndNested} {
		b := configBuilder{schema: resp.Schema, fill: fill}
		value, err := b.value(ctx, objectType, tftypes.NewAttributePath())
		if err != nil {
			t.Fatalf("building a configuration that %s: %v", fill, err)
		}
		config := tfsdk.Config{Schema: resp.Schema, Raw: value}
		for _, diag := range config.Get(ctx, model) {
			t.Errorf("decoding a configuration that %s into %T: %s: %s", fill, model,
				diag.Summary(), diag.Detail())
		}
	}
}

// configBuilder builds a configuration of schema that sets the attributes fill
// asks for and leaves out the others.
type configBuilder struct {
	schema schema.Schema
	fill   filling
}

// value returns a known, non-null value of typ for the attribute at path, or for
// the whole configuration at the empty path. A collection has one element, so
// that building it reaches every nested attribute. A string is "{}", which is
// also valid JSON, so that an attribute of a JSON string type such as
// jsontypes.Normalized decodes too.
func (b configBuilder) value(
	ctx context.Context,
	typ tftypes.Type,
	at *tftypes.AttributePath,
) (tftypes.Value, error) {
	switch {
	case typ.Is(tftypes.String):
		return tftypes.NewValue(typ, "{}"), nil
	case typ.Is(tftypes.Number):
		return tftypes.NewValue(typ, big.NewFloat(1)), nil
	case typ.Is(tftypes.Bool):
		return tftypes.NewValue(typ, true), nil
	}
	switch typ := typ.(type) {
	case tftypes.List:
		element, err := b.value(ctx, typ.ElementType, at.WithElementKeyInt(0))
		return tftypes.NewValue(typ, []tftypes.Value{element}), err
	case tftypes.Set:
		key := tftypes.NewValue(typ.ElementType, nil)
		element, err := b.value(ctx, typ.ElementType, at.WithElementKeyValue(key))
		return tftypes.NewValue(typ, []tftypes.Value{element}), err
	case tftypes.Map:
		element, err := b.value(ctx, typ.ElementType, at.WithElementKeyString("x"))
		return tftypes.NewValue(typ, map[string]tftypes.Value{"x": element}), err
	case tftypes.Object:
		attributes := map[string]tftypes.Value{}
		for name, attributeType := range typ.AttributeTypes {
			attributePath := at.WithAttributeName(name)
			leaveOut, err := b.leavesOut(ctx, attributePath, attributeType)
			if err != nil {
				return tftypes.Value{}, err
			}
			if leaveOut {
				attributes[name] = tftypes.NewValue(attributeType, nil)
				continue
			}
			value, err := b.value(ctx, attributeType, attributePath)
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("%s: %w", name, err)
			}
			attributes[name] = value
		}
		return tftypes.NewValue(typ, attributes), nil
	}
	return tftypes.Value{}, fmt.Errorf("no known value for type %s; extend value", typ)
}

// leavesOut reports whether the configuration leaves out the attribute at path,
// whose type is typ.
func (b configBuilder) leavesOut(
	ctx context.Context,
	at *tftypes.AttributePath,
	typ tftypes.Type,
) (bool, error) {
	if b.fill == everyAttribute {
		return false, nil
	}
	attribute, err := b.schema.AttributeAtTerraformPath(ctx, at)
	if err != nil {
		return false, fmt.Errorf("looking up the attribute %s: %w", at, err)
	}
	if attribute.IsRequired() {
		return false, nil
	}
	return b.fill != requiredAndNested || !holdsObjects(typ), nil
}

// holdsObjects reports whether typ is an object or a collection of objects, the
// type of a nested attribute.
func holdsObjects(typ tftypes.Type) bool {
	switch typ := typ.(type) {
	case tftypes.Object:
		return true
	case tftypes.List:
		return holdsObjects(typ.ElementType)
	case tftypes.Set:
		return holdsObjects(typ.ElementType)
	case tftypes.Map:
		return holdsObjects(typ.ElementType)
	}
	return false
}
