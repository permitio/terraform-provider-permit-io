// Command checkgotest reads `go test -json` output on stdin, prints the test output,
// and fails a test run that did less than it should. `go test` reports a skipped
// test as ok, so a run where every test skipped would otherwise pass.
//
// A required test is named by its package import path and its name, separated by
// a space, the way checkgotest lists tests. Naming the package keeps a test of the
// same name in another package from standing in for it.
//
// Exit codes:
//
//	0  every test passed and every required test ran and passed
//	1  a test failed or skipped, a package failed to build or run, or a required
//	   test failed or skipped
//	2  no test ran, a required test never ran, the input is not go test -json
//	   output, or the command line is wrong
//
// Build it before use; go run reports every non-zero exit as 1:
//
//	go build -o checkgotest ./internal/acctest/checkgotest
//	go test -json ./... | ./checkgotest -require 'example.com/m/pkg TestName'
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

// maxLineBytes bounds one line of go test -json output; a test that logs a large
// Terraform plan can write a long line.
const maxLineBytes = 16 << 20

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run parses the command line and checks the go test -json events on stdin. A
// command line it cannot use counts as not having run, so a mistyped CI step
// cannot pass.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("checkgotest", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var required []string
	const requireUsage = "a test that must run and pass, as `'import/path TestName'` (repeatable)"
	flags.Func("require", requireUsage, func(value string) error {
		pkg, test, ok := strings.Cut(value, " ")
		if !ok || pkg == "" || test == "" || strings.Contains(test, " ") {
			return errors.New("want an import path and a test name separated by one space")
		}
		required = append(required, value)
		return nil
	})
	if err := flags.Parse(args); err != nil {
		return exitDidNotRun
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "checkgotest: unexpected arguments %q; name required tests with "+
			"-require\n", flags.Args())
		flags.Usage()
		return exitDidNotRun
	}
	return check(stdin, stdout, required)
}

// testEvent is the part of a go test -json event (see `go doc test2json`) the
// check reads.
type testEvent struct {
	Action  string
	Package string
	Test    string
	Output  string
}

type results struct {
	passed, failed, skipped []string
	failedPackages          []string
	started, passedNames    map[string]bool
	malformedLines          int
}

func check(input io.Reader, output io.Writer, required []string) int {
	r, err := read(input, output)
	if err != nil {
		fmt.Fprintf(output, "checkgotest: DID NOT RUN: reading go test output: %v\n", err)
		return exitDidNotRun
	}
	fmt.Fprintf(output, "checkgotest: %d passed, %d failed, %d skipped\n",
		len(r.passed), len(r.failed), len(r.skipped))

	failed := report(output, "tests failed", r.failed)
	failed = report(output, "packages failed to build or run", r.failedPackages) || failed
	failed = report(output, "tests skipped; this run allows no skips", r.skipped) || failed
	var missing, notPassed []string
	for _, name := range required {
		switch {
		case !r.started[name]:
			missing = append(missing, name)
		case !r.passedNames[name]:
			notPassed = append(notPassed, name)
		}
	}
	failed = report(output, "required tests did not pass", notPassed) || failed
	if failed {
		return exitFailed
	}

	didNotRun := report(output, "DID NOT RUN: required tests never ran", missing)
	if len(r.passed) == 0 {
		fmt.Fprintln(output, "checkgotest: DID NOT RUN: no test ran")
		didNotRun = true
	}
	if r.malformedLines > 0 {
		fmt.Fprintf(output, "checkgotest: DID NOT RUN: %d input lines are not go test -json events\n",
			r.malformedLines)
		didNotRun = true
	}
	if didNotRun {
		return exitDidNotRun
	}
	return exitPassed
}

// read echoes the test output from the events on input and sorts the test results.
func read(input io.Reader, output io.Writer) (results, error) {
	r := results{started: map[string]bool{}, passedNames: map[string]bool{}}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(nil, maxLineBytes)
	for scanner.Scan() {
		var event testEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			fmt.Fprintf(output, "%s\n", scanner.Bytes())
			r.malformedLines++
			continue
		}
		fmt.Fprint(output, event.Output)
		if event.Test == "" {
			if event.Action == "fail" {
				r.failedPackages = append(r.failedPackages, event.Package)
			}
			continue
		}
		name := event.Package + " " + event.Test
		r.started[name] = true
		switch event.Action {
		case "pass":
			r.passed = append(r.passed, name)
			r.passedNames[name] = true
		case "fail":
			r.failed = append(r.failed, name)
		case "skip":
			r.skipped = append(r.skipped, name)
		}
	}
	return r, scanner.Err()
}

// report prints the names under a heading and returns whether there were any.
func report(output io.Writer, heading string, names []string) bool {
	if len(names) == 0 {
		return false
	}
	fmt.Fprintf(output, "checkgotest: %s:\n    %s\n", heading, strings.Join(names, "\n    "))
	return true
}
