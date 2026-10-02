package main

import (
	"strings"
	"testing"
)

// Events as go test -json writes them, one per line.
const (
	pkgNoTests  = `{"Action":"skip","Package":"m/empty"}`
	runA        = `{"Action":"run","Package":"m/a","Test":"TestA"}`
	outA        = `{"Action":"output","Package":"m/a","Test":"TestA","Output":"--- PASS: TestA\n"}`
	passA       = `{"Action":"pass","Package":"m/a","Test":"TestA"}`
	runB        = `{"Action":"run","Package":"m/a","Test":"TestB"}`
	passB       = `{"Action":"pass","Package":"m/a","Test":"TestB"}`
	failB       = `{"Action":"fail","Package":"m/a","Test":"TestB"}`
	skipB       = `{"Action":"skip","Package":"m/a","Test":"TestB"}`
	runBSub     = `{"Action":"run","Package":"m/a","Test":"TestB/sub"}`
	skipBSub    = `{"Action":"skip","Package":"m/a","Test":"TestB/sub"}`
	pkgPass     = `{"Action":"pass","Package":"m/a"}`
	pkgFail     = `{"Action":"fail","Package":"m/a"}`
	buildOutput = `{"ImportPath":"m/b","Action":"build-output","Output":"b.go:1: syntax error\n"}`
	pkgFailB    = `{"Action":"fail","Package":"m/b","FailedBuild":"m/b"}`
)

// outALarge is an output line longer than bufio.Scanner's default 64 KB limit, as a
// test that logs a Terraform plan can write.
var outALarge = `{"Action":"output","Package":"m/a","Test":"TestA","Output":"` +
	strings.Repeat("x", 100<<10) + `\n"}`

func TestCheck(t *testing.T) {
	tests := []struct {
		name       string
		events     []string
		required   []string
		wantExit   int
		wantOutput string
	}{
		{
			name:     "all passed",
			events:   []string{pkgNoTests, runA, outA, passA, runB, passB, pkgPass},
			required: []string{"m/a TestA"}, wantExit: exitPassed,
			wantOutput: "--- PASS: TestA\ncheckgotest: 2 passed, 0 failed, 0 skipped\n",
		},
		{
			name:     "a test failed",
			events:   []string{runA, passA, runB, failB, pkgFail},
			wantExit: exitFailed, wantOutput: "tests failed:\n    m/a TestB",
		},
		{
			name:     "a test skipped",
			events:   []string{runA, passA, runB, skipB, pkgPass},
			wantExit: exitFailed, wantOutput: "tests skipped; this run allows no skips:\n    m/a TestB",
		},
		{
			name:     "a subtest skipped",
			events:   []string{runA, passA, runB, runBSub, skipBSub, passB, pkgPass},
			wantExit: exitFailed, wantOutput: "m/a TestB/sub",
		},
		{
			name:     "a package failed to build",
			events:   []string{runA, passA, pkgPass, buildOutput, pkgFailB},
			wantExit: exitFailed,
			wantOutput: "b.go:1: syntax error\n" +
				"checkgotest: 1 passed, 0 failed, 0 skipped\n" +
				"checkgotest: packages failed to build or run:\n    m/b",
		},
		{
			name:     "a required test skipped",
			events:   []string{runA, passA, runB, skipB, pkgPass},
			required: []string{"m/a TestB"}, wantExit: exitFailed,
			wantOutput: "required tests did not pass:\n    m/a TestB",
		},
		{
			name:     "a required test never ran",
			events:   []string{runA, passA, pkgPass},
			required: []string{"m/a TestA", "m/a TestB"}, wantExit: exitDidNotRun,
			wantOutput: "DID NOT RUN: required tests never ran:\n    m/a TestB\n",
		},
		{
			name:     "a required test ran only in another package",
			events:   []string{runA, passA, pkgPass},
			required: []string{"m/other TestA"}, wantExit: exitDidNotRun,
			wantOutput: "DID NOT RUN: required tests never ran:\n    m/other TestA\n",
		},
		{
			name:       "an output line longer than 64 KB",
			events:     []string{runA, outALarge, passA, pkgPass},
			required:   []string{"m/a TestA"},
			wantExit:   exitPassed,
			wantOutput: "checkgotest: 1 passed, 0 failed, 0 skipped\n",
		},
		{
			name:     "no input",
			wantExit: exitDidNotRun, wantOutput: "DID NOT RUN: no test ran",
		},
		{
			name:     "only packages without tests",
			events:   []string{pkgNoTests, pkgPass},
			wantExit: exitDidNotRun, wantOutput: "DID NOT RUN: no test ran",
		},
		{
			name:     "not go test -json output",
			events:   []string{runA, passA, "ok  \tm/a\t0.1s"},
			wantExit: exitDidNotRun,
			wantOutput: "ok  \tm/a\t0.1s\n" +
				"checkgotest: 1 passed, 0 failed, 0 skipped\n" +
				"checkgotest: DID NOT RUN: 1 input lines are not go test -json events",
		},
		{
			name:     "failure wins over did not run",
			events:   []string{runB, failB, pkgFail},
			required: []string{"m/a TestA"}, wantExit: exitFailed,
			wantOutput: "tests failed:\n    m/a TestB",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.Join(tt.events, "\n")
			var output strings.Builder

			exit := check(strings.NewReader(input), &output, tt.required)

			if exit != tt.wantExit {
				t.Errorf("check() = %d, want %d; output:\n%s", exit, tt.wantExit, output.String())
			}
			if !strings.Contains(output.String(), tt.wantOutput) {
				t.Errorf("output does not contain %q:\n%s", tt.wantOutput, output.String())
			}
		})
	}
}

func TestRun(t *testing.T) {
	passingRun := strings.Join([]string{runA, passA, pkgPass}, "\n")
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name: "required test ran and passed", args: []string{"-require", "m/a TestA"},
			wantExit: exitPassed, wantStdout: "checkgotest: 1 passed, 0 failed, 0 skipped\n",
		},
		{
			name: "no required tests", wantExit: exitPassed,
			wantStdout: "checkgotest: 1 passed, 0 failed, 0 skipped\n",
		},
		{
			name: "a test name without -require", args: []string{"TestA"},
			wantExit: exitDidNotRun, wantStderr: `unexpected arguments ["TestA"]`,
		},
		{
			name: "an argument after -require", args: []string{"-require", "m/a TestA", "TestB"},
			wantExit: exitDidNotRun, wantStderr: `unexpected arguments ["TestB"]`,
		},
		{
			name: "-require without the package", args: []string{"-require", "TestA"},
			wantExit: exitDidNotRun, wantStderr: "want an import path and a test name",
		},
		{
			name: "-require without the test", args: []string{"-require", "m/a "},
			wantExit: exitDidNotRun, wantStderr: "want an import path and a test name",
		},
		{
			name: "-require with an extra word", args: []string{"-require", "m/a TestA x"},
			wantExit: exitDidNotRun, wantStderr: "want an import path and a test name",
		},
		{
			name: "-require without a value", args: []string{"-require"},
			wantExit: exitDidNotRun, wantStderr: "flag needs an argument",
		},
		{
			name: "an unknown flag", args: []string{"-required", "m/a TestA"},
			wantExit: exitDidNotRun, wantStderr: "flag provided but not defined",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder

			exit := run(tt.args, strings.NewReader(passingRun), &stdout, &stderr)

			if exit != tt.wantExit {
				t.Errorf("run() = %d, want %d; stdout:\n%s\nstderr:\n%s",
					exit, tt.wantExit, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout does not contain %q:\n%s", tt.wantStdout, stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr does not contain %q:\n%s", tt.wantStderr, stderr.String())
			}
			if tt.wantExit == exitDidNotRun && stdout.Len() > 0 {
				t.Errorf("stdout = %q, want nothing from a command line that was rejected",
					stdout.String())
			}
		})
	}
}

func TestCheckReadError(t *testing.T) {
	longLine := `{"Action":"output","Output":"` + strings.Repeat("x", maxLineBytes) + `"}`
	var output strings.Builder

	exit := check(strings.NewReader(longLine), &output, nil)

	if exit != exitDidNotRun {
		t.Errorf("check() = %d, want %d", exit, exitDidNotRun)
	}
	if want := "DID NOT RUN: reading go test output"; !strings.Contains(output.String(), want) {
		t.Errorf("output does not contain %q:\n%s", want, output.String())
	}
}
