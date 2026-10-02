package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

// specOperation is one operation of the OpenAPI spec.
type specOperation struct {
	id         string
	method     string
	path       string
	tags       []string
	deprecated bool
	// response is the schema of the first 2xx JSON response, or nil.
	response any
	// raw is the operation object as the spec has it.
	raw map[string]any
}

// ga reports whether the operation is generally available: no tag marks it as an
// early-access (EAP) operation.
func (o specOperation) ga() bool {
	for _, tag := range o.tags {
		if eapTag.MatchString(tag) {
			return false
		}
	}
	return true
}

// active reports whether the operation is GA and not deprecated.
func (o specOperation) active() bool {
	return o.ga() && !o.deprecated
}

// route returns the operation's method and path.
func (o specOperation) route() route {
	return route{method: o.method, path: o.path}
}

var eapTag = regexp.MustCompile(`\bEAP\b`)

// spec is a parsed OpenAPI spec.
type spec struct {
	version    string
	operations []specOperation
	byID       map[string]*specOperation
	byRoute    map[string]*specOperation
	tags       []string
	// schemas are the spec's component schemas by name.
	schemas map[string]any
	// doc is the whole spec as decoded JSON.
	doc map[string]any
}

// httpMethods are the operation keys of an OpenAPI path item.
var httpMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// parseSpec parses an OpenAPI 3 spec. It fails on a spec it cannot read, such as a
// truncated file, and on operations without an ID or with the same ID or route as
// another.
func parseSpec(data []byte) (*spec, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("the spec is not JSON: %w", err)
	}
	openapi, _ := doc["openapi"].(string)
	if !strings.HasPrefix(openapi, "3.") {
		return nil, fmt.Errorf("the spec's openapi version is %q, not 3.x", openapi)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		return nil, errors.New("the spec has no paths object")
	}
	info, _ := doc["info"].(map[string]any)
	s := &spec{
		byID:    map[string]*specOperation{},
		byRoute: map[string]*specOperation{},
		doc:     doc,
	}
	s.version, _ = info["version"].(string)
	components, _ := doc["components"].(map[string]any)
	s.schemas, _ = components["schemas"].(map[string]any)

	tags := map[string]bool{}
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		item, ok := paths[path].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %s is not an object", path)
		}
		for _, method := range httpMethods {
			raw, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			op, err := newSpecOperation(strings.ToUpper(method), path, raw)
			if err != nil {
				return nil, err
			}
			for _, tag := range op.tags {
				tags[tag] = true
			}
			s.operations = append(s.operations, op)
		}
	}
	for i := range s.operations {
		op := &s.operations[i]
		if other, dup := s.byID[op.id]; dup {
			return nil, fmt.Errorf("operation ID %s is used by %s %s and %s %s",
				op.id, other.method, other.path, op.method, op.path)
		}
		s.byID[op.id] = op
		key := op.route().key()
		if other, dup := s.byRoute[key]; dup {
			return nil, fmt.Errorf("%s %s and %s %s are the same route", other.method, other.path,
				op.method, op.path)
		}
		s.byRoute[key] = op
	}
	s.tags = slices.Sorted(maps.Keys(tags))
	return s, nil
}

func newSpecOperation(method, path string, raw map[string]any) (specOperation, error) {
	op := specOperation{method: method, path: path, raw: raw}
	op.id, _ = raw["operationId"].(string)
	if op.id == "" {
		return op, fmt.Errorf("%s %s has no operationId", method, path)
	}
	tags, _ := raw["tags"].([]any)
	for _, tag := range tags {
		if name, ok := tag.(string); ok {
			op.tags = append(op.tags, name)
		}
	}
	op.deprecated, _ = raw["deprecated"].(bool)
	responses, _ := raw["responses"].(map[string]any)
	for _, code := range slices.Sorted(maps.Keys(responses)) {
		if !strings.HasPrefix(code, "2") {
			continue
		}
		response, _ := responses[code].(map[string]any)
		content, _ := response["content"].(map[string]any)
		media, _ := content["application/json"].(map[string]any)
		if schema, ok := media["schema"]; ok {
			op.response = schema
			break
		}
	}
	return op, nil
}

// route is an HTTP method and a path template.
type route struct {
	method, path string
}

func (r route) String() string {
	return r.method + " " + r.path
}

// pathParameter matches a path template parameter.
var pathParameter = regexp.MustCompile(`\{[^}]*\}`)

// key identifies the route with its parameter names left out, since the spec, the
// SDK and mockpermit may name the same parameter differently.
func (r route) key() string {
	return r.method + " " + pathParameter.ReplaceAllString(r.path, "{}")
}

// parseRoute parses "METHOD /path", the form of an http.ServeMux pattern with a
// method.
func parseRoute(pattern string) (route, error) {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok || method == "" || !strings.HasPrefix(path, "/") {
		return route{}, fmt.Errorf("%q is not written METHOD /path", pattern)
	}
	return route{method: method, path: path}, nil
}

// lockFile pins the vendored spec: where it came from, the SHA-256 of its vendored
// form and when it was fetched.
type lockFile struct {
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	FetchedAt string `json:"fetched_at"`
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// checkLock returns an error unless data is the spec the lock file pins.
func checkLock(lock lockFile, data []byte) error {
	if lock.URL == "" || lock.FetchedAt == "" {
		return errors.New("the lock file needs url, sha256 and fetched_at")
	}
	if got := sha256Hex(data); got != lock.SHA256 {
		return fmt.Errorf("the spec's SHA-256 is %s, but the lock file pins %s; after replacing "+
			"the spec, run apicoverage -update-lock", got, lock.SHA256)
	}
	return nil
}

// docKeys are the keys that only document a spec: descriptions, summaries, titles
// and examples. The live spec regenerates the UUIDs in its examples on every
// start, so a comparison that kept them would find drift every week.
var docKeys = map[string]bool{
	"description": true, "summary": true, "title": true, "example": true, "examples": true,
}

// canonicalSpec returns the form coverage/openapi.json vendors a spec in: without
// the keys in docKeys, indented, with sorted keys. The dropped keys hold prose and
// examples, with sample people, company names and UUIDs that change on every
// fetch, so the vendored spec carries none of them and refetching an unchanged
// spec pins the same SHA-256.
func canonicalSpec(data []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var doc any
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("the spec is not JSON: %w", err)
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(withoutDocs(doc, false)); err != nil {
		return nil, fmt.Errorf("encoding the spec: %w", err)
	}
	return out.Bytes(), nil
}

// withoutDocs returns a copy of a decoded JSON value without the keys in docKeys.
// A key of a map of names, such as a schema property called "title", is kept.
func withoutDocs(value any, keysAreNames bool) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if docKeys[key] && !keysAreNames {
				continue
			}
			out[key] = withoutDocs(item, namesKeyed[key] && !keysAreNames)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = withoutDocs(item, false)
		}
		return out
	default:
		return value
	}
}

// namesKeyed are the spec keys whose values map names, not keywords, to objects.
var namesKeyed = map[string]bool{
	"paths": true, "schemas": true, "properties": true, "responses": true, "content": true,
	"patternProperties": true, "securitySchemes": true, "parameters": true,
}

// drift is how a spec differs from the vendored one, leaving documentation aside.
type drift struct {
	added, removed, changed []string
	schemasAdded            []string
	schemasRemoved          []string
	schemasChanged          []string
	other                   bool
}

func (d drift) empty() bool {
	return len(d.added)+len(d.removed)+len(d.changed)+len(d.schemasAdded)+
		len(d.schemasRemoved)+len(d.schemasChanged) == 0 && !d.other
}

// diffSpecs compares two specs, leaving documentation aside: the operations by ID,
// the component schemas by name, and whether anything else differs.
func diffSpecs(old, current *spec) drift {
	var d drift
	for _, op := range current.operations {
		prior, ok := old.byID[op.id]
		switch {
		case !ok:
			d.added = append(d.added, describeOperation(op))
		case !reflect.DeepEqual(withoutDocs(prior.raw, false), withoutDocs(op.raw, false)) ||
			prior.route() != op.route():
			d.changed = append(d.changed, describeOperation(op))
		}
	}
	for _, op := range old.operations {
		if _, ok := current.byID[op.id]; !ok {
			d.removed = append(d.removed, describeOperation(op))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(current.schemas)) {
		prior, ok := old.schemas[name]
		switch {
		case !ok:
			d.schemasAdded = append(d.schemasAdded, name)
		case !reflect.DeepEqual(withoutDocs(prior, false),
			withoutDocs(current.schemas[name], false)):
			d.schemasChanged = append(d.schemasChanged, name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(old.schemas)) {
		if _, ok := current.schemas[name]; !ok {
			d.schemasRemoved = append(d.schemasRemoved, name)
		}
	}
	if d.empty() {
		d.other = !reflect.DeepEqual(withoutDocs(old.doc, false), withoutDocs(current.doc, false))
	}
	return d
}

// describeOperation names an operation with its route, and marks one that is not
// GA or is deprecated.
func describeOperation(op specOperation) string {
	text := fmt.Sprintf("%s (%s %s)", op.id, op.method, op.path)
	if !op.ga() {
		text += ", not GA"
	}
	if op.deprecated {
		text += ", deprecated"
	}
	return text
}
