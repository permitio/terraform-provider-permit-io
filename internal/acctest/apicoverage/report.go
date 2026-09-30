package main

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// runResult is everything the report shows.
type runResult struct {
	specSource string
	spec       *spec
	lock       lockFile
	notRun     []string
	coverage   *coverage
	decode     decodeCheck
	drift      *drift
	// liveSHA256 is the SHA-256 of the live spec, in -live mode.
	liveSHA256 string
	findings   []string
}

func (r *runResult) exitCode() int {
	switch {
	case len(r.notRun) > 0:
		return exitDidNotRun
	case len(r.findings) > 0:
		return exitFindings
	default:
		return exitClean
	}
}

// writeReport writes the markdown report.
func writeReport(w io.Writer, r *runResult) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format+"\n", args...) }
	p("## Permit API coverage")
	p("")
	switch r.exitCode() {
	case exitDidNotRun:
		p("**DID NOT RUN**")
		p("")
		for _, reason := range r.notRun {
			p("- %s", reason)
		}
		return
	case exitFindings:
		p("**FAIL: %d findings** (listed at the end)", len(r.findings))
	default:
		p("**PASS**")
	}
	p("")
	p("Spec: version %s from %s, SHA-256 `%s`, fetched %s.", r.spec.version,
		r.specSource, r.lock.SHA256, r.lock.FetchedAt)
	if r.liveSHA256 != "" {
		p("Checked against the live spec, SHA-256 `%s`.", r.liveSHA256)
	}
	p("")
	writeSummary(p, r)
	writeCovered(p, r.coverage)
	writeDecode(p, r.decode)
	if r.drift != nil {
		writeDrift(p, *r.drift)
	}
	if len(r.findings) > 0 {
		p("### Findings")
		p("")
		for _, finding := range r.findings {
			p("- %s", finding)
		}
	}
}

func writeSummary(p func(string, ...any), r *runResult) {
	c := r.coverage
	active, sentActive := 0, 0
	for _, op := range c.spec.operations {
		if op.active() {
			active++
			if len(c.sent[op.id]) > 0 {
				sentActive++
			}
		}
	}
	sent := c.coveredOperations()
	sdkSites, httpSites := 0, 0
	calls := map[string]bool{}
	for _, site := range c.facts.sites {
		calls[site.call] = true
		if site.sdk {
			sdkSites++
		} else {
			httpSites++
		}
	}
	percent := 0.0
	if active > 0 {
		percent = float64(sentActive) * 100 / float64(active)
	}
	p("| | Count |")
	p("|---|---:|")
	p("| Operations in the spec | %d, with %d tags |", len(c.spec.operations), len(c.spec.tags))
	p("| GA (no EAP tag) and not deprecated | %d |", active)
	p("| Sent by the provider | %d |", len(sent))
	p("| Sent by the provider, GA and not deprecated | %d of %d (%.1f%%) |", sentActive, active,
		percent)
	p("| Provider call sites | %d: %d through the SDK, %d built with net/http; %d distinct calls |",
		len(c.facts.sites), sdkSites, httpSites, len(calls))
	p("| Resources and data sources walked | %d and %d |", c.facts.resources, c.facts.dataSources)
	p("")

	byStatus := map[string]int{}
	byReason := map[string]int{}
	for _, op := range c.spec.operations {
		if entry, ok := c.triage.Operations[op.id]; ok {
			byStatus[entry.Status]++
			if entry.Reason != "" {
				byReason[entry.Status+": "+entry.Reason]++
			}
		}
	}
	var parts []string
	for _, status := range statuses {
		parts = append(parts, fmt.Sprintf("%s %d", status, byStatus[status]))
	}
	p("Triage: %s.", strings.Join(parts, ", "))
	parts = nil
	for _, reason := range slices.Sorted(maps.Keys(byReason)) {
		parts = append(parts, fmt.Sprintf("%s %d", reason, byReason[reason]))
	}
	p("By reason: %s.", strings.Join(parts, ", "))
	p("")
}

func writeCovered(p func(string, ...any), c *coverage) {
	p("### Operations the provider sends")
	p("")
	p("Offline wire route: mockpermit serves the route to the offline tests.")
	p("")
	p("| Operation | Route | Surface | Calls | Offline wire route |")
	p("|---|---|---|---|---|")
	for _, id := range c.coveredOperations() {
		op := c.spec.byID[id]
		wired := "no"
		for _, site := range c.sent[id] {
			for _, r := range site.wire {
				if r.key() == op.route().key() {
					wired = "yes"
				}
			}
		}
		surface := strings.Join(c.triage.Operations[id].Surface, ", ")
		p("| %s | `%s` | %s | %s | %s |", id, op.route(), surface, siteCalls(c.sent[id]), wired)
	}
	p("")
}

func writeDecode(p func(string, ...any), d decodeCheck) {
	operations := 0
	for _, unit := range d.units {
		operations += len(unit.operations)
	}
	allowed := 0
	for _, failure := range d.failures {
		if failure.allowed != "" {
			allowed++
		}
	}
	p("### Decode check")
	p("")
	p("Response models: %d, of %d operations; fixtures: %d; failures: %d, %d of them known. "+
		"Not decoded into an SDK model: %s.", len(d.units), operations, d.fixtures,
		len(d.failures), allowed, strings.Join(d.untyped, ", "))
	p("")
	if len(d.failures) == 0 {
		return
	}
	p("| Model | Fixture | Error | Known |")
	p("|---|---|---|---|")
	for _, failure := range d.failures {
		known := "no"
		if failure.allowed != "" {
			known = failure.allowed
		}
		p("| `%s` | %s | %s | %s |", failure.unit.model, variantName(failure.result),
			strings.ReplaceAll(failure.result.err.Error(), "|", `\|`), known)
	}
	p("")
}

func writeDrift(p func(string, ...any), d drift) {
	p("### Drift of the live spec from the vendored one")
	p("")
	if d.empty() {
		p("None, leaving descriptions, summaries, titles and examples aside.")
		p("")
		return
	}
	list := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		p("%s:", title)
		p("")
		for _, item := range items {
			p("- %s", item)
		}
		p("")
	}
	list("Operations added", d.added)
	list("Operations removed", d.removed)
	list("Operations changed", d.changed)
	list("Schemas added", d.schemasAdded)
	list("Schemas removed", d.schemasRemoved)
	list("Schemas changed", d.schemasChanged)
	if d.other {
		p("Other parts of the spec changed.")
		p("")
	}
}
