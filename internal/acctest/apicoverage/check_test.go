package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// testSpec is a small spec: things with a color enum, a beta operation that is not
// GA, and a deprecated one.
const testSpec = `{
  "openapi": "3.1.0",
  "info": {"title": "Test API", "version": "1"},
  "paths": {
    "/v2/things": {
      "get": {"operationId": "list_things", "tags": ["Things"], "responses": {"200": {
        "description": "ok", "content": {"application/json": {"schema": {
          "type": "array", "items": {"$ref": "#/components/schemas/ThingRead"}}}}}}},
      "post": {"operationId": "create_thing", "tags": ["Things"], "responses": {"200": {
        "content": {"application/json": {"schema": {"$ref": "#/components/schemas/ThingRead"}}}}}}
    },
    "/v2/things/{thing_id}": {
      "get": {"operationId": "get_thing", "tags": ["Things"], "summary": "Get a thing",
        "responses": {"200": {"content": {"application/json": {"schema": {
          "$ref": "#/components/schemas/ThingRead"}}}}}},
      "delete": {"operationId": "delete_thing", "tags": ["Things"], "responses": {"204": {}}}
    },
    "/v2/beta": {"get": {"operationId": "beta_op", "tags": ["Beta (EAP)"], "responses": {}}},
    "/v2/old": {"get": {"operationId": "old_op", "tags": ["Things"], "deprecated": true,
      "responses": {}}}
  },
  "components": {"schemas": {
    "Color": {"type": "string", "enum": ["red", "green"]},
    "ThingRead": {"type": "object", "properties": {
      "key": {"type": "string", "example": "a-key"},
      "color": {"allOf": [{"$ref": "#/components/schemas/Color"}]}}}
  }}
}`

const testTriage = `operations:
  list_things: {status: equivalent, surface: [permitio_thing], reason: list-variant}
  create_thing: {status: covered, surface: [permitio_thing], calls: [Things.Create]}
  get_thing: {status: covered, surface: [permitio_thing, data.permitio_thing], calls: [Things.Get]}
  delete_thing: {status: covered, surface: [permitio_thing], calls: ["things.remove (HTTP)"]}
  beta_op: {status: excluded, reason: not-ga}
  old_op: {status: excluded, reason: deprecated}
`

// testColor rejects the values it does not know, as the SDK's enums do.
type testColor string

func (c *testColor) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value != "red" && value != "green" {
		return fmt.Errorf("%s is not a valid Color", value)
	}
	*c = testColor(value)
	return nil
}

type testThing struct {
	Key   string    `json:"key"`
	Color testColor `json:"color"`
}

// testFacts are call sites that send create_thing, get_thing and delete_thing.
func testFacts() facts {
	return facts{
		sites: []callSite{
			{position: "things/client.go:10:2", pkg: "things", call: "Things.Create", sdk: true,
				routes: []route{{"POST", "/v2/things"}}},
			{position: "things/client.go:20:2", pkg: "things", call: "Things.Get", sdk: true,
				routes: []route{{"GET", "/v2/things/{id}"}}},
			{position: "things/client.go:30:2", pkg: "things", call: "things.remove (HTTP)",
				routes: []route{{"DELETE", "/v2/things/{thing_id}"}}},
		},
		clientOperations: 3,
		models: map[string]reflect.Type{
			route{"POST", "/v2/things"}.key():    reflect.TypeFor[*testThing](),
			route{"GET", "/v2/things/{x}"}.key(): reflect.TypeFor[*testThing](),
			route{"GET", "/v2/things"}.key():     reflect.TypeFor[[]testThing](),
		},
		surfaces: map[string][]string{
			".":      {"provider"},
			"things": {"permitio_thing", "data.permitio_thing"},
		},
		resources: 1, dataSources: 1,
	}
}

var testMinimums = minimums{operations: 5, tags: 2, sdkCallSites: 1, clientOperations: 3,
	resources: 1, dataSources: 1}

// writeCoverage writes a coverage directory with the spec, in its vendored form
// when it is JSON, pinned by its lock.
func writeCoverage(t *testing.T, spec, triage string) string {
	t.Helper()
	dir := t.TempDir()
	if canonical, err := canonicalSpec([]byte(spec)); err == nil {
		spec = string(canonical)
	}
	lock := fmt.Sprintf(`{"url": "https://example.com/openapi.json", "sha256": %q, `+
		`"fetched_at": "2026-01-01T00:00:00Z"}`, sha256Hex([]byte(spec)))
	for name, content := range map[string]string{
		specFile: spec, lockName: lock, triageName: triage,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runCheck(t *testing.T, dir string, f facts, live string) *runResult {
	t.Helper()
	gather := func(string) (facts, error) { return f, nil }
	return check(options{dir: dir, root: ".", live: live}, gather, testMinimums)
}

// wantResult fails the test unless the result has the exit code and, for each of
// want, a finding or reason it did not run that contains it.
func wantResult(t *testing.T, r *runResult, code int, want ...string) {
	t.Helper()
	if got := r.exitCode(); got != code {
		t.Errorf("exit code = %d, want %d; findings %q, not run %q", got, code, r.findings,
			r.notRun)
	}
	all := strings.Join(append(slices.Clone(r.findings), r.notRun...), "\n")
	for _, text := range want {
		if !strings.Contains(all, text) {
			t.Errorf("no finding contains %q; findings:\n%s", text, all)
		}
	}
}

// addOperation returns the test spec with one more operation.
func addOperation(operation string) string {
	return strings.Replace(testSpec, `"/v2/beta":`, operation+`, "/v2/beta":`, 1)
}

// newOperation is PUT /v2/new, put_new, with a tag.
func newOperation(tag string) string {
	return fmt.Sprintf(`"/v2/new": {"put": {"operationId": "put_new", "tags": [%q]}}`, tag)
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name   string
		spec   string
		triage string
		facts  func(f *facts)
		code   int
		want   []string
	}{
		{name: "clean", code: exitClean},
		{
			name: "a new GA operation is untriaged",
			spec: addOperation(newOperation("Things")),
			code: exitFindings, want: []string{"untriaged: put_new (PUT /v2/new)"},
		},
		{
			name: "a new operation that is not GA is untriaged",
			spec: addOperation(newOperation("New (EAP)")),
			code: exitFindings, want: []string{"untriaged: put_new (PUT /v2/new), not GA"},
		},
		{
			name:  "a covered operation lost its call site",
			facts: func(f *facts) { f.sites = f.sites[1:] },
			code:  exitFindings,
			want:  []string{"stale: create_thing is triaged covered, but no call site sends it"},
		},
		{
			name: "a call sends a route the spec does not have",
			facts: func(f *facts) {
				f.sites[0].routes = append(f.sites[0].routes, route{"PUT", "/v2/things"})
			},
			code: exitFindings,
			want: []string{"things/client.go:10:2: Things.Create sends PUT /v2/things, " +
				"which is not in the spec"},
		},
		{
			name:  "an operation that is not triaged covered is sent",
			facts: func(f *facts) { f.sites[0].routes = []route{{"GET", "/v2/things"}} },
			code:  exitFindings,
			want: []string{"list_things is triaged equivalent, but Things.Create sends it",
				"stale: create_thing is triaged covered"},
		},
		{
			name:   "a covered operation lists a call that does not send it",
			triage: strings.Replace(testTriage, "calls: [Things.Get]", "calls: [Things.Read]", 1),
			code:   exitFindings,
			want: []string{"get_thing lists the call Things.Read, which does not send it",
				"get_thing is sent by Things.Get; add the call to its calls"},
		},
		{
			name:   "an entry for an operation the spec does not have",
			triage: testTriage + "  gone_op: {status: excluded, reason: not-planned}\n",
			code:   exitFindings,
			want:   []string{"stale: operations.yaml triages gone_op"},
		},
		{
			name: "an entry with a reason for another status",
			triage: strings.Replace(testTriage, "reason: list-variant", "reason: not-planned",
				1),
			code: exitFindings,
			want: []string{"list_things: reason not-planned does not explain a equivalent"},
		},
		{
			name: "an operation excluded as not-ga is GA",
			spec: strings.Replace(testSpec, `"Beta (EAP)"`, `"Beta"`, 1),
			code: exitFindings,
			want: []string{"stale: beta_op is excluded as not-ga, but the spec has it GA"},
		},
		{
			name: "an operation excluded as deprecated is not deprecated",
			spec: strings.Replace(testSpec, `"deprecated": true`, `"deprecated": false`, 1),
			code: exitFindings,
			want: []string{"stale: old_op is excluded as deprecated, but the spec does not " +
				"deprecate it"},
		},
		{
			name: "a deprecated operation excluded for another reason",
			triage: strings.Replace(testTriage, "old_op: {status: excluded, reason: deprecated}",
				"old_op: {status: excluded, reason: not-planned}", 1),
			code: exitFindings,
			want: []string{"old_op is deprecated in the spec; exclude it as deprecated"},
		},
		{
			name: "an operation that is not GA excluded for another reason",
			triage: strings.Replace(testTriage, "beta_op: {status: excluded, reason: not-ga}",
				"beta_op: {status: excluded, reason: not-planned}", 1),
			code: exitFindings,
			want: []string{"beta_op is not GA in the spec; exclude it as not-ga"},
		},
		{
			name: "a covered operation the spec deprecates",
			spec: strings.Replace(testSpec, `"summary": "Get a thing"`, `"deprecated": true`, 1),
			code: exitFindings,
			want: []string{"get_thing is covered, but the spec has it deprecated"},
		},
		{
			name: "a covered operation the spec deprecates, with a ticket",
			spec: strings.Replace(testSpec, `"summary": "Get a thing"`, `"deprecated": true`, 1),
			triage: strings.Replace(testTriage, "calls: [Things.Get]",
				"calls: [Things.Get], ticket: PER-1", 1),
			code: exitClean,
		},
		{
			name: "a covered operation the spec moves to early access",
			spec: strings.Replace(testSpec, `["Things"], "responses": {"204"`,
				`["Things (EAP)"], "responses": {"204"`, 1),
			code: exitFindings,
			want: []string{"delete_thing is covered, but the spec has it not GA"},
		},
		{
			name: "a surface the provider does not have",
			triage: strings.Replace(testTriage, "surface: [permitio_thing], reason",
				"surface: [permitio_other], reason", 1),
			code: exitFindings,
			want: []string{"list_things: surface permitio_other is not a resource or data source"},
		},
		{
			name: "a covered operation's surface is not where it is sent",
			facts: func(f *facts) {
				f.surfaces["other"] = []string{"permitio_other"}
			},
			triage: strings.Replace(testTriage,
				"create_thing: {status: covered, surface: [permitio_thing]",
				"create_thing: {status: covered, surface: [permitio_other]", 1),
			code: exitFindings,
			want: []string{"create_thing: none of its surfaces permitio_other is in a package " +
				"that sends it"},
		},
		{
			name: "the SDK rejects an enum value the spec gained",
			spec: strings.Replace(testSpec, `["red", "green"]`, `["red", "green", "blue"]`, 1),
			code: exitFindings,
			want: []string{
				"decode: *main.testThing Color=blue (at color): blue is not a valid Color",
			},
		},
		{
			name: "a known decode failure",
			spec: strings.Replace(testSpec, `["red", "green"]`, `["red", "green", "blue"]`, 1),
			triage: testTriage + "known_decode_failures:\n" +
				"  - {variant: Color=blue, note: Test ticket pending.}\n",
			code: exitClean,
		},
		{
			name: "a known decode failure that no longer fails",
			triage: testTriage + "known_decode_failures:\n" +
				"  - {variant: Color=blue, note: Test ticket pending.}\n",
			code: exitFindings,
			want: []string{"stale: the known decode failure Color=blue no longer fails"},
		},
		{
			name: "the problems of the walk are findings",
			facts: func(f *facts) {
				f.problems = []string{"x.go:1:1: mockpermit has no route for x.y (HTTP)"}
			},
			code: exitFindings, want: []string{"mockpermit has no route for x.y (HTTP)"},
		},
		{
			name: "a truncated spec did not run",
			spec: testSpec[:len(testSpec)/2],
			code: exitDidNotRun, want: []string{"the spec is not JSON"},
		},
		{
			name: "a spec with too few operations did not run",
			code: exitDidNotRun, want: []string{"the spec has 6 operations, expected at least 7"},
		},
		{
			name:   "an unknown key in the triage file did not run",
			triage: testTriage + "  old_op2: {status: excluded, reasons: deprecated}\n",
			code:   exitDidNotRun, want: []string{"field reasons not found"},
		},
		{
			name:   "an operation triaged twice did not run",
			triage: testTriage + "  old_op: {status: excluded, reason: deprecated}\n",
			code:   exitDidNotRun, want: []string{`mapping key "old_op" already defined`},
		},
		{
			name:  "too few resources did not run",
			facts: func(f *facts) { f.resources = 0 },
			code:  exitDidNotRun, want: []string{"found 0 resources, expected at least 1"},
		},
		{
			name:  "too few data sources did not run",
			facts: func(f *facts) { f.dataSources = 0 },
			code:  exitDidNotRun, want: []string{"found 0 data sources, expected at least 1"},
		},
		{
			name:  "too few SDK call sites did not run",
			facts: func(f *facts) { f.sites = f.sites[2:] },
			code:  exitDidNotRun,
			want:  []string{"found 0 SDK call sites in the provider, expected at least 1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, triage := tt.spec, tt.triage
			if spec == "" {
				spec = testSpec
			}
			if triage == "" {
				triage = testTriage
			}
			f := testFacts()
			if tt.facts != nil {
				tt.facts(&f)
			}
			least := testMinimums
			if strings.HasPrefix(tt.name, "a spec with too few operations") {
				least.operations = 7
			}
			gather := func(string) (facts, error) { return f, nil }
			r := check(options{dir: writeCoverage(t, spec, triage), root: "."}, gather, least)
			wantResult(t, r, tt.code, tt.want...)
		})
	}
}

func TestCheckDidNotRunOnItsInputs(t *testing.T) {
	t.Run("the spec is not the one the lock pins", func(t *testing.T) {
		dir := writeCoverage(t, testSpec, testTriage)
		changed := []byte(testSpec + " ")
		if err := os.WriteFile(filepath.Join(dir, specFile), changed, 0o600); err != nil {
			t.Fatal(err)
		}
		wantResult(t, runCheck(t, dir, testFacts(), ""), exitDidNotRun,
			"but the lock file pins", "apicoverage -update-lock")
	})
	t.Run("the spec is pinned but not in the vendored form", func(t *testing.T) {
		dir := writeCoverage(t, testSpec, testTriage)
		lock := fmt.Sprintf(`{"url": "https://example.com/openapi.json", "sha256": %q, `+
			`"fetched_at": "2026-01-01T00:00:00Z"}`, sha256Hex([]byte(testSpec)))
		for name, content := range map[string]string{specFile: testSpec, lockName: lock} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		wantResult(t, runCheck(t, dir, testFacts(), ""), exitDidNotRun,
			"is not in the form apicoverage -update-lock vendors")
	})
	for field, lock := range map[string]string{
		"url":        `{"sha256": %q, "fetched_at": "2026-01-01T00:00:00Z"}`,
		"fetched_at": `{"url": "https://example.com/openapi.json", "sha256": %q}`,
	} {
		t.Run("the lock has no "+field, func(t *testing.T) {
			dir := writeCoverage(t, testSpec, testTriage)
			data := fmt.Sprintf(lock, sha256Hex([]byte(testSpec)))
			if err := os.WriteFile(filepath.Join(dir, lockName), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			wantResult(t, runCheck(t, dir, testFacts(), ""), exitDidNotRun,
				"the lock file needs url, sha256 and fetched_at")
		})
	}
	t.Run("the walk failed", func(t *testing.T) {
		gather := func(string) (facts, error) { return facts{}, errors.New("loading failed") }
		r := check(options{dir: writeCoverage(t, testSpec, testTriage), root: "."}, gather,
			testMinimums)
		wantResult(t, r, exitDidNotRun, "loading failed")
	})
	t.Run("a live spec that is truncated", func(t *testing.T) {
		live := filepath.Join(t.TempDir(), "live.json")
		if err := os.WriteFile(live, []byte(testSpec[:100]), 0o600); err != nil {
			t.Fatal(err)
		}
		wantResult(t, runCheck(t, writeCoverage(t, testSpec, testTriage), testFacts(), live),
			exitDidNotRun, "the live spec: the spec is not JSON")
	})
}

func TestCheckLive(t *testing.T) {
	writeLive := func(t *testing.T, spec string) string {
		t.Helper()
		live := filepath.Join(t.TempDir(), "live.json")
		if err := os.WriteFile(live, []byte(spec), 0o600); err != nil {
			t.Fatal(err)
		}
		return live
	}
	dir := writeCoverage(t, testSpec, testTriage)

	t.Run("documentation changed only", func(t *testing.T) {
		live := strings.ReplaceAll(testSpec, `"a-key"`, `"another-key"`)
		live = strings.ReplaceAll(live, `"Get a thing"`, `"Read a thing"`)
		r := runCheck(t, dir, testFacts(), writeLive(t, live))
		wantResult(t, r, exitClean)
		if r.drift == nil || !r.drift.empty() {
			t.Errorf("drift = %+v, want none", r.drift)
		}
	})
	t.Run("an operation added, one changed and a schema changed", func(t *testing.T) {
		live := addOperation(newOperation("Things"))
		live = strings.Replace(live, `"deprecated": true`, `"deprecated": false`, 1)
		live = strings.Replace(live, `["red", "green"]`, `["red", "green", "blue"]`, 1)
		r := runCheck(t, dir, testFacts(), writeLive(t, live))
		wantResult(t, r, exitFindings, "drift: the live spec differs", "untriaged: put_new",
			"Color=blue")
		want := drift{
			added:          []string{"put_new (PUT /v2/new)"},
			changed:        []string{"old_op (GET /v2/old)"},
			schemasChanged: []string{"Color"},
		}
		if r.drift == nil || !reflect.DeepEqual(*r.drift, want) {
			t.Errorf("drift = %+v, want %+v", r.drift, want)
		}
	})
	t.Run("only a part other than operations and schemas changed", func(t *testing.T) {
		live := strings.Replace(testSpec, `"components": {`,
			`"components": {"securitySchemes": {"key": {"type": "http", "scheme": "bearer"}}, `, 1)
		r := runCheck(t, dir, testFacts(), writeLive(t, live))
		wantResult(t, r, exitFindings, "drift: the live spec differs")
		if r.drift == nil || !reflect.DeepEqual(*r.drift, drift{other: true}) {
			t.Errorf("drift = %+v, want other parts changed only", r.drift)
		}
	})
}

func TestTriageEntryValidate(t *testing.T) {
	tests := []struct {
		entry triageEntry
		want  []string
	}{
		{entry: triageEntry{
			Status: "covered", Surface: []string{"permitio_x"}, Calls: []string{"X.Get"},
		}},
		{entry: triageEntry{Status: "done"}, want: []string{`status "done" is not one of`}},
		{
			entry: triageEntry{Status: "covered", Reason: "not-ga"},
			want: []string{"a covered operation needs a surface",
				"calls are listed for covered operations, and only for them",
				"a covered operation takes no reason"},
		},
		{
			entry: triageEntry{Status: "excluded", Reason: "not-ga", Calls: []string{"X.Get"}},
			want:  []string{"calls are listed for covered operations, and only for them"},
		},
		{
			entry: triageEntry{Status: "inline", Surface: []string{"permitio_resource"}},
			want:  []string{"an inline operation's surface names an attribute"},
		},
		{entry: triageEntry{Status: "inline", Surface: []string{"permitio_resource.actions"}}},
		{
			entry: triageEntry{Status: "planned", Reason: "new-resource"},
			want:  []string{"a planned operation needs a ticket"},
		},
		{
			entry: triageEntry{Status: "planned", Reason: "new-resource",
				Ticket: "https://example.com/PER-1"},
			want: []string{"is not a ticket ID"},
		},
		{entry: triageEntry{Status: "planned", Reason: "new-resource", Ticket: "PER-1"}},
		{
			entry: triageEntry{Status: "excluded", Reason: "because"},
			want:  []string{`reason "because" is not in the vocabulary`},
		},
	}
	for _, tt := range tests {
		got := tt.entry.validate()
		if len(got) != len(tt.want) {
			t.Errorf("%+v: problems %q, want %d matching %q", tt.entry, got, len(tt.want), tt.want)
			continue
		}
		for i, want := range tt.want {
			if !strings.Contains(got[i], want) {
				t.Errorf("%+v: problem %q does not contain %q", tt.entry, got[i], want)
			}
		}
	}
}

func TestParseSpec(t *testing.T) {
	s, err := parseSpec([]byte(testSpec))
	if err != nil {
		t.Fatal(err)
	}
	var ga, active []string
	for _, op := range s.operations {
		if op.ga() {
			ga = append(ga, op.id)
		}
		if op.active() {
			active = append(active, op.id)
		}
	}
	slices.Sort(ga)
	slices.Sort(active)
	wantGA := []string{"create_thing", "delete_thing", "get_thing", "list_things", "old_op"}
	if !slices.Equal(ga, wantGA) {
		t.Errorf("GA operations = %q, want %q", ga, wantGA)
	}
	if want := slices.DeleteFunc(slices.Clone(wantGA), func(id string) bool {
		return id == "old_op"
	}); !slices.Equal(active, want) {
		t.Errorf("active operations = %q, want %q", active, want)
	}
	if got := s.byRoute[route{"GET", "/v2/things/{other_name}"}.key()]; got == nil ||
		got.id != "get_thing" {
		t.Errorf("the route with another parameter name found %v, want get_thing", got)
	}
	if want := []string{"Beta (EAP)", "Things"}; !slices.Equal(s.tags, want) {
		t.Errorf("tags = %q, want %q", s.tags, want)
	}

	for name, bad := range map[string]string{
		"not JSON":        `{"openapi": "3.1.0", "paths": {`,
		"not OpenAPI 3":   `{"swagger": "2.0", "paths": {}}`,
		"no operation ID": `{"openapi": "3.1.0", "paths": {"/a": {"get": {}}}}`,
		"an ID used twice": `{"openapi": "3.1.0", "paths": {"/a": {
			"get": {"operationId": "x"}, "put": {"operationId": "x"}}}}`,
		"one route twice": `{"openapi": "3.1.0", "paths": {
			"/a/{x}": {"get": {"operationId": "x"}}, "/a/{y}": {"get": {"operationId": "y"}}}}`,
		"a path not object": `{"openapi": "3.1.0", "paths": {"/a": []}}`,
	} {
		if _, err := parseSpec([]byte(bad)); err == nil {
			t.Errorf("%s: parseSpec succeeded, want an error", name)
		}
	}
}
