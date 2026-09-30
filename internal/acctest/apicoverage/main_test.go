package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// largeSpec returns a spec with enough operations and tags to pass the default
// sentinels.
func largeSpec(operations, tags int) string {
	paths := make([]string, operations)
	for i := range paths {
		paths[i] = fmt.Sprintf(`"/v2/op%d": {"get": {"operationId": "op%d", "tags": ["Tag %d"]}}`,
			i, i, i%tags)
	}
	return `{"openapi": "3.1.0", "info": {"title": "Large", "version": "1"}, "paths": {` +
		strings.Join(paths, ", ") + `}}`
}

func TestRunUpdateLock(t *testing.T) {
	now := func() time.Time {
		return time.Date(2026, 9, 30, 12, 0, 0, 0, time.FixedZone("x", 3600))
	}
	noWalk := func(string) (facts, error) {
		t.Fatal("-update-lock walked the provider")
		return facts{}, nil
	}

	t.Run("vendors and pins the spec", func(t *testing.T) {
		spec := strings.Replace(largeSpec(defaultMinimums.operations, defaultMinimums.tags),
			`"operationId": "op0",`,
			`"operationId": "op0", "description": "Ask someone@example.com.",`, 1)
		dir := writeCoverage(t, testSpec, testTriage)
		if err := os.WriteFile(filepath.Join(dir, specFile), []byte(spec), 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", dir, "-update-lock"}, &stdout, &stderr, noWalk, now)
		if code != exitClean {
			t.Fatalf("exit code %d, stderr %s", code, stderr.String())
		}
		vendored, err := os.ReadFile(filepath.Join(dir, specFile))
		if err != nil {
			t.Fatal(err)
		}
		for _, dropped := range []string{"someone@example.com", `"title"`} {
			if bytes.Contains(vendored, []byte(dropped)) {
				t.Errorf("the vendored spec still has %s", dropped)
			}
		}
		if canonical, err := canonicalSpec(vendored); err != nil ||
			!bytes.Equal(canonical, vendored) {
			t.Errorf("vendoring the vendored spec again changes it: %v", err)
		}
		lock, err := readLock(filepath.Join(dir, lockName))
		if err != nil {
			t.Fatal(err)
		}
		want := lockFile{URL: "https://example.com/openapi.json", SHA256: sha256Hex(vendored),
			FetchedAt: "2026-09-30T11:00:00Z"}
		if lock != want {
			t.Errorf("lock = %+v, want %+v", lock, want)
		}
	})

	t.Run("refuses a spec that fails a sentinel", func(t *testing.T) {
		dir := writeCoverage(t, testSpec, testTriage)
		before, err := os.ReadFile(filepath.Join(dir, lockName))
		if err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		code := run([]string{"-dir", dir, "-update-lock"}, &stdout, &stderr, noWalk, now)
		if code != exitDidNotRun {
			t.Errorf("exit code %d, want %d", code, exitDidNotRun)
		}
		after, err := os.ReadFile(filepath.Join(dir, lockName))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("the lock file changed to %s", after)
		}
		if !strings.Contains(stderr.String(), "the spec has 6 operations") {
			t.Errorf("stderr %q does not say why", stderr.String())
		}
	})
}

func TestRunReport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-dir", t.TempDir()}, &stdout, &stderr,
		func(string) (facts, error) { return facts{}, nil }, time.Now)
	if code != exitDidNotRun || !strings.Contains(stdout.String(), "**DID NOT RUN**") {
		t.Errorf("exit code %d, report:\n%s", code, stdout.String())
	}

	stdout.Reset()
	if code := run([]string{"extra"}, &stdout, &stderr, nil, time.Now); code != exitDidNotRun {
		t.Errorf("an extra argument: exit code %d, want %d", code, exitDidNotRun)
	}

	r := runCheck(t, writeCoverage(t, testSpec, testTriage), testFacts(), "")
	stdout.Reset()
	writeReport(&stdout, r)
	for _, want := range []string{
		"**PASS**",
		"| Sent by the provider, GA and not deprecated | 3 of 4 (75.0%) |",
		"| Provider call sites | 3: 2 through the SDK, 1 built with net/http; 3 distinct calls |",
		"Triage: covered 3, equivalent 1, inline 0, planned 0, excluded 2.",
		"| get_thing | `GET /v2/things/{thing_id}` | permitio_thing, data.permitio_thing | " +
			"Things.Get | no |",
		"Response models: 1, of 2 operations; fixtures: 2; failures: 0, 0 of them known.",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("the report lacks %q:\n%s", want, stdout.String())
		}
	}
}

func TestWithoutDocs(t *testing.T) {
	var value any
	if err := json.Unmarshal([]byte(`{
	  "description": "d", "title": "t",
	  "properties": {"title": {"type": "string", "description": "d", "example": "e"}},
	  "content": {"application/json": {"example": {"id": "1"}, "schema": {"type": "string"}}}
	}`), &value); err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(withoutDocs(value, false))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"content":{"application/json":{"schema":{"type":"string"}}},` +
		`"properties":{"title":{"type":"string"}}}`
	if string(got) != want {
		t.Errorf("withoutDocs = %s, want %s", got, want)
	}
}
