// Command vulnscan runs govulncheck on the provider's source or on built provider
// binaries, prints a report, and fails on every vulnerability govulncheck reports
// unless an accept file lists it with a reason.
//
// govulncheck itself fails only on vulnerable code that is called. vulnscan also
// fails on a vulnerable module that is required or imported but not called, so
// each one is either fixed or accepted on the record. Called code cannot be
// accepted. An accepted vulnerability is still printed, with its reason, and an
// accept entry whose vulnerability a complete scan no longer reports fails, so
// the accept file cannot go stale.
//
// The accept file has one vulnerability per line: its ID, the scan mode that
// reports it (source or binary), and the reason, separated by spaces. Blank lines
// and lines starting with # are ignored.
//
// Exit codes:
//
//	0  every vulnerability reported is accepted
//	1  a vulnerability is not accepted, called code is vulnerable, an accept entry
//	   is stale, or govulncheck exited 3 (vulnerabilities found)
//	2  the scan did not complete: govulncheck failed, its output was empty,
//	   truncated or of another scan mode, it found fewer modules than
//	   -min-modules, a binary is missing or their number is not -binaries, or the
//	   command line or accept file is wrong
//
// govulncheck is the version in go.mod's tool directive (go tool govulncheck), so
// run vulnscan in the module. Build it before use; go run reports every non-zero
// exit as 1:
//
//	go build -o vulnscan ./internal/acctest/vulnscan
//	./vulnscan -mode source -accept .github/vulnscan-accept.txt ./...
//	./vulnscan -mode binary -binaries 13 -accept .github/vulnscan-accept.txt bin/*
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	exitClean     = 0
	exitFindings  = 1
	exitDidNotRun = 2
)

// govulncheckFound is govulncheck's exit code when it finds vulnerabilities in its
// text output. With -format json it exits 0 and the findings are in the output.
const govulncheckFound = 3

const (
	modeSource = "source"
	modeBinary = "binary"
)

// defaultMinModules is the fewest modules a scan must find, by mode: fewer means it
// did not load what it was meant to scan. The provider's source has 59 modules and
// its binaries 32.
var defaultMinModules = map[string]int{modeSource: 50, modeBinary: 25}

var vulnIDPattern = regexp.MustCompile(`^GO-[0-9]{4}-[0-9]{4,}$`)

// scanFunc runs govulncheck with args and returns its stdout, its stderr and its
// exit code, or an error when it could not be started.
type scanFunc func(args []string) (stdout, stderr []byte, exitCode int, err error)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, goVulncheck))
}

func goVulncheck(args []string) ([]byte, []byte, int, error) {
	cmd := exec.Command("go", append([]string{"tool", "govulncheck"}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), stderr.Bytes(), exitErr.ExitCode(), nil
	}
	return stdout.Bytes(), stderr.Bytes(), 0, err
}

// options is the parsed command line.
type options struct {
	mode       string
	minModules int
	binaries   int
	accepted   map[string]acceptEntry
	targets    []string
}

// run parses the command line, scans every target and prints the report. A command
// line or accept file it cannot use counts as not having run, so a mistyped CI step
// cannot pass.
func run(args []string, stdout, stderr io.Writer, scan scanFunc) int {
	opts, err := parseArgs(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "vulnscan: %v\nvulnscan: DID NOT COMPLETE\n", err)
		return exitDidNotRun
	}
	r := &report{opts: opts, vulns: map[string]*vuln{}}
	for _, target := range opts.targets {
		r.scan(target, scan)
	}
	return r.print(stdout)
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	flags := flag.NewFlagSet("vulnscan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	mode := flags.String("mode", "", "source (scan package patterns) or binary (scan binaries)")
	acceptPath := flags.String("accept", "", "the accept file")
	minModules := flags.Int("min-modules", 0, fmt.Sprintf(
		"the fewest modules each scan must find (default %d for source, %d for binary)",
		defaultMinModules[modeSource], defaultMinModules[modeBinary]))
	binaries := flags.Int("binaries", 0, "the number of binaries given, in binary mode")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	opts := options{
		mode:       *mode,
		minModules: *minModules,
		binaries:   *binaries,
		accepted:   map[string]acceptEntry{},
		targets:    flags.Args(),
	}

	switch opts.mode {
	case modeSource:
		if len(opts.targets) == 0 {
			return options{}, errors.New("source mode needs a package pattern (./... for every package)")
		}
		if opts.binaries != 0 {
			return options{}, errors.New("-binaries is for binary mode only")
		}
	case modeBinary:
		if err := checkBinaries(opts.targets, opts.binaries); err != nil {
			return options{}, err
		}
	default:
		err := fmt.Errorf("-mode is %q, want %s or %s", opts.mode, modeSource, modeBinary)
		return options{}, err
	}

	if opts.minModules < 0 {
		return options{}, fmt.Errorf("-min-modules is %d, want 0 or more", opts.minModules)
	}
	if opts.minModules == 0 {
		opts.minModules = defaultMinModules[opts.mode]
	}

	if *acceptPath != "" {
		entries, err := readAcceptFile(*acceptPath)
		if err != nil {
			return options{}, err
		}
		for _, entry := range entries {
			if entry.mode == opts.mode {
				opts.accepted[entry.id] = entry
			}
		}
	}
	return opts, nil
}

// checkBinaries checks that there are as many binaries as expected and that each
// one is a file, so a build that lost a platform cannot pass.
func checkBinaries(paths []string, want int) error {
	if want <= 0 {
		return errors.New("binary mode needs -binaries, the number of binaries expected")
	}
	if len(paths) != want {
		return fmt.Errorf("%d binaries, want %d", len(paths), want)
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("binary: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("binary %s is not a regular file", path)
		}
	}
	return nil
}

// acceptEntry is one line of the accept file.
type acceptEntry struct {
	id     string
	mode   string
	reason string
}

func readAcceptFile(path string) ([]acceptEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("accept file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var entries []acceptEntry
	seen := map[string]bool{}
	lines := bufio.NewScanner(file)
	for number := 1; lines.Scan(); number++ {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		where := fmt.Sprintf("accept file %s line %d", path, number)
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, fmt.Errorf("%s: want an ID, a mode and a reason", where)
		}
		entry := acceptEntry{id: fields[0], mode: fields[1], reason: strings.Join(fields[2:], " ")}
		if !vulnIDPattern.MatchString(entry.id) {
			return nil, fmt.Errorf("%s: %q is not a Go vulnerability ID", where, entry.id)
		}
		if entry.mode != modeSource && entry.mode != modeBinary {
			return nil, fmt.Errorf("%s: mode %q, want %s or %s",
				where, entry.mode, modeSource, modeBinary)
		}
		key := entry.id + " " + entry.mode
		if seen[key] {
			return nil, fmt.Errorf("%s: %s is listed twice for %s mode",
				where, entry.id, entry.mode)
		}
		seen[key] = true
		entries = append(entries, entry)
	}
	if err := lines.Err(); err != nil {
		return nil, fmt.Errorf("accept file %s: %w", path, err)
	}
	return entries, nil
}

// level is how far govulncheck traced a vulnerability into the scanned code.
type level int

const (
	levelRequired level = iota // the module is in the build; no vulnerable package is imported
	levelImported              // a vulnerable package is imported; no vulnerable symbol is called
	levelCalled                // a vulnerable symbol is called, or linked into the binary
)

func (l level) String() string {
	switch l {
	case levelRequired:
		return "required, not imported"
	case levelImported:
		return "imported, not called"
	case levelCalled:
		return "called"
	}
	return fmt.Sprintf("level %d", int(l))
}

// vuln is one vulnerability across every scanned target.
type vuln struct {
	id      string
	summary string
	level   level
	module  string
	version string
	fixed   string
	symbols []string
	targets []string
}

// message is one object of govulncheck's -format json output, a stream of objects
// that each have one field.
type message struct {
	Config  *scanConfig `json:"config"`
	SBOM    *sbom       `json:"SBOM"`
	OSV     *osvEntry   `json:"osv"`
	Finding *finding    `json:"finding"`
}

type scanConfig struct {
	ScannerVersion string `json:"scanner_version"`
	DBLastModified string `json:"db_last_modified"`
	ScanLevel      string `json:"scan_level"`
	ScanMode       string `json:"scan_mode"`
}

type sbom struct {
	GoVersion string `json:"go_version"`
	Modules   []struct {
		Path string `json:"path"`
	} `json:"modules"`
}

type osvEntry struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

type finding struct {
	OSV          string  `json:"osv"`
	FixedVersion string  `json:"fixed_version"`
	Trace        []frame `json:"trace"`
}

type frame struct {
	Module   string `json:"module"`
	Version  string `json:"version"`
	Package  string `json:"package"`
	Function string `json:"function"`
	Receiver string `json:"receiver"`
}

// report collects the scans of every target and decides the exit code.
type report struct {
	opts       options
	lines      []string
	vulns      map[string]*vuln
	scanned    int
	incomplete bool
	exit3      bool
}

func (r *report) logf(format string, args ...any) {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

// scan runs govulncheck on one target and adds its findings to the report.
func (r *report) scan(target string, scan scanFunc) {
	args := []string{"-mode", r.opts.mode, "-format", "json", target}
	stdout, stderr, code, err := scan(args)
	switch {
	case err != nil:
		r.incomplete = true
		r.logf("%s: could not run govulncheck: %v", target, err)
		return
	case code == govulncheckFound:
		r.exit3 = true
		r.logf("%s: govulncheck exit 3: vulnerabilities found", target)
	case code != 0:
		r.incomplete = true
		r.logf("%s: govulncheck exit %d: %s", target, code, strings.TrimSpace(string(stderr)))
		return
	}

	config, goVersion, modules, err := r.parse(target, stdout)
	if err != nil {
		// Output that cannot be read after exit 3 still reports vulnerabilities.
		if code != govulncheckFound {
			r.incomplete = true
		}
		r.logf("%s: %v", target, err)
		return
	}
	r.scanned++
	r.logf("%s scan of %s: %d modules (at least %d expected); govulncheck %s, %s, database of %s",
		r.opts.mode, target, modules, r.opts.minModules,
		config.ScannerVersion, goVersion, config.DBLastModified)
}

// parse reads one govulncheck -format json output and returns its config, Go
// version and module count. It adds the findings to the report only when the
// output is complete: govulncheck writes its config, then the modules it found,
// then the vulnerability database entries for them, and the standard library
// always has some.
func (r *report) parse(target string, output []byte) (scanConfig, string, int, error) {
	var config *scanConfig
	var modules *sbom
	summaries := map[string]string{}
	var findings []finding
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var m message
		err := decoder.Decode(&m)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			err = fmt.Errorf("govulncheck output is not complete JSON: %w", err)
			return scanConfig{}, "", 0, err
		}
		switch {
		case m.Config != nil:
			config = m.Config
		case m.SBOM != nil:
			modules = m.SBOM
		case m.OSV != nil:
			summaries[m.OSV.ID] = m.OSV.Summary
		case m.Finding != nil:
			if m.Finding.OSV == "" || len(m.Finding.Trace) == 0 {
				err := errors.New("govulncheck output has a finding without an ID or a trace")
				return scanConfig{}, "", 0, err
			}
			findings = append(findings, *m.Finding)
		}
	}

	var err error
	switch {
	case config == nil:
		err = errors.New("govulncheck output has no config: it is empty or not -format json")
	case config.ScanMode != r.opts.mode:
		err = fmt.Errorf("govulncheck output is of scan mode %q, want %q",
			config.ScanMode, r.opts.mode)
	case config.ScanLevel != "symbol":
		err = fmt.Errorf("govulncheck output is of scan level %q, want symbol", config.ScanLevel)
	case modules == nil:
		err = errors.New("govulncheck output has no module list: it was cut short")
	case len(summaries) == 0:
		err = errors.New("govulncheck output has no vulnerability database entries: " +
			"it was cut short")
	case len(modules.Modules) < r.opts.minModules:
		err = fmt.Errorf("govulncheck found %d modules, fewer than the %d expected; lower "+
			"-min-modules only if the build lost modules on purpose",
			len(modules.Modules), r.opts.minModules)
	}
	if err != nil {
		return scanConfig{}, "", 0, err
	}

	for _, f := range findings {
		r.add(target, f, summaries[f.OSV])
	}
	return *config, modules.GoVersion, len(modules.Modules), nil
}

// add merges one finding into the report. govulncheck reports a vulnerability at
// the module level first, then again for each package and symbol it traces it to.
func (r *report) add(target string, f finding, summary string) {
	v, ok := r.vulns[f.OSV]
	if !ok {
		v = &vuln{id: f.OSV, summary: summary}
		r.vulns[f.OSV] = v
	}
	top := f.Trace[0]
	found := levelRequired
	switch {
	case top.Function != "":
		found = levelCalled
		symbol := filepath.Base(top.Package) + "."
		if top.Receiver != "" {
			symbol += strings.TrimPrefix(top.Receiver, "*") + "."
		}
		symbol += top.Function
		if !slices.Contains(v.symbols, symbol) {
			v.symbols = append(v.symbols, symbol)
		}
	case top.Package != "":
		found = levelImported
	}
	v.level = max(v.level, found)
	if top.Module != "" {
		v.module, v.version = top.Module, top.Version
	}
	if f.FixedVersion != "" {
		v.fixed = f.FixedVersion
	}
	if !slices.Contains(v.targets, target) {
		v.targets = append(v.targets, target)
	}
}

// print writes the report and returns the exit code.
func (r *report) print(w io.Writer) int {
	var out strings.Builder
	for _, line := range r.lines {
		out.WriteString(line + "\n")
	}

	toFix, accepted := 0, 0
	for _, id := range slices.Sorted(maps.Keys(r.vulns)) {
		if r.printVuln(&out, r.vulns[id]) {
			accepted++
		} else {
			toFix++
		}
	}

	// An entry is stale only when every scan completed: a scan that did not may have
	// missed the vulnerability it accepts.
	var stale []string
	allScanned := r.scanned == len(r.opts.targets)
	for id := range r.opts.accepted {
		if _, reported := r.vulns[id]; allScanned && !reported {
			stale = append(stale, id)
		}
	}
	slices.Sort(stale)
	for _, id := range stale {
		_, _ = fmt.Fprintf(&out, "\n%s  FAIL, stale accept entry: %s mode no longer reports it; "+
			"remove it from the accept file\n", id, r.opts.mode)
	}
	if !allScanned && len(r.opts.accepted) > 0 {
		_, _ = fmt.Fprintf(&out, "\naccept entries not checked for staleness: "+
			"not every scan completed\n")
	}

	code := exitClean
	switch {
	case r.incomplete:
		code = exitDidNotRun
	case toFix > 0 || len(stale) > 0 || r.exit3:
		code = exitFindings
	}
	verdict := map[int]string{
		exitClean:     "PASS",
		exitFindings:  "FAIL",
		exitDidNotRun: "DID NOT COMPLETE",
	}
	_, _ = fmt.Fprintf(&out, "\nvulnscan: %s: %d to fix, %d accepted, %d stale accept entries, "+
		"%d of %d scans complete\n",
		verdict[code], toFix, accepted, len(stale), r.scanned, len(r.opts.targets))
	_, _ = io.WriteString(w, out.String())
	return code
}

// printVuln writes one vulnerability and reports whether it is accepted.
func (r *report) printVuln(out io.Writer, v *vuln) bool {
	entry, listed := r.opts.accepted[v.id]
	isAccepted := listed && v.level != levelCalled
	verdict := "FAIL, " + v.level.String() + ", not accepted"
	switch {
	case isAccepted:
		verdict = "accepted, " + v.level.String()
	case listed:
		verdict = "FAIL, called; the accept entry does not apply: called code cannot be accepted"
	}
	fixed := "no fixed version"
	if v.fixed != "" {
		fixed = "fixed in " + v.fixed
	}
	_, _ = fmt.Fprintf(out, "\n%s  %s\n    %s %s, %s\n    %s\n",
		v.id, verdict, v.module, v.version, fixed, v.summary)
	if len(v.symbols) > 0 {
		_, _ = fmt.Fprintf(out, "    calls: %s\n", strings.Join(v.symbols, ", "))
	}
	switch {
	case r.opts.mode != modeBinary:
	case len(v.targets) == len(r.opts.targets):
		_, _ = fmt.Fprintf(out, "    in: all %d binaries\n", len(v.targets))
	default:
		_, _ = fmt.Fprintf(out, "    in: %s\n", strings.Join(v.targets, ", "))
	}
	if isAccepted {
		_, _ = fmt.Fprintf(out, "    accepted: %s\n", entry.reason)
	}
	return isAccepted
}
