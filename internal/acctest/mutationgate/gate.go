package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Defaults bound a run: gremlins' timeout coefficient at most maxCoefficient, and
// each test process at most defaultCap and defaultMemoryMiB. Two workers keep at
// most two mutated test processes running at a time, and go test -p the same
// number of unmutated ones in gremlins' coverage run.
const (
	defaultWorkers     = 2
	defaultCoefficient = 5
	maxCoefficient     = 5
	defaultCap         = 3 * time.Minute
	defaultMemoryMiB   = 4096
	defaultThreshold   = 90
)

// gateOptions is the parsed command line of the gate.
type gateOptions struct {
	base        string
	all         bool
	gremlins    string
	workers     int
	coefficient int
	cap         time.Duration
	memoryMiB   int64
	threshold   float64
	summary     string
}

// goPackage is one package of the module, as go list reports it.
type goPackage struct {
	dir        string
	importPath string
	name       string
}

// tools are the commands the gate runs, replaced in tests.
type tools struct {
	// self is the path of this executable, which go test runs as -exec.
	self string
	// goPath is the path of the go command, which the go subcommand runs.
	goPath string
	// diff returns git diff -U0 from the merge base with base to the work tree.
	diff func(base string) ([]byte, error)
	// gremlinsDiff returns git diff from the merge base with base as gremlins runs
	// it, with env.
	gremlinsDiff func(base string, env []string) ([]byte, error)
	// packages returns the module path, its root directory and its packages.
	packages func() (module, root string, pkgs []goPackage, err error)
	// probe runs a process that allocates without end under the limits, and
	// returns how much it allocated before it was stopped.
	probe func(opts gateOptions) (stoppedAtMiB int64, err error)
	// gremlins runs gremlins with args and env, streaming its output, and returns
	// its exit code.
	gremlins func(binary string, args, env []string) (int, error)
}

func systemTools() (tools, error) {
	self, err := os.Executable()
	if err != nil {
		return tools{}, fmt.Errorf("cannot find the path of this executable: %w", err)
	}
	goPath, err := exec.LookPath("go")
	if err == nil {
		goPath, err = filepath.Abs(goPath)
	}
	if err != nil {
		return tools{}, fmt.Errorf("cannot find the go command: %w", err)
	}
	return tools{
		self:         self,
		goPath:       goPath,
		diff:         gitDiff,
		gremlinsDiff: gremlinsGitDiff,
		packages:     goListPackages,
		probe:        func(opts gateOptions) (int64, error) { return probeMemoryLimit(self, opts) },
		gremlins:     runGremlins,
	}, nil
}

// runGate checks that the memory limit holds, works out which Go files to mutate,
// runs gremlins on them and reports the result. Anything that keeps it from
// measuring the tests counts as not having run, so a broken setup cannot pass.
func runGate(args []string, stdout, stderr io.Writer, t tools) int {
	out := sink{stdout: stdout, stderr: stderr}
	opts, err := parseGateArgs(args, stderr)
	if err != nil {
		return out.didNotRun(err, nil)
	}
	out.summary = opts.summary
	if err := checkPlainPath("path of this executable", t.self); err != nil {
		return out.didNotRun(err, nil)
	}
	if err := checkPlainPath("go command", t.goPath); err != nil {
		return out.didNotRun(err, nil)
	}
	stoppedAt, err := t.probe(opts)
	if err != nil {
		return out.didNotRun(fmt.Errorf("memory limit check: %w", err), nil)
	}
	limits := fmt.Sprintf("%d workers, go test -p %d, timeout coefficient %d, each test process "+
		"at most %s (half that without a mutant), %d MiB and %d MiB of output (a probe "+
		"allocating without end was stopped by the memory limit after %d MiB)",
		opts.workers, opts.workers, opts.coefficient, opts.cap, opts.memoryMiB, outputBudget>>20,
		stoppedAt)
	_, _ = fmt.Fprintf(stdout, "mutationgate: %s\n", limits)

	module, root, pkgs, err := t.packages()
	if err != nil {
		return out.didNotRun(err, nil)
	}
	excluded, err := misattributedDirs(module, root, pkgs)
	if err != nil {
		return out.didNotRun(err, nil)
	}
	s := scope{all: opts.all, base: opts.base, excludedDirs: excluded}
	var changed map[string][]lineRange
	if !opts.all {
		diff, err := t.diff(opts.base)
		if err != nil {
			return out.didNotRun(err, nil)
		}
		changed, err = parseAddedLines(diff)
		if err != nil {
			return out.didNotRun(err, nil)
		}
		s.changed, s.notMutated = splitExcluded(changed, excluded)
		if len(s.changed) == 0 {
			r := report{opts: opts, limits: limits, scope: s}
			return r.finish(out, outcome{exit: exitPassed, notice: noChangeNotice(s)})
		}
	}

	dir, err := os.MkdirTemp("", "mutationgate-")
	if err != nil {
		return out.didNotRun(err, nil)
	}
	defer removeWorkDir(dir, stderr)
	if err := checkPlainPath("work directory", dir); err != nil {
		return out.didNotRun(err, nil)
	}
	w, err := newWorkFiles(dir, t)
	if err != nil {
		return out.didNotRun(err, nil)
	}
	env := gremlinsEnv(os.Environ(), t.self, opts, w)
	if !opts.all {
		if err := checkGremlinsDiff(t, opts.base, env, changed); err != nil {
			return out.didNotRun(err, nil)
		}
	}
	gremlinsArgs := buildGremlinsArgs(opts, s, w.results, w.config)
	_, _ = fmt.Fprintf(stdout, "mutationgate: %s %s\n",
		opts.gremlins, strings.Join(gremlinsArgs, " "))
	start := time.Now()
	code, err := t.gremlins(opts.gremlins, gremlinsArgs, env)
	r := report{opts: opts, limits: limits, scope: s, elapsed: time.Since(start)}
	events, eventsErr := readEvents(w.events)
	if err != nil {
		return out.didNotRun(fmt.Errorf("gremlins: %w", err), events)
	}
	if code != 0 {
		return out.didNotRun(fmt.Errorf("gremlins exited %d%s", code, stopReasons(events)), events)
	}
	if eventsErr != nil {
		return out.didNotRun(eventsErr, nil)
	}
	r.events = events
	r.mutations, err = readResults(w.results)
	if err != nil {
		return out.didNotRun(err, events)
	}
	return r.finish(out, r.evaluate())
}

// sink is where the gate reports: stdout, stderr and the job summary file.
type sink struct {
	stdout, stderr io.Writer
	summary        string
}

// didNotRun reports a run that did not complete, with the events exec and go
// logged, and returns exitDidNotRun.
func (s sink) didNotRun(err error, events []event) int {
	_, _ = fmt.Fprintf(s.stderr, "mutationgate: %v\nmutationgate: DID NOT RUN\n", err)
	if s.summary != "" {
		var b strings.Builder
		fmt.Fprintf(&b, "### Mutation tests\n\n**DID NOT RUN**: %s\n\n", err)
		writeEvents(&b, events)
		if err := appendFile(s.summary, b.String()); err != nil {
			_, _ = fmt.Fprintf(s.stderr, "mutationgate: job summary: %v\n", err)
		}
	}
	annotate(s.stdout, "error", "The mutation tests did not run: "+err.Error())
	return exitDidNotRun
}

// stopReasons lists the test processes exec stopped, and the errors exec and go
// logged, for the message of a gremlins run that failed.
func stopReasons(events []event) string {
	var reasons []string
	for _, e := range events {
		if e.kind != eventBaseline {
			reasons = append(reasons, e.binary+": "+e.detail)
		}
	}
	if len(reasons) == 0 {
		return ""
	}
	return "; " + strings.Join(reasons, "; ")
}

func removeWorkDir(dir string, stderr io.Writer) {
	if err := os.RemoveAll(dir); err != nil {
		_, _ = fmt.Fprintf(stderr, "mutationgate: could not remove %s: %v\n", dir, err)
	}
}

func parseGateArgs(args []string, stderr io.Writer) (gateOptions, error) {
	fs := flag.NewFlagSet("mutationgate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opts gateOptions
	fs.StringVar(&opts.base, "base", "",
		"mutate the lines changed since the merge base with this git ref")
	fs.BoolVar(&opts.all, "all", false, "mutate every Go file in the module")
	fs.StringVar(&opts.gremlins, "gremlins", "gremlins", "gremlins executable")
	fs.IntVar(&opts.workers, "workers", defaultWorkers,
		"mutants tested, and go test -p, at a time (at least 1)")
	fs.IntVar(&opts.coefficient, "timeout-coefficient", defaultCoefficient,
		"gremlins' timeout: the coverage run's time times this (1 to 5)")
	fs.DurationVar(&opts.cap, "cap", defaultCap, "longest one test process may run")
	fs.Int64Var(&opts.memoryMiB, "memory-mib", defaultMemoryMiB,
		"most memory one test process may use, in MiB")
	fs.Float64Var(&opts.threshold, "threshold", defaultThreshold,
		"lowest percentage of mutants that ran the tests must detect (0 to 100)")
	fs.StringVar(&opts.summary, "summary", os.Getenv("GITHUB_STEP_SUMMARY"),
		"file to append the Markdown report to")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	switch {
	case fs.NArg() > 0:
		return opts, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	case opts.all == (opts.base != ""):
		return opts, errors.New("give exactly one of -base <ref> and -all")
	case opts.workers < 1:
		return opts, fmt.Errorf("-workers %d: need at least 1 (gremlins reads 0 as one per CPU)",
			opts.workers)
	case opts.coefficient < 1 || opts.coefficient > maxCoefficient:
		return opts, fmt.Errorf("-timeout-coefficient %d: need 1 to %d",
			opts.coefficient, maxCoefficient)
	case opts.cap <= 0:
		return opts, fmt.Errorf("-cap %s: need a positive duration", opts.cap)
	case opts.memoryMiB < 1:
		return opts, fmt.Errorf("-memory-mib %d: need at least 1", opts.memoryMiB)
	case opts.threshold < 0 || opts.threshold > 100:
		return opts, fmt.Errorf("-threshold %g: need 0 to 100", opts.threshold)
	}
	return opts, nil
}

// checkPlainPath rejects a path that GOFLAGS' quoting, go test's splitting of the
// -exec value or the go shim's shell quoting would break.
func checkPlainPath(what, p string) error {
	if p == "" {
		return fmt.Errorf("cannot find the %s", what)
	}
	if strings.ContainsAny(p, " \t\n'\"\\") {
		return fmt.Errorf("%s %q has a space, quote or backslash, which GOFLAGS and the go shim "+
			"cannot carry", what, p)
	}
	return nil
}

// workFiles are the files of one run in its work directory.
type workFiles struct {
	results string
	events  string
	config  string
	// bin holds the go shim, first on gremlins' PATH.
	bin string
}

// newWorkFiles creates gremlins' empty config file and the go shim in dir.
func newWorkFiles(dir string, t tools) (workFiles, error) {
	w := workFiles{
		results: filepath.Join(dir, "results.json"),
		events:  filepath.Join(dir, "events.log"),
		config:  filepath.Join(dir, "gremlins.yaml"),
		bin:     filepath.Join(dir, "bin"),
	}
	if err := os.WriteFile(w.config, nil, 0o600); err != nil {
		return w, err
	}
	if err := os.Mkdir(w.bin, 0o700); err != nil {
		return w, err
	}
	return w, writeGoShim(w.bin, t.self, t.goPath, w.events)
}

// scope is what a run mutates: every file (all), or the lines changed since the
// merge base with base, less the packages gremlins would test with the wrong
// package's tests.
type scope struct {
	all          bool
	base         string
	changed      map[string][]lineRange
	notMutated   []string
	excludedDirs []string
}

// lineRange is a run of added or edited lines, first to last.
type lineRange struct{ first, last int }

func (s scope) isChanged(file string, line int) bool {
	for _, r := range s.changed[file] {
		if line >= r.first && line <= r.last {
			return true
		}
	}
	return false
}

func noChangeNotice(s scope) string {
	if len(s.notMutated) > 0 {
		return fmt.Sprintf("Go files changed since the merge base with %s, but only in packages "+
			"gremlins cannot test (listed below); nothing was mutated.", s.base)
	}
	return fmt.Sprintf("No non-test Go line was added or changed since the merge base with %s; "+
		"nothing to mutate.", s.base)
}

var hunkHeader = regexp.MustCompile(`^@@ -[0-9]+(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

// parseAddedLines reads git diff -U0 output and returns, for each non-test Go file
// with added or edited lines, those lines. With no context lines each hunk's new
// side is one run of added lines, and a hunk that only deletes has a count of 0.
// Each hunk's body is skipped by its line counts, so a removed or added line that
// starts with "--- " or "+++ " is not read as a file header.
func parseAddedLines(diff []byte) (map[string][]lineRange, error) {
	changed := map[string][]lineRange{}
	file := ""
	bodyLeft := 0
	sc := bufio.NewScanner(bytes.NewReader(diff))
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, `\`):
			continue // "\ No newline at end of file" belongs to neither side of a hunk.
		case bodyLeft > 0:
			if line == "" || !strings.ContainsAny(line[:1], "+- ") {
				return nil, fmt.Errorf("git diff: hunk ends early at %q", line)
			}
			bodyLeft--
		case strings.HasPrefix(line, "+++ "):
			name := strings.TrimPrefix(line, "+++ ")
			switch {
			case name == "/dev/null":
				file = ""
			case strings.HasPrefix(name, "b/"):
				file = strings.TrimPrefix(name, "b/")
			default:
				return nil, fmt.Errorf("git diff: cannot read the file name in %q", line)
			}
			if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
				file = ""
			}
		case strings.HasPrefix(line, "@@ "):
			m := hunkHeader.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("git diff: cannot read the hunk header %q", line)
			}
			removed, first, added := hunkCount(m[1]), hunkCount(m[2]), hunkCount(m[3])
			bodyLeft = removed + added
			if file != "" && added > 0 {
				changed[file] = append(changed[file], lineRange{first, first + added - 1})
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	if bodyLeft > 0 {
		return nil, fmt.Errorf("git diff: the last hunk is missing %d lines", bodyLeft)
	}
	return changed, nil
}

// hunkCount reads a hunk header number; git omits a line count of 1.
func hunkCount(s string) int {
	if s == "" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}

// misattributedDirs returns the directories, relative to the module root, of the
// packages gremlins 0.6.0 would test with another package's tests. It names a
// mutant's package after the nearest directory, from the file's own up, whose path
// ends with the package name, or the module root when none does.
func misattributedDirs(module, root string, pkgs []goPackage) ([]string, error) {
	var dirs []string
	for _, p := range pkgs {
		rel, err := filepath.Rel(root, p.dir)
		if err != nil {
			return nil, fmt.Errorf("package %s is not under the module root: %w", p.importPath, err)
		}
		rel = filepath.ToSlash(rel)
		if rel != "." && gremlinsImportPath(module, rel, p.name) != p.importPath {
			dirs = append(dirs, rel)
		}
	}
	slices.Sort(dirs)
	return dirs, nil
}

func gremlinsImportPath(module, dir, name string) string {
	for p := dir; ; p = path.Dir(p) {
		if strings.HasSuffix(p, name) {
			return module + "/" + p
		}
		if path.Dir(p) == p {
			return module
		}
	}
}

// splitExcluded separates the changed files in excluded directories from the rest.
func splitExcluded(
	changed map[string][]lineRange, excluded []string,
) (map[string][]lineRange, []string) {
	kept := map[string][]lineRange{}
	var dropped []string
	for file, ranges := range changed {
		if slices.Contains(excluded, path.Dir(file)) {
			dropped = append(dropped, file)
			continue
		}
		kept[file] = ranges
	}
	slices.Sort(dropped)
	return kept, dropped
}

func buildGremlinsArgs(opts gateOptions, s scope, resultsPath, configPath string) []string {
	args := []string{
		"unleash",
		"--config", configPath,
		"--workers", strconv.Itoa(opts.workers),
		"--timeout-coefficient", strconv.Itoa(opts.coefficient),
		"--output", resultsPath,
	}
	if !s.all {
		args = append(args, "--diff", s.base)
	}
	for _, dir := range s.excludedDirs {
		args = append(args, "--exclude-files", "^"+regexp.QuoteMeta(dir)+`/[^/]+\.go$`)
	}
	return args
}

// isolatedGitVars are the environment variables that could change what git diff
// prints for gremlins: an external diff tool, for one, leaves it an empty diff.
var isolatedGitVars = []string{
	"GIT_EXTERNAL_DIFF", "GIT_DIFF_OPTS", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM",
	"GIT_CONFIG_PARAMETERS",
}

// gitIsolation keeps git from reading the user's and the system's git config, such
// as diff.algorithm, so that the gate and gremlins read the same diff.
var gitIsolation = []string{"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1"}

// isolateGit returns environ without isolatedGitVars and with gitIsolation.
func isolateGit(environ []string) []string {
	var env []string
	for _, kv := range environ {
		key, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(isolatedGitVars, key) {
			env = append(env, kv)
		}
	}
	return append(env, gitIsolation...)
}

// gremlinsEnv returns the environment for gremlins: the gate's own, without
// GREMLINS_ settings or git settings from the environment, the user or the system,
// with the go shim first on PATH, go test flags added to GOFLAGS and git diff's
// context set to 0 lines.
func gremlinsEnv(environ []string, self string, opts gateOptions, w workFiles) []string {
	var env []string
	goflags := ""
	searchPath := w.bin
	gitConfigCount := 0
	for _, kv := range environ {
		key, value, _ := strings.Cut(kv, "=")
		switch {
		case strings.HasPrefix(key, "GREMLINS_"), slices.Contains(isolatedGitVars, key):
			continue
		case key == "GOFLAGS":
			goflags = value
			continue
		case key == "PATH":
			if value != "" {
				searchPath += string(os.PathListSeparator) + value
			}
			continue
		case key == "GIT_CONFIG_COUNT":
			gitConfigCount, _ = strconv.Atoi(value)
			continue
		}
		env = append(env, kv)
	}
	execFlag := fmt.Sprintf("'-exec=%s %s -cap=%s -memory-mib=%d -log=%s'",
		self, commandExec, opts.cap, opts.memoryMiB, w.events)
	goflags = strings.TrimSpace(fmt.Sprintf("%s -count=1 -skip=^TestAcc -p=%d %s",
		goflags, opts.workers, execFlag))
	n := strconv.Itoa(gitConfigCount)
	env = append(env, "PATH="+searchPath, "GOFLAGS="+goflags)
	return append(append(env, gitIsolation...),
		"GIT_CONFIG_KEY_"+n+"=diff.context",
		"GIT_CONFIG_VALUE_"+n+"=0",
		"GIT_CONFIG_COUNT="+strconv.Itoa(gitConfigCount+1),
	)
}

// checkGremlinsDiff runs git diff as gremlins will, with its environment, and fails
// unless it reports the changed lines the gate read itself. The repository's own
// git config still applies to both.
func checkGremlinsDiff(t tools, base string, env []string, want map[string][]lineRange) error {
	out, err := t.gremlinsDiff(base, env)
	if err != nil {
		return err
	}
	got, err := parseAddedLines(out)
	if err != nil {
		return fmt.Errorf("git diff as gremlins runs it: %w; check the diff settings in .git/config",
			err)
	}
	if !maps.EqualFunc(got, want, func(a, b []lineRange) bool { return slices.Equal(a, b) }) {
		return fmt.Errorf("git diff --merge-base %s as gremlins runs it reports changed lines in %d "+
			"Go files, git diff -U0 --no-ext-diff in %d, or other lines: check the diff settings "+
			"in .git/config, such as diff.external", base, len(got), len(want))
	}
	return nil
}

// gitDiff runs git diff -U0 without the user's and the system's git config, as
// gremlins' git diff runs.
func gitDiff(base string) ([]byte, error) {
	cmd := exec.Command("git", "diff", "--merge-base", "-U0", "--no-color", "--no-ext-diff",
		"--src-prefix=a/", "--dst-prefix=b/", base)
	cmd.Env = isolateGit(os.Environ())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --merge-base %s: %w: %s",
			base, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// gremlinsGitDiff runs the git diff gremlins 0.6.0 runs, with env.
func gremlinsGitDiff(base string, env []string) ([]byte, error) {
	cmd := exec.Command("git", "diff", "--merge-base", base)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --merge-base %s as gremlins runs it: %w: %s",
			base, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func goListPackages() (string, string, []goPackage, error) {
	modOut, err := goList("-m", "-f", "{{.Path}}\t{{.Dir}}")
	if err != nil {
		return "", "", nil, err
	}
	module, root, ok := strings.Cut(strings.TrimSpace(modOut), "\t")
	if !ok {
		return "", "", nil, fmt.Errorf("go list -m: unexpected output %q", modOut)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", "", nil, err
	}
	if !sameDir(wd, root) {
		return "", "", nil, fmt.Errorf("run mutationgate in the module root %s, not %s", root, wd)
	}
	pkgOut, err := goList("-f", "{{.Dir}}\t{{.ImportPath}}\t{{.Name}}", "./...")
	if err != nil {
		return "", "", nil, err
	}
	var pkgs []goPackage
	for line := range strings.Lines(pkgOut) {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) != 3 {
			return "", "", nil, fmt.Errorf("go list: unexpected line %q", line)
		}
		pkgs = append(pkgs, goPackage{dir: fields[0], importPath: fields[1], name: fields[2]})
	}
	if len(pkgs) == 0 {
		return "", "", nil, errors.New("go list ./... found no packages")
	}
	return module, root, pkgs, nil
}

func sameDir(a, b string) bool {
	aInfo, errA := os.Stat(a)
	bInfo, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(aInfo, bInfo)
}

func goList(args ...string) (string, error) {
	cmd := exec.Command("go", append([]string{"list"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func runGremlins(binary string, args, env []string) (int, error) {
	cmd := exec.Command(binary, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}
