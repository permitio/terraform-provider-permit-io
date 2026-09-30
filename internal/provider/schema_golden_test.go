package provider

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/permitio/terraform-provider-permit-io/internal/acctest/providerschema"
)

const schemaGoldenPath = "testdata/provider-schema.json"

// The resources and data sources the provider serves. Change these counts together
// with provider.go and the golden file, so that regenerating the golden file cannot
// drop a resource or data source unnoticed.
const (
	wantResourceCount   = 14
	wantDataSourceCount = 5
)

var updateSchemaGolden = flag.Bool("update", false,
	"rewrite "+schemaGoldenPath+" from the provider schema")

// TestProviderSchemaGolden compares the provider schema, including each attribute's
// plan modifiers, validators and default, with testdata/provider-schema.json, so
// that every change to what users can configure shows up in review. After an
// intended change, regenerate the file and review its diff:
//
//	go test ./internal/provider/ -run TestProviderSchemaGolden -update
func TestProviderSchemaGolden(t *testing.T) {
	const regenerate = "go test ./internal/provider/ -run TestProviderSchemaGolden -update"
	snapshot, err := providerschema.Build(t.Context(), New("test")())
	if err != nil {
		t.Fatalf("building the provider schema snapshot: %v", err)
	}
	if len(snapshot.Resources) == 0 && len(snapshot.DataSources) == 0 {
		t.Fatal("DID NOT RUN: the provider schema has no resources and no data sources")
	}
	if len(snapshot.Resources) != wantResourceCount ||
		len(snapshot.DataSources) != wantDataSourceCount {
		t.Fatalf("the provider serves %d resources and %d data sources, want %d and %d; "+
			"after adding or removing one, update the counts in this test and run\n\t%s",
			len(snapshot.Resources), len(snapshot.DataSources), wantResourceCount,
			wantDataSourceCount, regenerate)
	}
	got, err := providerschema.Encode(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	if *updateSchemaGolden {
		if err := os.MkdirAll(filepath.Dir(schemaGoldenPath), 0o755); err != nil {
			t.Fatalf("creating the golden file's directory: %v", err)
		}
		if err := os.WriteFile(schemaGoldenPath, got, 0o644); err != nil {
			t.Fatalf("writing %s: %v", schemaGoldenPath, err)
		}
		t.Logf("wrote %s", schemaGoldenPath)
		return
	}

	want, err := os.ReadFile(schemaGoldenPath)
	if err != nil {
		t.Fatalf("reading the golden file: %v; create it with\n\t%s", err, regenerate)
	}
	if bytes.Equal(got, want) {
		return
	}
	golden, err := providerschema.Decode(want)
	if err != nil {
		t.Fatalf("%s is not a schema snapshot (%v); regenerate it with\n\t%s",
			schemaGoldenPath, err, regenerate)
	}
	var changes strings.Builder
	for _, change := range providerschema.Diff(golden, snapshot) {
		changes.WriteString("\n\t" + change.String())
	}
	if changes.Len() == 0 {
		changes.WriteString("\n\tnone: only the file's formatting differs")
	}
	t.Errorf("the provider schema differs from %s. Changes from the file to the provider:%s"+
		"\nIf the change is intended, regenerate the file and review its diff:\n\t%s",
		schemaGoldenPath, changes.String(), regenerate)
}
