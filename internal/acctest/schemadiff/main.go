// Command schemadiff compares two provider schema snapshots, such as a base and a
// head copy of internal/provider/testdata/provider-schema.json, and prints each
// change labelled BREAKING or non-breaking, then a count. The providerschema
// package's Diff says which changes are breaking.
//
// Usage:
//
//	schemadiff BASE.json HEAD.json
//
// Exit codes:
//
//	0  no breaking change (there may be non-breaking ones)
//	1  at least one breaking change
//	2  did not run: wrong arguments, or a file that cannot be read, is empty, is
//	   not a snapshot, or has no resources and no data sources
//
// Build it before use; go run reports every non-zero exit as 1:
//
//	go build -o schemadiff ./internal/acctest/schemadiff
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/permitio/terraform-provider-permit-io/internal/acctest/providerschema"
)

const (
	exitNoBreaking = 0
	exitBreaking   = 1
	exitDidNotRun  = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(args []string, output io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(output, "schemadiff: DID NOT RUN: want two arguments, "+
			"usage: schemadiff BASE.json HEAD.json")
		return exitDidNotRun
	}
	base, err := load(args[0])
	if err != nil {
		fmt.Fprintf(output, "schemadiff: DID NOT RUN: base: %v\n", err)
		return exitDidNotRun
	}
	head, err := load(args[1])
	if err != nil {
		fmt.Fprintf(output, "schemadiff: DID NOT RUN: head: %v\n", err)
		return exitDidNotRun
	}

	breaking := 0
	changes := providerschema.Diff(base, head)
	for _, change := range changes {
		fmt.Fprintln(output, change)
		if change.Breaking {
			breaking++
		}
	}
	fmt.Fprintf(output, "schemadiff: %d changes, %d breaking\n", len(changes), breaking)
	if breaking > 0 {
		return exitBreaking
	}
	return exitNoBreaking
}

func load(path string) (providerschema.Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return providerschema.Snapshot{}, err
	}
	snapshot, err := providerschema.Decode(data)
	if err != nil {
		return providerschema.Snapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	return snapshot, nil
}
