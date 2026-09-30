package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/permitio/terraform-provider-permit-io/internal/acctest/providerschema"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
)

// Consumer fixtures: configurations written as a user would write them, checked
// with the validate command of the Terraform or OpenTofu the offline tests use.
// valid/*.tf is one configuration that must validate and must use every resource
// type and data source the provider serves. Each invalid/<case>/ directory holds a
// main.tf that must fail validation and an expect.txt with the text that every
// error it reports must contain.
const (
	consumerValidDir   = "testdata/consumer/valid"
	consumerInvalidDir = "testdata/consumer/invalid"
)

// minInvalidFixtures is the fewest must-fail cases invalid/ may hold. Raise it
// when you add a case, so that a deleted case fails the test.
const minInvalidFixtures = 16

// blockHeader matches the header of a top-level resource or data block in a
// configuration formatted by terraform fmt.
var blockHeader = regexp.MustCompile(`(?m)^(resource|data)\s+"([^"]+)"`)

// TestConsumerFixtures builds the provider, points Terraform at the build with a
// dev_overrides CLI configuration, and runs validate in each fixture directory.
// Validate reads no state and needs no terraform init or Permit API, so this runs
// offline on every Terraform and OpenTofu leg.
func TestConsumerFixtures(t *testing.T) {
	cli := consumerCLI(t)
	checkValidFixturesCoverProvider(t)
	cases := invalidFixtures(t)
	cliConfig := devOverridesConfig(t, buildProvider(t))

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		out := validate(t, cli, cliConfig, consumerValidDir)
		if errs := out.reportedErrors(); len(errs) > 0 {
			t.Errorf("%s must validate, but reports:\n\t%s", consumerValidDir,
				strings.Join(errs, "\n\t"))
		}
	})
	for _, name := range cases {
		dir := filepath.Join(consumerInvalidDir, name)
		t.Run("invalid/"+name, func(t *testing.T) {
			t.Parallel()
			readInvalidConfig(t, dir)
			want := expectedError(t, dir)
			errs := validate(t, cli, cliConfig, dir).reportedErrors()
			if len(errs) == 0 {
				t.Fatalf("%s validates; it must fail with an error containing %q", dir, want)
			}
			for _, got := range errs {
				if !strings.Contains(got, want) {
					t.Errorf("%s reports an error without %q:\n\t%s", dir, want, got)
				}
			}
		})
	}
}

// consumerCLI returns the Terraform or OpenTofu executable to validate with:
// TF_ACC_TERRAFORM_PATH, which CI sets on every offline leg, or terraform on PATH.
func consumerCLI(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("TF_ACC_TERRAFORM_PATH"); path != "" {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("TF_ACC_TERRAFORM_PATH names no executable: %v", err)
		}
		return path
	}
	path, err := exec.LookPath("terraform")
	if err != nil {
		t.Fatalf("no Terraform to validate the consumer fixtures with (%v); set "+
			"TF_ACC_TERRAFORM_PATH to a terraform or tofu executable, or put terraform "+
			"on PATH", err)
	}
	return path
}

// checkValidFixturesCoverProvider fails the test unless the valid fixtures declare
// at least one of every resource type and data source the provider serves.
func checkValidFixturesCoverProvider(t *testing.T) {
	t.Helper()
	snapshot, err := providerschema.Build(t.Context(), provider.New("test")())
	if err != nil {
		t.Fatalf("reading the provider's resource types and data sources: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(consumerValidDir, "*.tf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("DID NOT RUN: no .tf files in %s", consumerValidDir)
	}
	declared := map[string]bool{}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		for _, match := range blockHeader.FindAllStringSubmatch(string(content), -1) {
			declared[match[1]+" "+match[2]] = true
		}
	}
	for name := range snapshot.Resources {
		if !declared["resource "+name] {
			t.Errorf("the valid fixtures declare no %s resource; add one to %s",
				name, consumerValidDir)
		}
	}
	for name := range snapshot.DataSources {
		if !declared["data "+name] {
			t.Errorf("the valid fixtures declare no %s data source; add one to %s",
				name, consumerValidDir)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

// invalidFixtures returns the must-fail case directories, sorted by os.ReadDir, and
// fails the test when there are fewer than minInvalidFixtures or when invalid/
// holds a file.
func invalidFixtures(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(consumerInvalidDir)
	if err != nil {
		t.Fatalf("reading the must-fail fixtures: %v", err)
	}
	var cases []string
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Errorf("%s holds one directory per case; move %s into one",
				consumerInvalidDir, entry.Name())
			continue
		}
		cases = append(cases, entry.Name())
	}
	if len(cases) < minInvalidFixtures {
		t.Errorf("%s holds %d cases, want at least %d; a case was deleted or not found",
			consumerInvalidDir, len(cases), minInvalidFixtures)
	}
	if t.Failed() {
		t.FailNow()
	}
	return cases
}

// readInvalidConfig fails the test unless dir holds a main.tf. Only the validate
// child process reads the configuration, so reading it here also makes go test's
// cache notice when it changes.
func readInvalidConfig(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "main.tf")
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("reading the must-fail configuration: %v; write it to %s", err, path)
	}
}

// expectedError returns the text in dir/expect.txt, which every error that
// validating dir reports must contain.
func expectedError(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "expect.txt")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the expected error: %v; write the text the error must "+
			"contain in %s", err, path)
	}
	want := strings.TrimSpace(string(content))
	if want == "" {
		t.Fatalf("%s is empty; write the text the error must contain", path)
	}
	return want
}

// buildProvider builds the provider into a temporary directory under the name
// dev_overrides looks for, and returns the directory.
func buildProvider(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	name := "terraform-provider-permit-io"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-o", filepath.Join(dir, name), ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the provider: %v\n%s", err, out)
	}
	return dir
}

// devOverridesConfig writes a CLI configuration that serves the provider from dir
// and returns its path. Terraform reads the fixtures' source "permitio/permit-io"
// as registry.terraform.io and OpenTofu as registry.opentofu.org, so both are
// overridden.
func devOverridesConfig(t *testing.T, dir string) string {
	t.Helper()
	dir = filepath.ToSlash(dir)
	config := fmt.Sprintf(`provider_installation {
  dev_overrides {
    "registry.terraform.io/permitio/permit-io" = %q
    "registry.opentofu.org/permitio/permit-io" = %q
  }
  direct {}
}
`, dir, dir)
	path := filepath.Join(t.TempDir(), "dev.tfrc")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("writing the CLI configuration: %v", err)
	}
	return path
}

// validateOutput is the part of `validate -json` output the test reads.
type validateOutput struct {
	FormatVersion string `json:"format_version"`
	Valid         bool   `json:"valid"`
	Diagnostics   []struct {
		Severity string `json:"severity"`
		Summary  string `json:"summary"`
		Detail   string `json:"detail"`
		Range    *struct {
			Filename string `json:"filename"`
			Start    struct {
				Line int `json:"line"`
			} `json:"start"`
		} `json:"range"`
	} `json:"diagnostics"`
}

// reportedErrors returns each error diagnostic as "file:line: summary: detail". The
// warning that development overrides are in effect is not an error.
func (o validateOutput) reportedErrors() []string {
	var errs []string
	for _, d := range o.Diagnostics {
		if d.Severity != "error" {
			continue
		}
		location := "(no location)"
		if d.Range != nil {
			location = fmt.Sprintf("%s:%d", d.Range.Filename, d.Range.Start.Line)
		}
		errs = append(errs, fmt.Sprintf("%s: %s: %s", location, d.Summary, d.Detail))
	}
	return errs
}

// validate runs `validate -json` in dir with the CLI configuration and returns its
// output. It fails the test when validate reports no result, or when its exit
// status and its result disagree.
func validate(t *testing.T, cli, cliConfig, dir string) validateOutput {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), cli, "validate", "-json")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TF_CLI_CONFIG_FILE="+cliConfig, "CHECKPOINT_DISABLE=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		t.Fatalf("running %s validate in %s: %v", cli, dir, runErr)
	}

	var out validateOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil || out.FormatVersion == "" {
		t.Fatalf("DID NOT RUN: %s validate in %s reported no result (%v)\nstdout:\n%s\n"+
			"stderr:\n%s", cli, dir, err, stdout.String(), stderr.String())
	}
	if out.Valid != (runErr == nil) || out.Valid != (len(out.reportedErrors()) == 0) {
		t.Fatalf("%s validate in %s reports valid=%t with %d errors, but exited with %v"+
			"\nstdout:\n%s\nstderr:\n%s", cli, dir, out.Valid, len(out.reportedErrors()), runErr,
			stdout.String(), stderr.String())
	}
	return out
}
