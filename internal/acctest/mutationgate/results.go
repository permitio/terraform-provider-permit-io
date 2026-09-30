package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"
)

// The statuses gremlins 0.6.0 gives a mutant in its JSON output.
const (
	statusKilled     = "KILLED"
	statusLived      = "LIVED"
	statusTimedOut   = "TIMED OUT"
	statusNotCovered = "NOT COVERED"
	statusNotViable  = "NOT VIABLE"
	statusSkipped    = "SKIPPED"
	statusRunnable   = "RUNNABLE"
)

var knownStatuses = []string{
	statusKilled, statusLived, statusTimedOut, statusNotCovered, statusNotViable, statusSkipped,
	statusRunnable,
}

// mutation is one mutant gremlins reports.
type mutation struct {
	file   string
	line   int
	column int
	kind   string
	status string
}

func (m mutation) String() string { return fmt.Sprintf("%s:%d:%d", m.file, m.line, m.column) }

type gremlinsOutput struct {
	Files []struct {
		FileName  string `json:"file_name"`
		Mutations []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		} `json:"mutations"`
	} `json:"files"`
}

// readResults reads gremlins' --output file. gremlins writes none when it finds no
// mutant at all, so a missing file means it mutated nothing.
func readResults(p string) ([]mutation, error) {
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errors.New("gremlins wrote no results: it found nothing to mutate in the module")
	}
	if err != nil {
		return nil, err
	}
	var out gremlinsOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("gremlins results: %w", err)
	}
	var mutations []mutation
	for _, f := range out.Files {
		for _, m := range f.Mutations {
			if f.FileName == "" || m.Line < 1 || !slices.Contains(knownStatuses, m.Status) {
				return nil, fmt.Errorf("gremlins results: unexpected mutant %q line %d status %q",
					f.FileName, m.Line, m.Status)
			}
			mutations = append(mutations, mutation{
				file: f.FileName, line: m.Line, column: m.Column, kind: m.Type, status: m.Status,
			})
		}
	}
	slices.SortFunc(mutations, func(a, b mutation) int {
		return cmp.Or(cmp.Compare(a.file, b.file), cmp.Compare(a.line, b.line),
			cmp.Compare(a.column, b.column), cmp.Compare(a.kind, b.kind))
	})
	return mutations, nil
}

// event is a test process that exec stopped, or a limit it could not apply.
type event struct {
	kind   string
	binary string
	detail string
}

// readEvents reads the log exec appends to. No log means no event.
func readEvents(p string) ([]event, error) {
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var events []event
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := strings.SplitN(sc.Text(), "\t", 3)
		if len(fields) != 3 {
			return nil, fmt.Errorf("exec log: unexpected line %q", sc.Text())
		}
		events = append(events, event{kind: fields[0], binary: fields[1], detail: fields[2]})
	}
	return events, sc.Err()
}

// report is everything the gate knows about a run.
type report struct {
	opts      gateOptions
	limits    string
	scope     scope
	elapsed   time.Duration
	events    []event
	mutations []mutation
}

// outcome is a run's verdict.
type outcome struct {
	exit     int
	notice   string
	counts   map[string]int
	detected int
	tested   int
	efficacy float64
	listed   []mutation
}

// evaluate decides the verdict from gremlins' results and exec's log.
func (r report) evaluate() outcome {
	for _, e := range r.events {
		if e.kind == eventError {
			return outcome{exit: exitDidNotRun, notice: fmt.Sprintf(
				"the gate's exec or go subcommand could not do its part: %s: %s", e.binary, e.detail)}
		}
	}
	var mismatched []mutation
	counts := map[string]int{}
	for _, m := range r.mutations {
		skipped := m.status == statusSkipped
		if r.scope.all && skipped || !r.scope.all && skipped == r.scope.isChanged(m.file, m.line) {
			mismatched = append(mismatched, m)
		}
		counts[m.status]++
	}
	if len(mismatched) > 0 {
		return outcome{exit: exitDidNotRun, listed: mismatched, notice: fmt.Sprintf(
			"gremlins' changed lines differ from git diff's for %d mutants (listed below)", len(mismatched))}
	}
	if counts[statusRunnable] > 0 {
		return outcome{exit: exitDidNotRun, counts: counts, notice: fmt.Sprintf(
			"gremlins left %d mutants unrun", counts[statusRunnable])}
	}
	inScope := len(r.mutations) - counts[statusSkipped]
	if inScope == 0 {
		if r.scope.all {
			return outcome{exit: exitDidNotRun, notice: "gremlins found nothing to mutate in the module"}
		}
		return outcome{exit: exitPassed, counts: counts, notice: fmt.Sprintf(
			"The lines changed since the merge base with %s hold nothing gremlins mutates.", r.scope.base)}
	}
	o := outcome{counts: counts, detected: counts[statusKilled] + counts[statusTimedOut]}
	o.tested = o.detected + counts[statusLived]
	if o.tested == 0 {
		if !r.scope.all && counts[statusNotViable] == inScope {
			o.exit = exitPassed
			o.notice = fmt.Sprintf("None of the %d mutants on the lines changed since the merge base "+
				"with %s compiles, such as - between two strings, so there is nothing for the tests "+
				"to detect.", inScope, r.scope.base)
			return o
		}
		o.exit = exitDidNotRun
		o.notice = fmt.Sprintf("No mutant ran: of the %d mutants in scope, %d are on lines no "+
			"test covers and %d do not compile", inScope, counts[statusNotCovered],
			counts[statusNotViable])
		return o
	}
	o.efficacy = 100 * float64(o.detected) / float64(o.tested)
	o.notice = fmt.Sprintf("The tests detected %d of the %d mutants that ran: %.2f%%, threshold %g%%.",
		o.detected, o.tested, o.efficacy, r.opts.threshold)
	if n := counts[statusNotViable]; n > 0 {
		o.notice += fmt.Sprintf(" %d mutants that do not compile are not counted.", n)
	}
	if o.efficacy < r.opts.threshold {
		o.exit = exitFailed
	}
	return o
}

// finish prints the report, appends it to the summary file and returns the exit
// code.
func (r report) finish(out sink, o outcome) int {
	text := r.markdown(o)
	_, _ = io.WriteString(out.stdout, text)
	if out.summary != "" {
		if err := appendFile(out.summary, text); err != nil {
			out.summary = ""
			return out.didNotRun(fmt.Errorf("job summary: %w", err), nil)
		}
	}
	switch o.exit {
	case exitPassed:
		annotate(out.stdout, "notice", o.notice)
	case exitFailed:
		annotate(out.stdout, "error", o.notice+" The job summary lists the surviving mutants.")
	default:
		annotate(out.stdout, "error", "The mutation tests did not complete: "+o.notice)
	}
	return o.exit
}

func appendFile(p, text string) error {
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (r report) markdown(o outcome) string {
	var b strings.Builder
	verdict := map[int]string{exitPassed: "PASSED", exitFailed: "FAILED"}[o.exit]
	if verdict == "" {
		verdict = "DID NOT RUN"
	}
	fmt.Fprintf(&b, "### Mutation tests\n\n**%s**: %s\n\n", verdict, o.notice)
	if r.scope.all {
		b.WriteString("Scope: every Go file in the module.\n\n")
	} else {
		fmt.Fprintf(&b, "Scope: the lines changed since the merge base with `%s`; changed files: %d.\n\n",
			r.scope.base, len(r.scope.changed))
	}
	fmt.Fprintf(&b, "Limits: %s.\n\n", r.limits)
	if r.elapsed > 0 {
		fmt.Fprintf(&b, "gremlins ran for %s.\n\n", r.elapsed.Round(time.Second))
	}
	if o.counts != nil {
		b.WriteString("| Killed | Timed out | Lived | Not covered | Not viable |\n" +
			"|---|---|---|---|---|\n")
		fmt.Fprintf(&b, "| %d | %d | %d | %d | %d |\n\n", o.counts[statusKilled],
			o.counts[statusTimedOut], o.counts[statusLived], o.counts[statusNotCovered],
			o.counts[statusNotViable])
		b.WriteString("Killed and timed-out mutants count as detected; efficacy is detected / " +
			"(detected + lived). Mutants that do not compile are not counted.\n\n")
	}
	writeBaseline(&b, r.events)
	writeEvents(&b, r.events)
	writeMutations(&b, "Surviving mutants", r.withStatus(statusLived))
	writeMutations(&b, "Mutants on lines no test covers", r.withStatus(statusNotCovered))
	writeMutations(&b, "Mutants whose changed lines differ between gremlins and git diff", o.listed)
	notMutated, what := r.scope.notMutated, "Changed files"
	if r.scope.all {
		notMutated, what = r.scope.excludedDirs, "Packages"
	}
	if len(notMutated) > 0 {
		fmt.Fprintf(&b, "#### %s not mutated (%d)\n\ngremlins 0.6.0 would test them "+
			"with another package's tests: their package is main in a subdirectory, or its name "+
			"differs from its directory.\n\n", what, len(notMutated))
		for _, f := range notMutated {
			fmt.Fprintf(&b, "- `%s`\n", f)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (r report) withStatus(status string) []mutation {
	var out []mutation
	for _, m := range r.mutations {
		if m.status == status {
			out = append(out, m)
		}
	}
	return out
}

// writeBaseline reports the slowest test process of gremlins' coverage run, which
// ran the tests without a mutant.
func writeBaseline(b *strings.Builder, events []event) {
	var slowest event
	var longest time.Duration
	for _, e := range events {
		if e.kind != eventBaseline {
			continue
		}
		if d, err := time.ParseDuration(e.detail); err == nil && d >= longest {
			slowest, longest = e, d
		}
	}
	if slowest.binary != "" {
		fmt.Fprintf(b, "Slowest test process without a mutant: `%s`, %s.\n\n", slowest.binary, longest)
	}
}

// writeEvents lists the test processes exec stopped or saw run out of memory, and
// the errors exec and go logged.
func writeEvents(b *strings.Builder, events []event) {
	var listed []event
	for _, e := range events {
		if e.kind != eventBaseline {
			listed = append(listed, e)
		}
	}
	if len(listed) == 0 {
		return
	}
	fmt.Fprintf(b, "#### Test processes exec stopped, and errors (%d)\n\nA mutant whose test "+
		"process was stopped or ran out of memory counts as killed.\n\n", len(listed))
	for _, e := range listed {
		fmt.Fprintf(b, "- `%s` (%s): %s\n", e.binary, e.kind, e.detail)
	}
	b.WriteString("\n")
}

func writeMutations(b *strings.Builder, title string, mutations []mutation) {
	if len(mutations) == 0 {
		return
	}
	fmt.Fprintf(b, "#### %s (%d)\n\n| Mutant | Type | Status |\n|---|---|---|\n",
		title, len(mutations))
	for _, m := range mutations {
		fmt.Fprintf(b, "| `%s` | %s | %s |\n", m, m.kind, m.status)
	}
	b.WriteString("\n")
}

// annotate prints a GitHub Actions annotation when running there.
func annotate(w io.Writer, level, message string) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return
	}
	escaped := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(message)
	_, _ = fmt.Fprintf(w, "::%s title=Mutation tests::%s\n", level, escaped)
}
