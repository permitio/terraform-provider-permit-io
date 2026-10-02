package provider

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/terraform-provider-permit-io/internal/acctest/providerschema"
)

// TestSchemaDescriptions fails for every part of the provider schema that has an
// empty description: the provider, each resource and data source, and each of
// their attributes and blocks at any nesting depth. The docs show these
// descriptions, so an undescribed part is undocumented. Each is named by its
// Terraform address: "provider.api_key", "permitio_role.key" for a resource and
// "data.permitio_role.key" for a data source, with the dotted path of a nested
// attribute.
func TestSchemaDescriptions(t *testing.T) {
	snapshot, err := providerschema.Build(t.Context(), New("test")())
	if err != nil {
		t.Fatalf("building the provider schema snapshot: %v", err)
	}
	if len(snapshot.Resources) != wantResourceCount ||
		len(snapshot.DataSources) != wantDataSourceCount {
		t.Fatalf("DID NOT RUN: the provider serves %d resources and %d data sources, "+
			"want %d and %d", len(snapshot.Resources), len(snapshot.DataSources),
			wantResourceCount, wantDataSourceCount)
	}

	schemas := map[string]providerschema.Schema{"provider": snapshot.Provider}
	for name, schema := range snapshot.Resources {
		schemas[name] = schema
	}
	for name, schema := range snapshot.DataSources {
		schemas["data."+name] = schema
	}

	checked := 0
	for _, address := range slices.Sorted(maps.Keys(schemas)) {
		schema := schemas[address]
		if len(schema.Attributes) == 0 {
			t.Errorf("DID NOT RUN: %s has no attributes", address)
		}
		checked++
		if strings.TrimSpace(schema.Description) == "" {
			t.Errorf("%s has no description; give it one", address)
		}
		for _, attributePath := range slices.Sorted(maps.Keys(schema.Attributes)) {
			checked++
			if strings.TrimSpace(schema.Attributes[attributePath].Description) == "" {
				t.Errorf("%s.%s has no description; give it one", address, attributePath)
			}
		}
		for _, blockPath := range slices.Sorted(maps.Keys(schema.Blocks)) {
			checked++
			if strings.TrimSpace(schema.Blocks[blockPath].Description) == "" {
				t.Errorf("block %s.%s has no description; give it one", address, blockPath)
			}
		}
	}
	// The provider, 14 resources and 5 data sources each have at least one attribute.
	if minChecked := 2 * (1 + wantResourceCount + wantDataSourceCount); checked < minChecked {
		t.Fatalf("DID NOT RUN: %d descriptions checked, want at least %d", checked, minChecked)
	}
}
