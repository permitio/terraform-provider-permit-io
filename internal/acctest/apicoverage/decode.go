package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// The decode check builds response fixtures from the spec and decodes them with
// the SDK's models, as the SDK's generated client does, to find values the API
// may send that the SDK rejects. Every object with properties gets a field the
// spec does not have, as a newer API may send, and every choice the spec offers,
// each enum value and each anyOf or oneOf alternative, gets a fixture of its own
// with every other choice at its default.

// unknownField is the field the spec does not have, added to every fixture object
// with properties, and mapKey the key of the one entry of a map.
const (
	unknownField = "tolerant_reader_unknown_field"
	mapKey       = "tolerant_reader_key"
)

// decodeUnit is a response model the provider decodes: the Go type the SDK
// decodes a covered operation's response into, with the spec's schema of it.
type decodeUnit struct {
	model      string
	goType     reflect.Type
	schema     any
	operations []string
}

// variantResult is the outcome of one fixture.
type variantResult struct {
	// label names the choice the fixture makes, such as AttributeType=object, and is
	// empty for the base fixture, which makes every choice at its default.
	label string
	// path is where the choice is in the fixture, and key identifies the choice.
	path, key string
	// alternative is the index of the choice's alternative.
	alternative int
	// depth is the number of choices the fixture makes.
	depth int
	err   error
}

// decodeFailure is a fixture of a unit that failed.
type decodeFailure struct {
	unit    *decodeUnit
	result  variantResult
	allowed string
}

// decodeCheck is the result of the decode check.
type decodeCheck struct {
	units    []*decodeUnit
	fixtures int
	failures []decodeFailure
	// untyped are the covered operations whose response the SDK does not decode
	// into a model: no content, or a request the provider sends itself.
	untyped  []string
	findings []string
}

// checkDecoding decodes fixtures of every response model of the covered
// operations, and returns the failures, with the known ones marked allowed. A
// failure that is not known, and a known failure that no longer fails, are
// findings.
func checkDecoding(s *spec, covered []string, models map[string]reflect.Type,
	known []knownFailure,
) decodeCheck {
	var check decodeCheck
	byKey := map[string]*decodeUnit{}
	for _, id := range covered {
		op := s.byID[id]
		goType := models[op.route().key()]
		if goType == nil || op.response == nil {
			check.untyped = append(check.untyped, id)
			continue
		}
		schemaJSON, err := json.Marshal(op.response)
		if err != nil {
			check.findings = append(check.findings, fmt.Sprintf("%s: the response schema cannot "+
				"be encoded: %v", id, err))
			continue
		}
		key := goType.String() + " " + string(schemaJSON)
		unit := byKey[key]
		if unit == nil {
			unit = &decodeUnit{model: goType.String(), goType: goType, schema: op.response}
			byKey[key] = unit
			check.units = append(check.units, unit)
		}
		unit.operations = append(unit.operations, id)
	}

	allowed := map[string]string{}
	for _, entry := range known {
		if entry.Variant == "" || entry.Note == "" {
			check.findings = append(check.findings, "a known decode failure needs a variant and "+
				"a note")
			continue
		}
		allowed[entry.Variant] = entry.Note
	}
	failing := map[string]bool{}
	for _, unit := range check.units {
		results := checkUnit(s.schemas, unit)
		check.fixtures += len(results)
		for _, result := range results {
			if result.err == nil {
				continue
			}
			failure := decodeFailure{unit: unit, result: result, allowed: allowed[result.label]}
			failing[result.label] = true
			check.failures = append(check.failures, failure)
			if failure.allowed == "" {
				check.findings = append(check.findings, fmt.Sprintf("decode: %s %s: %v",
					unit.model, variantName(result), result.err))
			}
		}
	}
	for _, label := range slices.Sorted(maps.Keys(allowed)) {
		if !failing[label] {
			check.findings = append(check.findings, fmt.Sprintf("stale: the known decode failure "+
				"%s no longer fails; remove it from operations.yaml", label))
		}
	}
	return check
}

func variantName(result variantResult) string {
	switch {
	case result.label == "":
		return "base fixture"
	case result.path == "":
		return result.label + " (at the root)"
	default:
		return fmt.Sprintf("%s (at %s)", result.label, result.path)
	}
}

// checkUnit decodes every fixture of a unit. When the base fixture fails and a
// fixture that changes one choice decodes, that choice's default was the cause,
// so it takes the alternative that decodes and the unit is checked again. The
// base fixture then decodes, and each failure is down to the one choice its
// fixture makes.
func checkUnit(schemas map[string]any, unit *decodeUnit) []variantResult {
	defaults := map[string]int{}
	for {
		results := runVariants(schemas, unit, defaults)
		if results[0].err == nil {
			return results
		}
		fixed := false
		for _, result := range results[1:] {
			if _, set := defaults[result.key]; result.err == nil && result.depth == 1 && !set {
				defaults[result.key] = result.alternative
				fixed = true
				break
			}
		}
		if !fixed {
			return results
		}
	}
}

// runVariants builds and decodes the base fixture, then a fixture for each other
// alternative of each choice the base fixture makes, and so on for the choices
// that only the alternatives of a choice have.
func runVariants(schemas map[string]any, unit *decodeUnit,
	defaults map[string]int,
) []variantResult {
	type pending struct {
		result   variantResult
		explicit map[string]int
		// parentMet are the choices of the fixture this one varies, which that
		// fixture's variants already cover.
		parentMet map[string]bool
	}
	queue := []pending{{explicit: map[string]int{}}}
	var results []variantResult
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		g := &fixtureGen{schemas: schemas, explicit: item.explicit, defaults: defaults}
		value, built := g.generate(unit.schema, location{subject: modelName(unit.goType)})
		result := item.result
		switch {
		case len(g.problems) > 0:
			result.err = fmt.Errorf("cannot build the fixture: %s", strings.Join(g.problems, "; "))
		case !built:
			result.err = errors.New("cannot build the fixture")
		default:
			result.err = decodeInto(unit.goType, value)
		}
		results = append(results, result)

		met := map[string]bool{}
		for _, point := range g.met {
			met[point.key] = true
		}
		for _, point := range g.met {
			if item.parentMet[point.key] {
				continue
			}
			for alternative, label := range point.labels {
				if alternative == point.chosen {
					continue
				}
				explicit := maps.Clone(item.explicit)
				explicit[point.key] = alternative
				queue = append(queue, pending{
					result: variantResult{label: label, path: point.path, key: point.key,
						alternative: alternative, depth: len(explicit)},
					explicit:  explicit,
					parentMet: met,
				})
			}
		}
	}
	return results
}

// modelName names the model a response decodes into, without pointers, slices or
// package, as the subject of a choice the response schema makes inline.
func modelName(goType reflect.Type) string {
	for goType.Kind() == reflect.Pointer || goType.Kind() == reflect.Slice {
		goType = goType.Elem()
	}
	return goType.Name()
}

// decodeInto decodes a fixture into a new value of goType, as the SDK's generated
// client decodes a JSON response into the operation's result.
func decodeInto(goType reflect.Type, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encoding the fixture: %w", err)
	}
	return json.Unmarshal(data, reflect.New(goType).Interface())
}

// location is where a schema is: in the fixture, such as attributes.*.type, and
// in the spec, the component schema and the path in it, such as
// AttributeBlockRead.type. refs are the component schemas being built, to stop at
// a cycle.
type location struct {
	path    string
	subject string
	refs    []string
}

func (l location) property(name string) location {
	return location{path: joinPath(l.path, name), subject: joinPath(l.subject, name), refs: l.refs}
}

func (l location) items() location {
	return location{path: l.path + "[]", subject: l.subject + "[]", refs: l.refs}
}

func joinPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

// choicePoint is a place where the spec offers alternatives. Its key tells apart
// two choices at one path, such as an enum in an anyOf.
type choicePoint struct {
	path, key string
	labels    []string
	chosen    int
}

// fixtureGen builds one fixture from a schema.
type fixtureGen struct {
	schemas map[string]any
	// explicit are the choices this fixture makes, by path; defaults are the
	// alternatives of the other choices that are not the first that can be built.
	explicit, defaults map[string]int
	met                []choicePoint
	problems           []string
}

// generate returns a fixture value for schema, or false when there is nothing to
// put there, as at a cycle. It records what the spec does not let it build in
// problems.
func (g *fixtureGen) generate(schema any, at location) (any, bool) {
	m, ok := schema.(map[string]any)
	if !ok {
		if allowed, isBool := schema.(bool); isBool && allowed {
			return "x", true
		}
		g.problem(at, "the schema is not an object")
		return nil, false
	}
	if ref, ok := m["$ref"].(string); ok {
		name, local := strings.CutPrefix(ref, "#/components/schemas/")
		target, exists := g.schemas[name]
		if !local || !exists {
			g.problem(at, fmt.Sprintf("$ref %s is not a component schema", ref))
			return nil, false
		}
		if slices.Contains(at.refs, name) {
			return nil, false
		}
		refs := append(slices.Clone(at.refs), name)
		return g.generate(target, location{path: at.path, subject: name, refs: refs})
	}
	if parts, ok := m["allOf"].([]any); ok && len(parts) > 0 {
		return g.allOf(parts, at)
	}
	if values, ok := m["enum"].([]any); ok && len(values) > 0 {
		labels := make([]string, len(values))
		for i, value := range values {
			labels[i] = fmt.Sprintf("%s=%v", subject(at), value)
		}
		return values[g.choose(at, "enum", labels, nil)], true
	}
	if value, ok := m["const"]; ok {
		return value, true
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		if alternatives, ok := m[keyword].([]any); ok && len(alternatives) > 0 {
			return g.alternatives(keyword, alternatives, at)
		}
	}
	switch typ := m["type"].(type) {
	case string:
		return g.typed(typ, m, at)
	case []any:
		alternatives := make([]any, len(typ))
		for i, name := range typ {
			alternative := maps.Clone(m)
			alternative["type"] = name
			alternatives[i] = alternative
		}
		return g.alternatives("type", alternatives, at)
	case nil:
		if _, ok := m["properties"]; ok {
			return g.typed("object", m, at)
		}
		if _, ok := m["additionalProperties"]; ok {
			return g.typed("object", m, at)
		}
		return "x", true
	default:
		g.problem(at, fmt.Sprintf("type %v is not a string or a list", typ))
		return nil, false
	}
}

// alternatives builds one of the alternatives of an anyOf, a oneOf or a list of
// types.
func (g *fixtureGen) alternatives(kind string, alternatives []any, at location) (any, bool) {
	labels := make([]string, len(alternatives))
	for i, alternative := range alternatives {
		labels[i] = subject(at) + " as " + describeSchema(alternative)
	}
	chosen := g.choose(at, kind, labels,
		func(i int) bool { return g.buildable(alternatives[i], at) })
	if chosen < 0 {
		return nil, false
	}
	return g.generate(alternatives[chosen], at)
}

// allOf merges the objects built for each part.
func (g *fixtureGen) allOf(parts []any, at location) (any, bool) {
	if len(parts) == 1 {
		return g.generate(parts[0], at)
	}
	merged := map[string]any{}
	for _, part := range parts {
		value, ok := g.generate(part, at)
		if !ok {
			continue
		}
		object, isObject := value.(map[string]any)
		if !isObject {
			g.problem(at, "allOf of schemas that are not all objects")
			return nil, false
		}
		maps.Copy(merged, object)
	}
	return merged, true
}

// typed builds a value of a JSON Schema type.
func (g *fixtureGen) typed(typ string, m map[string]any, at location) (any, bool) {
	switch typ {
	case "object":
		return g.object(m, at), true
	case "array":
		items, ok := m["items"]
		if !ok {
			return []any{"x"}, true
		}
		if value, ok := g.generate(items, at.items()); ok {
			return []any{value}, true
		}
		return []any{}, true
	case "string":
		format, _ := m["format"].(string)
		return stringOfFormat(format), true
	case "integer":
		return 1, true
	case "number":
		return 1.5, true
	case "boolean":
		return true, true
	case "null":
		return nil, true
	default:
		g.problem(at, fmt.Sprintf("type %s is not a JSON Schema type", typ))
		return nil, false
	}
}

// object builds an object with every property, a field the spec does not have
// when it has properties, and one entry when it is a map.
func (g *fixtureGen) object(m map[string]any, at location) map[string]any {
	object := map[string]any{}
	properties, _ := m["properties"].(map[string]any)
	for _, name := range slices.Sorted(maps.Keys(properties)) {
		if value, ok := g.generate(properties[name], at.property(name)); ok {
			object[name] = value
		}
	}
	if len(properties) > 0 {
		object[unknownField] = "x"
	}
	entries, isMap := m["additionalProperties"].(map[string]any)
	if !isMap || len(entries) == 0 {
		patterns, _ := m["patternProperties"].(map[string]any)
		if keys := slices.Sorted(maps.Keys(patterns)); len(keys) > 0 {
			entries, isMap = patterns[keys[0]].(map[string]any)
		}
	}
	switch {
	case isMap && len(entries) > 0:
		if value, ok := g.generate(entries, at.property("*")); ok {
			object[mapKey] = value
		}
	case len(properties) == 0 && m["additionalProperties"] != false:
		object[mapKey] = "x"
	}
	return object
}

// stringOfFormat returns a string in the format, or "x" for a format it does not
// know.
func stringOfFormat(format string) string {
	switch format {
	case "date-time":
		return "2026-01-01T00:00:00Z"
	case "date":
		return "2026-01-01"
	case "uuid":
		return "00000000-0000-4000-8000-000000000000"
	case "email":
		return "user@example.com"
	case "uri", "url":
		return "https://example.com/x"
	default:
		return "x"
	}
}

// choose returns the alternative this fixture takes at a choice point of a kind:
// the explicit one, else the default, else the first that buildable accepts, or
// -1 when it accepts none. The fixture then leaves the value out, and the
// fixtures of the alternatives report why each cannot be built.
func (g *fixtureGen) choose(at location, kind string, labels []string,
	buildable func(int) bool,
) int {
	key := at.path + "|" + kind + "|" + at.subject
	chosen := 0
	if i, ok := g.explicit[key]; ok && i < len(labels) {
		chosen = i
	} else if i, ok := g.defaults[key]; ok && i < len(labels) {
		chosen = i
	} else if buildable != nil {
		chosen = -1
		for i := range labels {
			if buildable(i) {
				chosen = i
				break
			}
		}
	}
	g.met = append(g.met, choicePoint{path: at.path, key: key, labels: labels, chosen: chosen})
	return chosen
}

// buildable reports whether schema can be built at, leaving no trace in the
// fixture's choices or problems.
func (g *fixtureGen) buildable(schema any, at location) bool {
	met, problems := len(g.met), len(g.problems)
	_, ok := g.generate(schema, at)
	built := ok && len(g.problems) == problems
	g.met, g.problems = g.met[:met], g.problems[:problems]
	return built
}

func (g *fixtureGen) problem(at location, text string) {
	g.problems = append(g.problems, subject(at)+": "+text)
}

// subject names a location in the spec, or in the fixture when it is in no
// component schema.
func subject(at location) string {
	switch {
	case at.subject != "":
		return at.subject
	case at.path != "":
		return at.path
	default:
		return "response"
	}
}

// describeSchema names an alternative: its component schema, or its type and
// format.
func describeSchema(schema any) string {
	m, ok := schema.(map[string]any)
	if !ok {
		return fmt.Sprint(schema)
	}
	if ref, ok := m["$ref"].(string); ok {
		return strings.TrimPrefix(ref, "#/components/schemas/")
	}
	description := fmt.Sprint(m["type"])
	if m["type"] == nil {
		description = "schema"
	}
	if format, ok := m["format"].(string); ok {
		description += "/" + format
	}
	return description
}
