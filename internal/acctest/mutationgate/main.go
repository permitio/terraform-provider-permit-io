// Command mutationgate runs gremlins mutation testing on the Go lines a change adds
// or edits, or on the whole module, with every test run it starts bounded in time,
// memory and output, and fails when the tests detect too few of the mutants.
//
// gremlins alone cannot serve as the gate, so mutationgate wraps it:
//
//   - gremlins bounds a test run only by a timeout, the whole module's coverage run
//     times a coefficient, and on that timeout kills the go command but not the
//     test process it started. mutationgate runs every test process through its
//     own exec subcommand (go test -exec, set in GOFLAGS), which stops the process
//     when it runs longer than -cap, uses more than -memory-mib, writes more than
//     64 MiB of output, or outlives the go command. On Linux the kernel enforces
//     the memory limit (RLIMIT_AS, as ulimit -v does); macOS does not, so there
//     exec polls the process's physical footprint every 100ms. Before gremlins
//     starts, a probe process that allocates without end must be stopped by the
//     limit, or the run does not start. GOFLAGS also sets go test -p to -workers,
//     so gremlins' coverage run, which tests every package without a mutant, runs
//     at most that many test processes at a time too. A test process of that run
//     may take half of -cap, so that a mutant of its package has room before the
//     cap counts it as killed.
//   - gremlins counts a mutant as not viable only when go test exits 2, but go test
//     exits 1 when the build fails, so a mutant that does not compile would count
//     as killed. mutationgate puts its go subcommand first on gremlins' PATH, which
//     builds a mutated package before testing it and exits 2 when it does not
//     compile. Mutants that do not compile are not counted.
//   - gremlins counts a timed-out mutant as neither killed nor lived, and its
//     efficacy threshold fails a result equal to it. mutationgate counts killed and
//     timed-out mutants as detected, and fails when detected / (detected + lived)
//     is below -threshold.
//   - gremlins exits 0 when it finds no mutant at all, and with --diff it treats an
//     empty diff as a change to every line. mutationgate does not start gremlins
//     when no Go line changed, passes with a notice then or when the changed lines
//     hold nothing to mutate or only mutants that do not compile, and otherwise
//     fails with exit code 2 when no mutant ran.
//   - gremlins 0.6.0 tests a mutant with the package named by the nearest directory
//     that ends with the file's package name, or the module root: the wrong package
//     for package main in a subdirectory, or a package whose name differs from its
//     directory. mutationgate excludes those packages and lists them as not
//     mutated.
//   - gremlins reads which lines changed from git diff with its default context,
//     and marks as changed the lines that follow each hunk's leading context, which
//     misses the second of two edits that share a hunk. It also reads the user's
//     git config and GIT_EXTERNAL_DIFF, and an external diff tool leaves it an empty
//     diff. mutationgate runs gremlins with diff.context set to 0, and gremlins and
//     its own git diff without the user's and the system's git config, such as
//     diff.algorithm. It checks that git diff run as gremlins runs it reports the
//     changed lines it reads itself, and fails when gremlins' changed lines differ
//     from them.
//
// Test runs skip the acceptance tests (-skip '^TestAcc'), which need the Permit
// API, and do not use the test cache (-count=1). Offline tests run Terraform, so
// set TF_ACC_TERRAFORM_PATH. mutationgate runs on Linux and macOS, in the module
// root, which must be the repository root:
//
//	go build -o mutationgate ./internal/acctest/mutationgate
//	./mutationgate -gremlins path/to/gremlins -base origin/main
//	./mutationgate -gremlins path/to/gremlins -all
//
// Locally, pass a -memory-mib below any memory watchdog on the machine, so that the
// gate's limit, not the watchdog, stops a runaway test process.
//
// The report goes to stdout and, with -summary or GITHUB_STEP_SUMMARY, to that
// file as Markdown. Under GitHub Actions the result is also an annotation.
//
// Exit codes:
//
//	0  the tests detect at least -threshold percent of the mutants that ran, no
//	   Go line changed, or the changed lines hold nothing to mutate or only
//	   mutants that do not compile
//	1  the tests detect fewer than -threshold percent of the mutants that ran
//	2  the run did not complete: the command line is wrong, the memory limit is
//	   not enforced, git diff as gremlins runs it differs from the gate's, gremlins
//	   failed or wrote no results, its changed lines differ from git diff's, a
//	   mutant was left unrun, exec or go could not do their part, or Go lines
//	   changed and no mutant on them ran
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

const (
	commandExec     = "exec"
	commandAllocate = "allocate"
)

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch runs the gate, or one of the subcommands the gate runs itself as: exec,
// which go test runs each test binary with, allocate, the memory limit probe, and
// go, which gremlins runs as the go command.
func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case commandExec:
			return runExecCommand(args[1:], stderr)
		case commandAllocate:
			return runAllocate(args[1:], stdout, stderr)
		case commandGo:
			return runGoShim(args[1:], stderr)
		}
	}
	if runtime.GOOS == "windows" {
		_, _ = fmt.Fprintln(stderr,
			"mutationgate: runs on Linux and macOS only\nmutationgate: DID NOT RUN")
		return exitDidNotRun
	}
	t, err := systemTools()
	if err != nil {
		return sink{stdout: stdout, stderr: stderr}.didNotRun(err, nil)
	}
	return runGate(args, stdout, stderr, t)
}
