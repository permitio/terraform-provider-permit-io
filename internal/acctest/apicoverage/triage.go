package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The statuses of a triaged operation.
const (
	// statusCovered: a call site of the provider sends the operation.
	statusCovered = "covered"
	// statusEquivalent: the provider does not send the operation, but a covered
	// operation does what it does for the Terraform surface.
	statusEquivalent = "equivalent"
	// statusInline: the provider manages the operation's object as an attribute of
	// another resource, through that resource's own operations.
	statusInline = "inline"
	// statusPlanned: a ticket plans a Terraform surface for the operation.
	statusPlanned = "planned"
	// statusExcluded: the provider will not manage the operation's object.
	statusExcluded = "excluded"
)

var statuses = []string{
	statusCovered, statusEquivalent, statusInline, statusPlanned, statusExcluded,
}

// The reasons the check compares with the spec.
const (
	reasonNotGA      = "not-ga"
	reasonDeprecated = "deprecated"
)

// reasons is the vocabulary of triage reasons, with the status each one explains.
// The header of coverage/operations.yaml says what each one means.
var reasons = map[string]string{
	"list-variant":   statusEquivalent,
	"bulk-variant":   statusEquivalent,
	"alternate-form": statusEquivalent,
	"new-resource":   statusPlanned,
	reasonNotGA:      statusExcluded,
	reasonDeprecated: statusExcluded,
	"account-admin":  statusExcluded,
	"runtime-data":   statusExcluded,
	"read-only":      statusExcluded,
	"imperative":     statusExcluded,
	"pdp-runtime":    statusExcluded,
	"not-planned":    statusExcluded,
}

// ticketID is a Linear issue ID. The triage file names tickets by ID only.
var ticketID = regexp.MustCompile(`^[A-Z]{2,10}-[0-9]+$`)

// triageFile is coverage/operations.yaml.
type triageFile struct {
	// Operations maps each spec operation ID to its triage.
	Operations map[string]triageEntry `yaml:"operations"`
	// KnownDecodeFailures are the decode failures the check allows.
	KnownDecodeFailures []knownFailure `yaml:"known_decode_failures"`
}

type triageEntry struct {
	Status string `yaml:"status"`
	// Surface names the Terraform resources and data sources the operation is
	// for: permitio_x for a resource, data.permitio_x for a data source, and
	// either followed by .attribute for one attribute.
	Surface []string `yaml:"surface"`
	// Calls are the provider calls that send a covered operation.
	Calls  []string `yaml:"calls"`
	Reason string   `yaml:"reason"`
	Ticket string   `yaml:"ticket"`
}

// knownFailure allows the decode failures of one fixture variant, named by the
// label the report gives it, such as AttributeType=object.
type knownFailure struct {
	Variant string `yaml:"variant"`
	Note    string `yaml:"note"`
}

// parseTriage parses the triage file. Unknown keys, a key given twice and a file
// with no operations are errors.
func parseTriage(data []byte) (triageFile, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var file triageFile
	if err := decoder.Decode(&file); err != nil {
		if errors.Is(err, io.EOF) {
			return file, errors.New("the triage file is empty")
		}
		return file, fmt.Errorf("reading the triage file: %w", err)
	}
	if len(file.Operations) == 0 {
		return file, errors.New("the triage file has no operations")
	}
	return file, nil
}

// validate returns the problems of one entry that do not depend on the provider
// or the spec: its status, reason, ticket and which fields its status needs.
func (e triageEntry) validate() []string {
	var problems []string
	if !slices.Contains(statuses, e.Status) {
		return []string{fmt.Sprintf("status %q is not one of %s", e.Status,
			strings.Join(statuses, ", "))}
	}
	needsSurface := e.Status == statusCovered || e.Status == statusEquivalent ||
		e.Status == statusInline
	if needsSurface && len(e.Surface) == 0 {
		problems = append(problems, fmt.Sprintf("a %s operation needs a surface", e.Status))
	}
	if e.Status == statusInline {
		for _, surface := range e.Surface {
			if _, attribute := splitSurface(surface); attribute == "" {
				problems = append(problems, fmt.Sprintf("an inline operation's surface names an "+
					"attribute, as in permitio_resource.actions, not %q", surface))
			}
		}
	}
	if (e.Status == statusCovered) != (len(e.Calls) > 0) {
		problems = append(problems, "calls are listed for covered operations, and only for them")
	}
	switch e.Status {
	case statusCovered, statusInline:
		if e.Reason != "" {
			problems = append(problems, fmt.Sprintf("a %s operation takes no reason", e.Status))
		}
	default:
		explains, known := reasons[e.Reason]
		switch {
		case !known:
			problems = append(problems, fmt.Sprintf("reason %q is not in the vocabulary", e.Reason))
		case explains != e.Status:
			problems = append(problems, fmt.Sprintf("reason %s does not explain a %s operation",
				e.Reason, e.Status))
		}
	}
	if e.Status == statusPlanned && e.Ticket == "" {
		problems = append(problems, "a planned operation needs a ticket")
	}
	if e.Ticket != "" && !ticketID.MatchString(e.Ticket) {
		problems = append(problems, fmt.Sprintf("ticket %q is not a ticket ID such as PER-123",
			e.Ticket))
	}
	return problems
}

// splitSurface splits a surface into the Terraform type it names, "data.permitio_x"
// for a data source, and the attribute after it, if any.
func splitSurface(surface string) (string, string) {
	prefix := ""
	rest := surface
	if after, ok := strings.CutPrefix(surface, "data."); ok {
		prefix = "data."
		rest = after
	}
	typeName, attribute, _ := strings.Cut(rest, ".")
	return prefix + typeName, attribute
}
