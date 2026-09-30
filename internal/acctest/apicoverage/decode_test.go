package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/pkg/models"
)

// decodeSchemas parses component schemas written as JSON.
func decodeSchemas(t *testing.T, text string) map[string]any {
	t.Helper()
	var schemas map[string]any
	if err := json.Unmarshal([]byte(text), &schemas); err != nil {
		t.Fatal(err)
	}
	return schemas
}

// failing returns the labels of the results that failed, and fails the test when
// the base fixture is one of them.
func failing(t *testing.T, results []variantResult) []string {
	t.Helper()
	if results[0].err != nil {
		t.Fatalf("the base fixture failed: %v", results[0].err)
	}
	var labels []string
	for _, result := range results {
		if result.err != nil {
			labels = append(labels, result.label)
		}
	}
	return labels
}

type testSecretThing struct {
	Color  testColor `json:"color"`
	Secret string    `json:"secret"`
}

func TestCheckUnit(t *testing.T) {
	schemas := decodeSchemas(t, `{
	  "Color": {"type": "string", "enum": ["blue", "red", "green"]},
	  "Thing": {"type": "object", "properties": {
	    "color": {"$ref": "#/components/schemas/Color"},
	    "secret": {"anyOf": [{"type": "Weird"}, {"type": "string"}]}}},
	  "Unbuildable": {"type": "object", "properties": {
	    "secret": {"anyOf": [{"type": "Weird"}, {"type": "Odd"}]}}}
	}`)

	t.Run("a failing default is replaced, so a failure is down to its choice", func(t *testing.T) {
		unit := &decodeUnit{goType: reflect.TypeFor[*testSecretThing](),
			schema: map[string]any{"$ref": "#/components/schemas/Thing"}}
		results := checkUnit(schemas, unit)
		want := []string{"Color=blue", "Thing.secret as Weird"}
		if got := failing(t, results); !slices.Equal(got, want) {
			t.Errorf("failing fixtures = %q, want %q", got, want)
		}
		labels := make([]string, len(results))
		for i, result := range results {
			labels[i] = result.label
		}
		want = []string{"", "Color=blue", "Color=green", "Thing.secret as Weird"}
		if !slices.Equal(labels, want) {
			t.Errorf("fixtures = %q, want %q", labels, want)
		}
	})

	t.Run("a choice with no alternative that can be built is left out", func(t *testing.T) {
		unit := &decodeUnit{goType: reflect.TypeFor[*testSecretThing](),
			schema: map[string]any{"$ref": "#/components/schemas/Unbuildable"}}
		results := checkUnit(schemas, unit)
		want := []string{"Unbuildable.secret as Weird", "Unbuildable.secret as Odd"}
		if got := failing(t, results); !slices.Equal(got, want) {
			t.Errorf("failing fixtures = %q, want %q", got, want)
		}
		if err := results[1].err; err == nil ||
			!strings.Contains(err.Error(), "type Weird is not a JSON Schema type") {
			t.Errorf("the Weird fixture failed with %v, want the type it cannot build", err)
		}
	})
}

// strictThing rejects fields it does not know, as a strict decoder would.
type strictThing struct {
	Key string `json:"key"`
}

func (s *strictThing) UnmarshalJSON(data []byte) error {
	type plain strictThing
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*plain)(s))
}

func TestFixtures(t *testing.T) {
	schemas := decodeSchemas(t, `{
	  "Node": {"type": "object", "properties": {
	    "key": {"type": "string"},
	    "parent": {"$ref": "#/components/schemas/Node"},
	    "tags": {"type": "array", "items": {"type": "string", "format": "uuid"}},
	    "labels": {"type": "object", "additionalProperties": {"type": "integer"}},
	    "settings": {"type": "object"},
	    "kind": {"anyOf": [
	      {"type": "object", "properties": {"mode": {"type": "string", "enum": ["a", "b"]}}},
	      {"type": "null"}]}}}
	}`)
	unit := &decodeUnit{goType: reflect.TypeFor[map[string]any](),
		schema: map[string]any{"$ref": "#/components/schemas/Node"}}

	g := &fixtureGen{schemas: schemas}
	value, ok := g.generate(unit.schema, location{})
	if !ok || len(g.problems) > 0 {
		t.Fatalf("generate = %v, %v, problems %q", value, ok, g.problems)
	}
	want := map[string]any{
		"key":        "x",
		"tags":       []any{"00000000-0000-4000-8000-000000000000"},
		"labels":     map[string]any{mapKey: 1},
		"settings":   map[string]any{mapKey: "x"},
		"kind":       map[string]any{"mode": "a", unknownField: "x"},
		unknownField: "x",
	}
	if !reflect.DeepEqual(value, want) {
		t.Errorf("fixture = %#v, want %#v", value, want)
	}

	results := checkUnit(schemas, unit)
	labels := make([]string, len(results))
	for i, result := range results {
		labels[i] = result.label
	}
	wantLabels := []string{"", "Node.kind as null", "Node.kind.mode=b"}
	if !slices.Equal(labels, wantLabels) {
		t.Errorf("fixtures = %q, want %q", labels, wantLabels)
	}

	strict := &decodeUnit{goType: reflect.TypeFor[*strictThing](),
		schema: map[string]any{"type": "object", "properties": map[string]any{
			"key": map[string]any{"type": "string"}}}}
	results = checkUnit(schemas, strict)
	if len(results) != 1 || results[0].err == nil ||
		!strings.Contains(results[0].err.Error(), unknownField) {
		t.Errorf("the strict model's results = %+v, want the base fixture to fail on %s",
			results, unknownField)
	}
}

// TestSDKCannotReadHeadersSecret pins an SDK defect the decode check cannot reach.
// The spec types a proxy config's secret with names that are not JSON Schema
// types (PER-16342), so no fixture of it can be built, and the SDK reads secret
// as a string, so it cannot read a Headers secret, an object (PER-16617). When
// the SDK can, this fails: remove it and fix the ProxyConfigRead.secret notes in
// coverage/operations.yaml.
func TestSDKCannotReadHeadersSecret(t *testing.T) {
	goType := reflect.TypeFor[*models.ProxyConfigRead]()
	if err := decodeInto(goType, map[string]any{"key": "k", "secret": "token"}); err != nil {
		t.Fatalf("a string secret: %v", err)
	}
	headers := map[string]any{"key": "k", "secret": map[string]any{"X-Api-Key": "token"}}
	err := decodeInto(goType, headers)
	if err == nil || !strings.Contains(err.Error(), "ProxyConfigRead.secret") {
		t.Errorf("decoding a Headers secret: error %v, want the SDK to reject secret; if the "+
			"SDK reads it now, remove this test and fix the notes (PER-16617)", err)
	}
}

func TestCheckDecodingUnits(t *testing.T) {
	s, err := parseSpec([]byte(testSpec))
	if err != nil {
		t.Fatal(err)
	}
	check := checkDecoding(s, []string{"list_things", "create_thing", "get_thing", "delete_thing"},
		testFacts().models, nil)
	var units []string
	for _, unit := range check.units {
		units = append(units, unit.model+" "+strings.Join(unit.operations, ","))
	}
	want := []string{"[]main.testThing list_things", "*main.testThing create_thing,get_thing"}
	if !slices.Equal(units, want) {
		t.Errorf("units = %q, want %q", units, want)
	}
	if want := []string{"delete_thing"}; !slices.Equal(check.untyped, want) {
		t.Errorf("untyped = %q, want %q", check.untyped, want)
	}
	if len(check.findings) > 0 || check.fixtures != 4 {
		t.Errorf("findings %q and %d fixtures, want none and 4", check.findings, check.fixtures)
	}
}
