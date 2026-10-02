package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// commandGo is the subcommand the gate puts first on gremlins' PATH as go.
//
// gremlins 0.6.0 counts a mutant as not viable only when go test exits 2, but go
// test exits 1 when the build fails, so a mutant that does not compile, such as +
// turned into - between two strings, would count as killed. For a gremlins mutant
// run, go first builds the mutated package and exits 2 when it does not compile.
// It then replaces itself with the real go command, as it does for every other
// command, so gremlins' timeout still kills the process it started. A mutant that
// breaks only the package's test files still counts as killed.
const commandGo = "go"

// exitNotViable is the go test exit code gremlins reads as a mutant that does not
// compile.
const exitNotViable = 2

// goShimOptions is the parsed command line of the go subcommand.
type goShimOptions struct {
	realGo  string
	logPath string
	args    []string
}

func runGoShim(args []string, stderr io.Writer) int {
	opts, err := parseGoShimArgs(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mutationgate go: %v\n", err)
		return exitFailed
	}
	if pkg, buildFlags, ok := mutantTestRun(opts.args); ok {
		if code, done := checkMutantBuilds(opts, pkg, buildFlags, stderr); done {
			return code
		}
	}
	err = syscall.Exec(opts.realGo, append([]string{"go"}, opts.args...), os.Environ())
	shimError(opts, stderr, fmt.Sprintf("cannot run %s: %v", opts.realGo, err))
	return exitFailed
}

func parseGoShimArgs(args []string, stderr io.Writer) (goShimOptions, error) {
	fs := flag.NewFlagSet("mutationgate go", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opts goShimOptions
	fs.StringVar(&opts.realGo, "real-go", "", "the go command to run")
	fs.StringVar(&opts.logPath, "log", "", "file to append events to")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	opts.args = fs.Args()
	if !filepath.IsAbs(opts.realGo) {
		return opts, fmt.Errorf("-real-go %q: need an absolute path", opts.realGo)
	}
	return opts, nil
}

// mutantTestRun reports whether args are gremlins 0.6.0's go test of one mutant,
// "test [-tags T] -timeout D -failfast [-cpu N] package", and returns the package
// and the flags to build it with.
func mutantTestRun(args []string) (string, []string, bool) {
	if len(args) < 2 || args[0] != "test" || !slices.Contains(args, "-failfast") {
		return "", nil, false
	}
	pkg := args[len(args)-1]
	if strings.HasPrefix(pkg, "-") || strings.HasSuffix(pkg, "...") {
		return "", nil, false
	}
	var buildFlags []string
	if i := slices.Index(args, "-tags"); i > 0 && i+1 < len(args)-1 {
		buildFlags = []string{"-tags", args[i+1]}
	}
	return pkg, buildFlags, true
}

// checkMutantBuilds builds the mutated package. When it does not compile, it
// returns exitNotViable and done. A build that fails for another reason, such as a
// package go list cannot load, is logged as an error, which fails the gate.
func checkMutantBuilds(
	opts goShimOptions, pkg string, buildFlags []string, stderr io.Writer,
) (int, bool) {
	build := exec.Command(opts.realGo,
		slices.Concat([]string{"build", "-o", os.DevNull}, buildFlags, []string{pkg})...)
	build.Stdout = stderr
	build.Stderr = stderr
	err := build.Run()
	if err == nil {
		return 0, false
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		shimError(opts, stderr, fmt.Sprintf("go build %s: %v", pkg, err))
		return exitFailed, true
	}
	list := exec.Command(opts.realGo, slices.Concat([]string{"list"}, buildFlags, []string{pkg})...)
	list.Stdout = io.Discard
	list.Stderr = stderr
	if err := list.Run(); err != nil {
		shimError(opts, stderr,
			fmt.Sprintf("go build %s failed, and go list cannot load the package: %v", pkg, err))
		return exitFailed, true
	}
	_, _ = fmt.Fprintf(stderr, "mutationgate go: %s does not compile with this mutant\n", pkg)
	return exitNotViable, true
}

func shimError(opts goShimOptions, stderr io.Writer, detail string) {
	_, _ = fmt.Fprintf(stderr, "mutationgate go: %s\n", detail)
	logEvent(opts.logPath, stderr, event{kind: eventError, binary: commandGo, detail: detail})
}

// writeGoShim writes dir/go, a script that runs the go subcommand of the
// executable self with the real go command realGo and the event log logPath. Each
// path must pass checkPlainPath.
func writeGoShim(dir, self, realGo, logPath string) error {
	script := fmt.Sprintf("#!/bin/sh\nexec '%s' %s -real-go='%s' -log='%s' -- \"$@\"\n",
		self, commandGo, realGo, logPath)
	return os.WriteFile(filepath.Join(dir, commandGo), []byte(script), 0o700)
}
