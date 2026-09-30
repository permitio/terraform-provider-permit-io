//go:build unix

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeRun is a gate run against fake tools: diff returns the fixture, go list
// finds two packages, one of them main in a subdirectory, and gremlins writes
// results (when set) and appends events (when set). git diff as gremlins runs it
// returns gremlinsDiff when set, and the fixture otherwise.
type fakeRun struct {
	diff         string
	gremlinsDiff *string
	results      string
	events       string
	gremlinsExit int
	probeErr     error
	self         string
	goPath       string
	calls        int
	env          []string
}

func (f *fakeRun) tools() tools {
	self := f.self
	if self == "" {
		self = "/opt/mutationgate"
	}
	goPath := f.goPath
	if goPath == "" {
		goPath = "/usr/local/go/bin/go"
	}
	return tools{
		self:   self,
		goPath: goPath,
		diff:   func(string) ([]byte, error) { return []byte(f.diff), nil },
		gremlinsDiff: func(string, []string) ([]byte, error) {
			if f.gremlinsDiff != nil {
				return []byte(*f.gremlinsDiff), nil
			}
			return []byte(f.diff), nil
		},
		packages: func() (string, string, []goPackage, error) {
			return "m", "/repo", []goPackage{
				{dir: "/repo/pkg", importPath: "m/pkg", name: "pkg"},
				{dir: "/repo/cmd/tool", importPath: "m/cmd/tool", name: "main"},
			}, nil
		},
		probe:    func(gateOptions) (int64, error) { return 1600, f.probeErr },
		gremlins: f.gremlins,
	}
}

func (f *fakeRun) gremlins(_ string, args, env []string) (int, error) {
	f.calls++
	f.env = env
	if f.results != "" {
		out := args[slices.Index(args, "--output")+1]
		if err := os.WriteFile(out, []byte(f.results), 0o600); err != nil {
			return 0, err
		}
	}
	if f.events != "" {
		// The gate appends its -exec to any GOFLAGS it runs with, as when these
		// tests run under the gate itself, so its -log is the last one.
		goflags := envValue(env, "GOFLAGS")
		log := goflags[strings.LastIndex(goflags, "-log=")+len("-log="):]
		log = strings.TrimSuffix(log, "'")
		if err := os.WriteFile(log, []byte(f.events), 0o600); err != nil {
			return 0, err
		}
	}
	return f.gremlinsExit, nil
}

func envValue(env []string, key string) string {
	i := slices.IndexFunc(env, func(kv string) bool { return strings.HasPrefix(kv, key+"=") })
	if i < 0 {
		return ""
	}
	return strings.TrimPrefix(env[i], key+"=")
}

const newFileDiff = "--- /dev/null\n+++ b/pkg/new.go\n@@ -0,0 +1,3 @@\n" +
	"+package pkg\n+\n+func n() {}\n"

func results(mutations ...string) string {
	return `{"go_module":"m","files":[{"file_name":"pkg/new.go","mutations":[` +
		strings.Join(mutations, ",") + `]}]}`
}

func mutant(line int, status string) string {
	return fmt.Sprintf(`{"type":"CONDITIONALS_NEGATION","status":%q,"line":%d,"column":2}`,
		status, line)
}

func TestRunGate(t *testing.T) {
	base := []string{"-base", "main"}
	emptyDiff := ""
	tests := []struct {
		name        string
		args        []string
		run         fakeRun
		wantExit    int
		wantOutput  []string
		wantSummary []string
		noGremlins  bool
		noSummary   bool
	}{
		{
			name:     "no Go line changed",
			args:     base,
			run:      fakeRun{diff: "--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-a\n+b\n"},
			wantExit: exitPassed, noGremlins: true,
			wantOutput: []string{
				"No non-test Go line was added or changed since the merge base with main",
			},
		},
		{
			name: "only a package gremlins cannot test changed",
			args: base,
			run: fakeRun{
				diff: "--- a/cmd/tool/main.go\n+++ b/cmd/tool/main.go\n@@ -1 +1 @@\n-a\n+b\n",
			},
			wantExit: exitPassed, noGremlins: true,
			wantOutput: []string{"only in packages gremlins cannot test", "- `cmd/tool/main.go`"},
		},
		{
			name: "the memory limit is not enforced",
			args: base,
			run: fakeRun{
				diff: newFileDiff, probeErr: errors.New("the limit is not enforced"),
			},
			wantExit: exitDidNotRun, noGremlins: true,
			wantOutput:  []string{"memory limit check: the limit is not enforced", "DID NOT RUN"},
			wantSummary: []string{"**DID NOT RUN**: memory limit check: the limit is not enforced"},
		},
		{
			name:     "a wrong command line",
			args:     []string{"-base", "main", "-workers", "0"},
			run:      fakeRun{diff: newFileDiff},
			wantExit: exitDidNotRun, noGremlins: true, noSummary: true,
			wantOutput: []string{"-workers 0"},
		},
		{
			name:     "an executable path GOFLAGS cannot carry",
			args:     base,
			run:      fakeRun{diff: newFileDiff, self: "/my dir/mutationgate"},
			wantExit: exitDidNotRun, noGremlins: true, wantOutput: []string{"GOFLAGS and the go shim"},
		},
		{
			name:     "a go command path the go shim cannot carry",
			args:     base,
			run:      fakeRun{diff: newFileDiff, goPath: "/it's/go"},
			wantExit: exitDidNotRun, noGremlins: true, wantOutput: []string{`go command "/it's/go"`},
		},
		{
			name:     "git diff as gremlins runs it is empty",
			args:     base,
			run:      fakeRun{diff: newFileDiff, gremlinsDiff: &emptyDiff},
			wantExit: exitDidNotRun, noGremlins: true,
			wantOutput: []string{"as gremlins runs it reports changed lines in 0 Go files, " +
				"git diff -U0 --no-ext-diff in 1"},
			wantSummary: []string{"**DID NOT RUN**: git diff --merge-base main as gremlins runs it"},
		},
		{
			name: "gremlins fails",
			args: base,
			run: fakeRun{diff: newFileDiff, gremlinsExit: 1,
				events: "baseline\tpkg.test\t4s\n" +
					"cap\tprovider.test\tran longer than 1m30s without a mutant\n"},
			wantExit: exitDidNotRun,
			wantOutput: []string{
				"gremlins exited 1; provider.test: ran longer than 1m30s without a mutant",
			},
			wantSummary: []string{
				"**DID NOT RUN**: gremlins exited 1",
				"- `provider.test` (cap): ran longer than 1m30s without a mutant",
			},
		},
		{
			name:     "gremlins writes no results",
			args:     base,
			run:      fakeRun{diff: newFileDiff},
			wantExit: exitDidNotRun, wantOutput: []string{"gremlins wrote no results"},
		},
		{
			name: "the tests detect enough mutants",
			args: []string{"-base", "main", "-threshold", "50"},
			run: fakeRun{diff: newFileDiff, results: results(mutant(1, statusKilled),
				mutant(2, statusLived), mutant(3, statusTimedOut), mutant(3, statusNotViable))},
			wantExit: exitPassed,
			wantOutput: []string{
				"**PASSED**", "detected 2 of the 3 mutants that ran: 66.67%, threshold 50%",
				"1 mutants that do not compile are not counted",
				"#### Surviving mutants (1)", "| `pkg/new.go:2:2` | CONDITIONALS_NEGATION | LIVED |",
				"--diff main --exclude-files ^cmd/tool/",
			},
		},
		{
			name: "the tests detect too few mutants",
			args: base,
			run: fakeRun{diff: newFileDiff, results: results(mutant(1, statusKilled),
				mutant(2, statusLived), mutant(3, statusLived))},
			wantExit: exitFailed, wantOutput: []string{"**FAILED**", "33.33%, threshold 90%"},
		},
		{
			name: "only mutants that do not compile",
			args: base,
			run: fakeRun{diff: newFileDiff, results: results(mutant(1, statusNotViable),
				mutant(3, statusNotViable))},
			wantExit: exitPassed, wantOutput: []string{"**PASSED**: None of the 2 mutants"},
		},
		{
			name: "exec could not apply a limit",
			args: base,
			run: fakeRun{diff: newFileDiff, results: results(mutant(1, statusKilled)),
				events: "error\tpkg.test\tcannot limit memory to 4096 MiB: EPERM\n"},
			wantExit:   exitDidNotRun,
			wantOutput: []string{"could not do its part: pkg.test: cannot limit memory"},
		},
		{
			name: "whole module",
			args: []string{"-all", "-threshold", "0"},
			run: fakeRun{results: results(mutant(1, statusLived)),
				events: "baseline\tpkg.test\t4s\nbaseline\tprovider.test\t1m1s\n" +
					"cap\tpkg.test\tran longer than the 3m0s cap\n"},
			wantExit: exitPassed,
			wantOutput: []string{
				"Scope: every Go file in the module.",
				"Slowest test process without a mutant: `provider.test`, 1m1s.",
				"#### Test processes exec stopped, and errors (1)",
				"- `pkg.test` (cap): ran longer than the 3m0s cap",
				"#### Packages not mutated (1)", "- `cmd/tool`",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := filepath.Join(t.TempDir(), "summary.md")
			var stdout, stderr bytes.Buffer
			args := append(slices.Clone(tt.args), "-summary", summary)
			exit := runGate(args, &stdout, &stderr, tt.run.tools())
			output := stdout.String() + stderr.String()
			if exit != tt.wantExit {
				t.Errorf("exit %d, want %d\n%s", exit, tt.wantExit, output)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(output, want) {
					t.Errorf("output lacks %q:\n%s", want, output)
				}
			}
			if tt.noGremlins != (tt.run.calls == 0) {
				t.Errorf("gremlins ran %d times", tt.run.calls)
			}
			written, err := os.ReadFile(summary)
			if tt.noSummary {
				if !errors.Is(err, os.ErrNotExist) {
					t.Errorf("summary %q, %v", written, err)
				}
				return
			}
			if err != nil || !strings.HasPrefix(string(written), "### Mutation tests") {
				t.Errorf("summary %q, %v", written, err)
			}
			for _, want := range tt.wantSummary {
				if !strings.Contains(string(written), want) {
					t.Errorf("summary lacks %q:\n%s", want, written)
				}
			}
		})
	}
}

// gremlins must find the go shim first on its PATH, run through the go subcommand
// with the real go command, and go test must run at most -workers test processes.
func TestRunGateGivesGremlinsTheGoShim(t *testing.T) {
	run := fakeRun{diff: newFileDiff, results: results(mutant(1, statusKilled))}
	args := []string{"-base", "main", "-workers", "1", "-summary", ""}
	var shim []byte
	tools := run.tools()
	gremlins := tools.gremlins
	tools.gremlins = func(binary string, args, env []string) (int, error) {
		first, _, _ := strings.Cut(envValue(env, "PATH"), string(os.PathListSeparator))
		var err error
		shim, err = os.ReadFile(filepath.Join(first, "go"))
		if err != nil {
			return 0, err
		}
		return gremlins(binary, args, env)
	}
	if exit := runGate(args, &bytes.Buffer{}, &bytes.Buffer{}, tools); exit != exitPassed {
		t.Fatalf("exit %d", exit)
	}
	want := "exec '/opt/mutationgate' go -real-go='/usr/local/go/bin/go' -log='"
	if !strings.HasPrefix(string(shim), "#!/bin/sh\n"+want) {
		t.Errorf("go shim:\n%s", shim)
	}
	if goflags := envValue(run.env, "GOFLAGS"); !strings.Contains(goflags, " -p=1 ") {
		t.Errorf("GOFLAGS %q", goflags)
	}
}

func TestRunGateAnnotatesUnderGitHubActions(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	run := fakeRun{diff: newFileDiff, results: results(mutant(1, statusLived))}
	var stdout bytes.Buffer
	args := []string{"-base", "main", "-summary", ""}
	exit := runGate(args, &stdout, &bytes.Buffer{}, run.tools())
	want := "::error title=Mutation tests::The tests detected 0 of the 1 mutants that ran: " +
		"0.00%25, threshold 90%25. The job summary lists the surviving mutants.\n"
	if exit != exitFailed || !strings.HasSuffix(stdout.String(), want) {
		t.Errorf("exit %d, output:\n%s", exit, stdout.String())
	}
}
