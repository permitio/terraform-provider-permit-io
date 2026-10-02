package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/permitio/terraform-provider-permit-io/internal/acctest/providerschema"
)

const (
	baseFixture        = "testdata/base.json"
	breakingFixture    = "testdata/head-breaking.json"
	nonBreakingFixture = "testdata/head-nonbreaking.json"
	providerGolden     = "../../provider/testdata/provider-schema.json"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	empty := write("empty.json", "")
	truncated := write("truncated.json", `{"resources": {"example_role": {`)
	noResources := write("no-resources.json", `{"provider": {"version": 0}, "resources": {}}`)

	// A case checks the whole output when wantOutput is set, else that it contains
	// wantContains.
	tests := []struct {
		name         string
		args         []string
		wantExit     int
		wantOutput   string
		wantContains string
	}{
		{
			name:     "breaking changes",
			args:     []string{baseFixture, breakingFixture},
			wantExit: exitBreaking,
			wantOutput: `non-breaking  resource example_role: attribute "description" added (optional)
BREAKING      resource example_role: attribute "name" changed from optional to required: ` +
				`configurations that leave it unset now fail
BREAKING      resource example_role: attribute "permissions" removed
BREAKING      resource example_role: attribute "resource" now forces replacement: ` +
				`stringplanmodifier.requiresReplaceIfModifier: If the value of this attribute ` +
				`changes, Terraform will destroy and recreate the resource.
schemadiff: 4 changes, 3 breaking
`,
		},
		{
			name:     "only non-breaking changes",
			args:     []string{baseFixture, nonBreakingFixture},
			wantExit: exitNoBreaking,
			wantOutput: `non-breaking  resource example_role: attribute "description" added (optional)
non-breaking  resource example_role: attribute "name" deprecated: Use description.
schemadiff: 2 changes, 0 breaking
`,
		},
		{
			name:       "no changes",
			args:       []string{baseFixture, baseFixture},
			wantExit:   exitNoBreaking,
			wantOutput: "schemadiff: 0 changes, 0 breaking\n",
		},
		{
			name:         "no arguments",
			args:         nil,
			wantExit:     exitDidNotRun,
			wantContains: "schemadiff: DID NOT RUN: want two arguments",
		},
		{
			name:         "one argument",
			args:         []string{baseFixture},
			wantExit:     exitDidNotRun,
			wantContains: "schemadiff: DID NOT RUN: want two arguments",
		},
		{
			name:         "missing base",
			args:         []string{filepath.Join(dir, "missing.json"), baseFixture},
			wantExit:     exitDidNotRun,
			wantContains: "schemadiff: DID NOT RUN: base: open ",
		},
		{
			name:         "empty head",
			args:         []string{baseFixture, empty},
			wantExit:     exitDidNotRun,
			wantContains: "schemadiff: DID NOT RUN: head: " + empty + ": the input is empty",
		},
		{
			name:         "empty base",
			args:         []string{empty, baseFixture},
			wantExit:     exitDidNotRun,
			wantContains: "schemadiff: DID NOT RUN: base: " + empty + ": the input is empty",
		},
		{
			name:         "truncated head",
			args:         []string{baseFixture, truncated},
			wantExit:     exitDidNotRun,
			wantContains: "schemadiff: DID NOT RUN: head: " + truncated + ": not a provider schema snapshot",
		},
		{
			name:     "head without resources",
			args:     []string{baseFixture, noResources},
			wantExit: exitDidNotRun,
			wantContains: "schemadiff: DID NOT RUN: head: " + noResources +
				": the snapshot has no resources and no data sources",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			exit := run(tt.args, &output)
			if exit != tt.wantExit {
				t.Errorf("exit = %d, want %d", exit, tt.wantExit)
			}
			switch {
			case tt.wantOutput != "" && output.String() != tt.wantOutput:
				t.Errorf("output:\n%s\nwant:\n%s", output.String(), tt.wantOutput)
			case tt.wantOutput == "" && !strings.Contains(output.String(), tt.wantContains):
				t.Errorf("output:\n%s\nwant it to contain:\n%s", output.String(), tt.wantContains)
			}
		})
	}
}

// TestRunOnProviderGolden runs the comparison on the provider's committed golden
// file, unchanged and with one attribute removed.
func TestRunOnProviderGolden(t *testing.T) {
	var output strings.Builder
	if exit := run([]string{providerGolden, providerGolden}, &output); exit != exitNoBreaking {
		t.Fatalf("golden file against itself: exit = %d, want %d; output:\n%s", exit,
			exitNoBreaking, output.String())
	}
	if output.String() != "schemadiff: 0 changes, 0 breaking\n" {
		t.Errorf("golden file against itself: output:\n%s", output.String())
	}

	data, err := os.ReadFile(providerGolden)
	if err != nil {
		t.Fatal(err)
	}
	head, err := providerschema.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := head.Resources["permitio_role"].Attributes["key"]; !ok {
		t.Fatal("the golden file has no permitio_role attribute key to remove")
	}
	delete(head.Resources["permitio_role"].Attributes, "key")
	planted, err := providerschema.Encode(head)
	if err != nil {
		t.Fatal(err)
	}
	headPath := filepath.Join(t.TempDir(), "head.json")
	if err := os.WriteFile(headPath, planted, 0o600); err != nil {
		t.Fatal(err)
	}

	output.Reset()
	if exit := run([]string{providerGolden, headPath}, &output); exit != exitBreaking {
		t.Fatalf("attribute removed: exit = %d, want %d; output:\n%s", exit, exitBreaking,
			output.String())
	}
	want := `BREAKING      resource permitio_role: attribute "key" removed
schemadiff: 1 changes, 1 breaking
`
	if output.String() != want {
		t.Errorf("attribute removed: output:\n%s\nwant:\n%s", output.String(), want)
	}
}
