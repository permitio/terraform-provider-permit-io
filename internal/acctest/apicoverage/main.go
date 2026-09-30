// Command apicoverage maps the provider's calls to the operations of the Permit
// API's OpenAPI spec, checks the triage of every operation, and decodes fixtures
// of every response the provider reads with the SDK's models. It prints a
// markdown report.
//
// The spec is vendored in coverage/openapi.json and pinned by
// coverage/spec.lock.json, so a run is offline and deterministic. It is vendored
// without descriptions, summaries, titles and examples, which hold sample data
// and UUIDs that change on every fetch, indented and with sorted keys; -update-lock
// writes that form. With -live, it checks a freshly fetched spec instead and also
// reports how it drifted from the vendored one, leaving those keys aside.
//
// A call site is a call in the provider's non-test source that sends requests: a
// method of the SDK's Api, an operation of the SDK's generated client, or a
// net/http request the provider builds itself. apicoverage loads the provider and
// the SDK with go/packages, follows each SDK Api method through the SDK's source
// to the generated client operations it calls, and reads each operation's method
// and path from its Execute method. A request the provider builds itself takes
// its route from mockpermit's route table, whose tests check it against the
// function that sends it, and each SDK call's route is checked against that table
// too. Calls are named as mockpermit names its routes' operations.
//
// mockpermit's TestProviderCallSitesHaveRoutes walks the same source by syntax
// alone, in a test file this command cannot import. That walk cannot follow an
// Api method to the generated client operations it calls, nor see a direct call
// of the generated client, so this one loads types. mockpermit's route table ties
// the two together: this walk fails on a call that has no route there, and
// mockpermit's walk fails on a route for a call it does not find, outside its
// releasedOnly list.
//
// coverage/operations.yaml triages every operation of the spec: covered (a call
// site sends it, with the calls that do), equivalent, inline, planned (with a
// ticket) or excluded, with the Terraform surface it is for and a reason from a
// fixed vocabulary. It also lists the decode failures that are known.
//
// The decode check builds, for each covered operation whose response the SDK
// decodes into a model, a fixture from the spec with a field the spec does not
// have, and a fixture for every enum value and every anyOf or oneOf alternative,
// and decodes each as the SDK's generated client does.
//
// Exit codes:
//
//	0  every operation is triaged, the triage matches the call sites, and every
//	   decode failure is known
//	1  an operation is untriaged, an entry is stale or wrong (a covered operation
//	   no call site sends, a call the entry does not list, an unknown reason, an
//	   exclusion as not-ga or deprecated that the spec no longer backs, an EAP or
//	   deprecated operation excluded for another reason or covered without a
//	   ticket), a call sends a route the spec does not have or has no route in
//	   mockpermit, a decode failure is not known or a known one no longer fails,
//	   or the live spec drifted
//	2  the check did not run: the spec is unreadable, truncated, not the one the
//	   lock file pins or not in the vendored form, the triage file cannot be
//	   read, the packages do not load, or a sentinel fails (fewer than 250
//	   operations or 40 tags in the spec, 50 SDK call sites, 150 generated client
//	   operations, 14 resources or 5 data sources)
//
// Run it from the repository root. Build it first; go run reports every non-zero
// exit as 1:
//
//	go build -o apicoverage ./internal/acctest/apicoverage
//	./apicoverage                   # check the vendored spec
//	./apicoverage -live live.json   # check a fetched spec and report its drift
//	./apicoverage -update-lock      # vendor and pin coverage/openapi.json after replacing it
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	exitClean     = 0
	exitFindings  = 1
	exitDidNotRun = 2
)

// The files in the coverage directory.
const (
	specFile   = "openapi.json"
	lockName   = "spec.lock.json"
	triageName = "operations.yaml"
)

// gatherFunc walks the module at root.
type gatherFunc func(root string) (facts, error)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, gatherFacts, time.Now))
}

type options struct {
	dir, root, live string
	updateLock      bool
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	flags := flag.NewFlagSet("apicoverage", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var opts options
	flags.StringVar(&opts.dir, "dir", "coverage", "the directory with the spec, lock and triage")
	flags.StringVar(&opts.root, "root", ".", "the module root")
	flags.StringVar(&opts.live, "live", "", "a fetched spec to check instead of the vendored one")
	flags.BoolVar(&opts.updateLock, "update-lock", false,
		"pin the vendored spec in the lock file and exit")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() > 0 {
		return opts, fmt.Errorf("unexpected arguments %q", flags.Args())
	}
	return opts, nil
}

// run checks the coverage and prints the report. A command line or input it
// cannot use counts as not having run, so a mistyped CI step cannot pass.
func run(args []string, stdout, stderr io.Writer, gather gatherFunc,
	now func() time.Time,
) int {
	opts, err := parseArgs(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "apicoverage: %v\napicoverage: DID NOT RUN\n", err)
		return exitDidNotRun
	}
	if opts.updateLock {
		return updateLock(opts.dir, stdout, stderr, now)
	}
	r := check(opts, gather, defaultMinimums)
	writeReport(stdout, r)
	return r.exitCode()
}

// check runs every check and returns the result. It stops at the first input it
// cannot use.
func check(opts options, gather gatherFunc, least minimums) *runResult {
	r := &runResult{specSource: filepath.ToSlash(filepath.Join(opts.dir, specFile))}
	notRun := func(err error) *runResult {
		r.notRun = append(r.notRun, err.Error())
		return r
	}
	data, err := os.ReadFile(filepath.Join(opts.dir, specFile))
	if err != nil {
		return notRun(err)
	}
	lock, err := readLock(filepath.Join(opts.dir, lockName))
	if err != nil {
		return notRun(err)
	}
	r.lock = lock
	if err := checkLock(lock, data); err != nil {
		return notRun(err)
	}
	vendored, err := parseSpec(data)
	if err != nil {
		return notRun(err)
	}
	canonical, err := canonicalSpec(data)
	if err != nil {
		return notRun(err)
	}
	if !bytes.Equal(canonical, data) {
		return notRun(fmt.Errorf("%s is not in the form apicoverage -update-lock vendors, "+
			"without descriptions, summaries, titles and examples; run apicoverage -update-lock",
			r.specSource))
	}
	r.spec = vendored
	if opts.live != "" {
		liveData, err := os.ReadFile(opts.live)
		if err != nil {
			return notRun(err)
		}
		live, err := parseSpec(liveData)
		if err != nil {
			return notRun(fmt.Errorf("the live spec: %w", err))
		}
		r.spec = live
		r.liveSHA256 = sha256Hex(liveData)
		d := diffSpecs(vendored, live)
		r.drift = &d
	}
	r.notRun = append(r.notRun, specSentinels(r.spec, least)...)
	triageData, err := os.ReadFile(filepath.Join(opts.dir, triageName))
	if err != nil {
		return notRun(err)
	}
	triage, err := parseTriage(triageData)
	if err != nil {
		return notRun(err)
	}
	if len(r.notRun) > 0 {
		return r
	}
	f, err := gather(opts.root)
	if err != nil {
		return notRun(err)
	}
	if r.notRun = factSentinels(f, least); len(r.notRun) > 0 {
		return r
	}

	r.coverage = evaluate(r.spec, triage, f)
	r.decode = checkDecoding(r.spec, r.coverage.coveredOperations(), f.models,
		triage.KnownDecodeFailures)
	r.findings = append(r.findings, r.coverage.findings...)
	r.findings = append(r.findings, r.decode.findings...)
	if r.drift != nil && !r.drift.empty() {
		r.findings = append(r.findings, "drift: the live spec differs from the vendored one; "+
			"update coverage/openapi.json and its lock, and triage the changes")
	}
	return r
}

func readLock(path string) (lockFile, error) {
	var lock lockFile
	data, err := os.ReadFile(path)
	if err != nil {
		return lock, err
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return lock, fmt.Errorf("reading %s: %w", path, err)
	}
	return lock, nil
}

// updateLock vendors the spec in coverage/openapi.json, such as one just
// fetched, in its canonical form and pins it: it sets the lock file's SHA-256 to
// that form's and fetched_at to now, keeping its URL. It refuses a spec that does
// not parse or fails a sentinel.
func updateLock(dir string, stdout, stderr io.Writer, now func() time.Time) int {
	fail := func(err error) int {
		_, _ = fmt.Fprintf(stderr, "apicoverage: %v\napicoverage: DID NOT RUN\n", err)
		return exitDidNotRun
	}
	data, err := os.ReadFile(filepath.Join(dir, specFile))
	if err != nil {
		return fail(err)
	}
	s, err := parseSpec(data)
	if err != nil {
		return fail(err)
	}
	if failed := specSentinels(s, defaultMinimums); len(failed) > 0 {
		return fail(errors.New(failed[0]))
	}
	lockPath := filepath.Join(dir, lockName)
	lock, err := readLock(lockPath)
	if err != nil {
		return fail(err)
	}
	if lock.URL == "" {
		return fail(fmt.Errorf("%s has no url", lockPath))
	}
	canonical, err := canonicalSpec(data)
	if err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(dir, specFile), canonical, 0o644); err != nil {
		return fail(err)
	}
	lock.SHA256 = sha256Hex(canonical)
	lock.FetchedAt = now().UTC().Format(time.RFC3339)
	encoded, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fail(err)
	}
	if err := os.WriteFile(lockPath, append(encoded, '\n'), 0o644); err != nil {
		return fail(err)
	}
	_, _ = fmt.Fprintf(stdout, "pinned %s: sha256 %s, fetched_at %s\n",
		filepath.Join(dir, specFile), lock.SHA256, lock.FetchedAt)
	return exitClean
}
