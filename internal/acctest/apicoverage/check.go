package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// minimums are the sentinels: the least a run must read, or it did not run.
type minimums struct {
	operations, tags, sdkCallSites, clientOperations, resources, dataSources int
}

// defaultMinimums hold for the Permit API spec, the SDK and the provider as they
// are. The spec has 263 operations with 43 tags, the SDK's generated client 178
// operations, and the provider 58 SDK call sites, 14 resources and 5 data sources.
var defaultMinimums = minimums{
	operations: 250, tags: 40, sdkCallSites: 50, clientOperations: 150, resources: 14,
	dataSources: 5,
}

// specSentinels returns why a spec is too small to be the Permit API's.
func specSentinels(s *spec, least minimums) []string {
	var failed []string
	if len(s.operations) < least.operations {
		failed = append(failed, fmt.Sprintf("the spec has %d operations, expected at least %d",
			len(s.operations), least.operations))
	}
	if len(s.tags) < least.tags {
		failed = append(failed, fmt.Sprintf("the spec's operations have %d tags, expected at "+
			"least %d", len(s.tags), least.tags))
	}
	return failed
}

// factSentinels returns why the walk found too little to be the provider's.
func factSentinels(f facts, least minimums) []string {
	var failed []string
	sdkSites := 0
	for _, site := range f.sites {
		if site.sdk {
			sdkSites++
		}
	}
	atLeast := func(got, want int, what string) {
		if got < want {
			failed = append(failed,
				fmt.Sprintf("found %d %s, expected at least %d", got, what, want))
		}
	}
	atLeast(sdkSites, least.sdkCallSites, "SDK call sites in the provider")
	atLeast(f.clientOperations, least.clientOperations, "operations in the SDK's generated client")
	atLeast(f.resources, least.resources, "resources")
	atLeast(f.dataSources, least.dataSources, "data sources")
	return failed
}

// coverage is the result of checking the triage against the spec and the
// provider's call sites.
type coverage struct {
	spec   *spec
	triage triageFile
	facts  facts
	// sent maps an operation ID to the call sites that send it.
	sent     map[string][]callSite
	findings []string
}

// evaluate checks the triage file against the spec and the call sites.
func evaluate(s *spec, t triageFile, f facts) *coverage {
	c := &coverage{spec: s, triage: t, facts: f, sent: map[string][]callSite{}}
	c.findings = append(c.findings, f.problems...)
	for _, site := range f.sites {
		for _, r := range site.routes {
			op := s.byRoute[r.key()]
			if op == nil {
				c.addf("%s: %s sends %s, which is not in the spec", site.position, site.call, r)
				continue
			}
			c.sent[op.id] = append(c.sent[op.id], site)
		}
	}

	typePackages := map[string]string{}
	for dir, surfaces := range f.surfaces {
		for _, surface := range surfaces {
			typePackages[surface] = dir
		}
	}
	for _, op := range s.operations {
		entry, triaged := t.Operations[op.id]
		if !triaged {
			c.addf("untriaged: %s; add it to operations.yaml", describeOperation(op))
			continue
		}
		c.checkEntry(op, entry, typePackages)
	}
	for _, id := range slices.Sorted(maps.Keys(t.Operations)) {
		if s.byID[id] == nil {
			c.addf("stale: operations.yaml triages %s, which the spec does not have", id)
		}
	}
	return c
}

func (c *coverage) addf(format string, args ...any) {
	c.findings = append(c.findings, fmt.Sprintf(format, args...))
}

// checkEntry checks one operation's triage: its fields, its surfaces, and for a
// covered operation that the calls it lists are the ones that send it.
func (c *coverage) checkEntry(op specOperation, entry triageEntry, typePackages map[string]string) {
	for _, problem := range entry.validate() {
		c.addf("%s: %s", op.id, problem)
	}
	if finding := availabilityFinding(op, entry); finding != "" {
		c.findings = append(c.findings, finding)
	}
	surfaceDirs := map[string]bool{}
	for _, surface := range entry.Surface {
		typeName, _ := splitSurface(surface)
		dir, known := typePackages[typeName]
		if !known {
			c.addf("%s: surface %s is not a resource or data source of the provider", op.id,
				surface)
			continue
		}
		surfaceDirs[dir] = true
	}

	sites := c.sent[op.id]
	if entry.Status != statusCovered {
		if len(sites) > 0 {
			c.addf("%s is triaged %s, but %s sends it", op.id, entry.Status, siteCalls(sites))
		}
		return
	}
	if len(sites) == 0 {
		c.addf("stale: %s is triaged covered, but no call site sends it", op.id)
		return
	}
	actual := siteCallSet(sites)
	for _, call := range entry.Calls {
		if !actual[call] {
			c.addf("%s lists the call %s, which does not send it", op.id, call)
		}
	}
	for _, call := range slices.Sorted(maps.Keys(actual)) {
		if !slices.Contains(entry.Calls, call) {
			c.addf("%s is sent by %s; add the call to its calls", op.id, call)
		}
	}
	matched := false
	for _, site := range sites {
		matched = matched || surfaceDirs[site.pkg]
	}
	if len(surfaceDirs) > 0 && !matched {
		c.addf("%s: none of its surfaces %s is in a package that sends it (%s)", op.id,
			strings.Join(entry.Surface, ", "), siteCalls(sites))
	}
}

// availabilityFinding returns a finding when an entry no longer matches whether
// the spec has its operation GA and not deprecated: an exclusion as not-ga or
// deprecated that the spec no longer backs, an operation the spec deprecates or
// marks not GA that is excluded for another reason, and a covered operation the
// spec deprecates or marks not GA without a ticket to move the provider off it.
// It returns "" when the entry matches.
func availabilityFinding(op specOperation, entry triageEntry) string {
	state := "not GA"
	reason := reasonNotGA
	if op.deprecated {
		state = "deprecated"
		reason = reasonDeprecated
	}
	switch {
	case entry.Reason == reasonNotGA && op.ga():
		return fmt.Sprintf("stale: %s is excluded as not-ga, but the spec has it GA; "+
			"triage it again", op.id)
	case entry.Reason == reasonDeprecated && !op.deprecated:
		return fmt.Sprintf("stale: %s is excluded as deprecated, but the spec does not "+
			"deprecate it; triage it again", op.id)
	case op.active():
		return ""
	case entry.Status == statusExcluded && entry.Reason != reasonNotGA &&
		entry.Reason != reasonDeprecated:
		return fmt.Sprintf("%s is %s in the spec; exclude it as %s", op.id, state, reason)
	case entry.Status == statusCovered && entry.Ticket == "":
		return fmt.Sprintf("%s is covered, but the spec has it %s; move the provider off it, "+
			"or give its entry the ticket that will", op.id, state)
	}
	return ""
}

func siteCallSet(sites []callSite) map[string]bool {
	calls := map[string]bool{}
	for _, site := range sites {
		calls[site.call] = true
	}
	return calls
}

func siteCalls(sites []callSite) string {
	return strings.Join(slices.Sorted(maps.Keys(siteCallSet(sites))), ", ")
}

// coveredOperations returns the IDs of the operations a call site sends, in spec
// order.
func (c *coverage) coveredOperations() []string {
	var ids []string
	for _, op := range c.spec.operations {
		if len(c.sent[op.id]) > 0 {
			ids = append(ids, op.id)
		}
	}
	return ids
}
