package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/terraform-provider-permit-io/internal/acctest/providerschema"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
)

// examplesDir holds the examples that the docs embed, which tfplugindocs finds by
// path: provider/provider.tf, resources/<name>/resource.tf and
// data-sources/<name>/data-source.tf, and standalone configurations such as
// basic-example/.
const examplesDir = "examples"

// devOverridesWarning is the summary of the warning that Terraform and OpenTofu
// report whenever a dev_overrides CLI configuration is in effect.
const devOverridesWarning = "Provider development overrides are in effect"

// minExampleDirs is the fewest example directories examples/ may hold: the
// provider's, one for each of the 14 resources and 5 data sources, basic-example
// and rebac. Raise it when you add a directory, so that a deleted one fails the
// test.
const minExampleDirs = 22

// TestExamples builds the provider, points Terraform at the build with a
// dev_overrides CLI configuration, and runs validate in every example directory,
// so that each example the docs show is valid with this build and draws no
// warning, such as the one for a deprecated quoted reference. It also fails when
// the provider, a resource or a data source has no example for its doc page. Like
// TestConsumerFixtures, it needs no terraform init or Permit API.
func TestExamples(t *testing.T) {
	cli := consumerCLI(t)
	dirs := exampleDirs(t)
	checkExamplesCoverProvider(t)
	cliConfig := devOverridesConfig(t, buildProvider(t))

	for _, dir := range dirs {
		t.Run(filepath.ToSlash(dir), func(t *testing.T) {
			t.Parallel()
			out := validate(t, cli, cliConfig, dir)
			if errs := out.reportedErrors(); len(errs) > 0 {
				t.Errorf("%s must validate, but reports:\n\t%s", dir, strings.Join(errs, "\n\t"))
			}
			var warnings []string
			for _, warning := range out.reported("warning") {
				if !strings.Contains(warning, devOverridesWarning) {
					warnings = append(warnings, warning)
				}
			}
			if len(warnings) > 0 {
				t.Errorf("%s must validate without warnings, but reports:\n\t%s", dir,
					strings.Join(warnings, "\n\t"))
			}
		})
	}
}

// exampleDirs returns every directory under examples/ that holds a .tf file, and
// reads each .tf file so that go test's cache notices when one changes. It fails
// the test when there are fewer than minExampleDirs.
func exampleDirs(t *testing.T) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(examplesDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".tf" {
			return nil
		}
		if _, err := os.ReadFile(path); err != nil {
			return err
		}
		if dir := filepath.Dir(path); !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the examples: %v", err)
	}
	if len(dirs) < minExampleDirs {
		t.Fatalf("%s holds %d directories with a .tf file, want at least %d; an example "+
			"was deleted or not found", examplesDir, len(dirs), minExampleDirs)
	}
	return dirs
}

// checkExamplesCoverProvider fails the test unless examples/ holds the example
// that tfplugindocs embeds in each doc page, and each resource or data source
// example declares the resource type or data source it documents.
func checkExamplesCoverProvider(t *testing.T) {
	t.Helper()
	snapshot, err := providerschema.Build(t.Context(), provider.New("test")())
	if err != nil {
		t.Fatalf("reading the provider's resource types and data sources: %v", err)
	}
	if _, err := os.Stat(filepath.Join(examplesDir, "provider", "provider.tf")); err != nil {
		t.Errorf("the provider has no example for the docs index: %v", err)
	}
	for name := range snapshot.Resources {
		checkExampleDeclares(t, filepath.Join(examplesDir, "resources", name, "resource.tf"),
			"resource", name)
	}
	for name := range snapshot.DataSources {
		checkExampleDeclares(t,
			filepath.Join(examplesDir, "data-sources", name, "data-source.tf"), "data", name)
	}
	if t.Failed() {
		t.FailNow()
	}
}

// checkExampleDeclares fails the test unless the example at path declares a
// block of kind ("resource" or "data") and type name.
func checkExampleDeclares(t *testing.T, path, kind, name string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("the %s %s has no example for its doc page: %v", kind, name, err)
		return
	}
	for _, match := range blockHeader.FindAllStringSubmatch(string(content), -1) {
		if match[1] == kind && match[2] == name {
			return
		}
	}
	t.Errorf("%s declares no %s %q block", path, kind, name)
}
