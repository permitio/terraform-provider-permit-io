package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// result is what the fake govulncheck returns for one target.
type result struct {
	stdout []byte
	stderr string
	code   int
	err    error
}

// fakeScanner stands in for go tool govulncheck. It answers by the scanned target,
// the last argument, and records every command line it gets.
type fakeScanner struct {
	results map[string]result
	calls   [][]string
}

func (f *fakeScanner) scan(args []string) ([]byte, []byte, int, error) {
	f.calls = append(f.calls, args)
	r, ok := f.results[args[len(args)-1]]
	if !ok {
		return nil, []byte("no fake result for this target"), 1, nil
	}
	return r.stdout, []byte(r.stderr), r.code, r.err
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

// rewrite decodes a govulncheck JSON stream and encodes it again with each message
// passed through change, which edits the message in place or returns false to
// drop it.
func rewrite(t *testing.T, data []byte, change func(message map[string]any) bool) []byte {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	for {
		var message map[string]any
		err := decoder.Decode(&message)
		if errors.Is(err, io.EOF) {
			return out.Bytes()
		}
		if err != nil {
			t.Fatalf("decode fixture: %v", err)
		}
		if !change(message) {
			continue
		}
		if err := encoder.Encode(message); err != nil {
			t.Fatalf("encode fixture: %v", err)
		}
	}
}

// withModules returns the fixture with its module list cut to its first n modules.
func withModules(t *testing.T, name string, n int) []byte {
	t.Helper()
	return rewrite(t, fixture(t, name), func(message map[string]any) bool {
		sbom, ok := message["SBOM"].(map[string]any)
		if !ok {
			return true
		}
		modules, _ := sbom["modules"].([]any)
		if len(modules) < n {
			t.Fatalf("%s has %d modules, want at least %d", name, len(modules), n)
		}
		sbom["modules"] = modules[:n]
		return true
	})
}

// acceptFile writes an accept file with the given lines and returns its path.
func acceptFile(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "accept.txt")
	content := "# accepted vulnerabilities\n\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write accept file: %v", err)
	}
	return path
}

// binaries creates n empty files to scan and returns their paths.
func binaries(t *testing.T, n int) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for i := range n {
		path := filepath.Join(dir, "terraform-provider-permit-io_"+string(rune('a'+i)))
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("write binary: %v", err)
		}
		paths = append(paths, path)
	}
	return paths
}

func runWith(t *testing.T, scanner *fakeScanner, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr, scanner.scan)
	out := stdout.String() + stderr.String()
	t.Logf("exit %d, output:\n%s", code, out)
	return code, out
}

func assertContains(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output does not contain %q", w)
		}
	}
}

func TestSourceScanWithoutFindingsPasses(t *testing.T) {
	scanner := &fakeScanner{results: map[string]result{
		"./...": {stdout: fixture(t, "source-clean.json")},
	}}
	code, out := runWith(t, scanner, "-mode", "source", "./...")
	if code != exitClean {
		t.Fatalf("exit %d, want %d", code, exitClean)
	}
	want := []string{"-mode", "source", "-format", "json", "./..."}
	if len(scanner.calls) != 1 || !slices.Equal(scanner.calls[0], want) {
		t.Errorf("govulncheck calls %q, want one call with %q", scanner.calls, want)
	}
	assertContains(t, out, "govulncheck v1.8.0", "59 modules", "PASS")
}

func TestUnacceptedUncalledVulnerabilityFails(t *testing.T) {
	scanner := &fakeScanner{results: map[string]result{
		"./...": {stdout: fixture(t, "source-uncalled.json")},
	}}
	code, out := runWith(t, scanner, "-mode", "source", "./...")
	if code != exitFindings {
		t.Fatalf("exit %d, want %d", code, exitFindings)
	}
	assertContains(t, out, "GO-2026-5932", "not accepted", "golang.org/x/crypto v0.57.0", "FAIL")
}

func TestAcceptedVulnerabilityIsPrintedWithItsReason(t *testing.T) {
	scanner := &fakeScanner{results: map[string]result{
		"./...": {stdout: fixture(t, "source-uncalled.json")},
	}}
	accept := acceptFile(t, "GO-2026-5932 source   openpgp is not imported by any package")
	code, out := runWith(t, scanner, "-mode", "source", "-accept", accept, "./...")
	if code != exitClean {
		t.Fatalf("exit %d, want %d", code, exitClean)
	}
	assertContains(t, out, "GO-2026-5932", "accepted: openpgp is not imported by any package",
		"x/crypto/openpgp package is unmaintained", "1 accepted", "PASS")
}

func TestStaleAcceptEntryFails(t *testing.T) {
	scanner := &fakeScanner{results: map[string]result{
		"./...": {stdout: fixture(t, "source-clean.json")},
	}}
	accept := acceptFile(t, "GO-2026-5932 source openpgp is not imported by any package")
	code, out := runWith(t, scanner, "-mode", "source", "-accept", accept, "./...")
	if code != exitFindings {
		t.Fatalf("exit %d, want %d", code, exitFindings)
	}
	assertContains(t, out, "GO-2026-5932", "stale", "FAIL")
}

func TestAcceptEntryIsNotCalledStaleWhenAScanDidNotComplete(t *testing.T) {
	scanner := &fakeScanner{results: map[string]result{
		"./...": {stdout: fixture(t, "source-clean.json"), stderr: "loading packages: boom", code: 1},
	}}
	accept := acceptFile(t, "GO-2026-5932 source openpgp is not imported by any package")
	code, out := runWith(t, scanner, "-mode", "source", "-accept", accept, "./...")
	if code != exitDidNotRun {
		t.Fatalf("exit %d, want %d", code, exitDidNotRun)
	}
	assertContains(t, out, "accept entries not checked", "0 stale accept entries", "DID NOT COMPLETE")
	if strings.Contains(out, "stale accept entry:") {
		t.Errorf("output calls an accept entry stale after a scan that did not complete")
	}
}

func TestAcceptEntryOfTheOtherModeIsNotStale(t *testing.T) {
	bins := binaries(t, 1)
	scanner := &fakeScanner{results: map[string]result{
		bins[0]: {stdout: fixture(t, "binary-clean.json")},
	}}
	accept := acceptFile(t, "GO-2026-5932 source openpgp is not imported by any package")
	code, _ := runWith(t, scanner, "-mode", "binary", "-binaries", "1", "-accept", accept, bins[0])
	if code != exitClean {
		t.Fatalf("exit %d, want %d", code, exitClean)
	}
}

func TestCalledVulnerabilitiesFailInEveryBinary(t *testing.T) {
	bins := binaries(t, 2)
	scanner := &fakeScanner{results: map[string]result{
		bins[0]: {stdout: fixture(t, "binary-clean.json")},
		bins[1]: {stdout: fixture(t, "binary-called.json")},
	}}
	args := append([]string{"-mode", "binary", "-binaries", "2"}, bins...)
	code, out := runWith(t, scanner, args...)
	if code != exitFindings {
		t.Fatalf("exit %d, want %d", code, exitFindings)
	}
	if len(scanner.calls) != 2 {
		t.Fatalf("govulncheck ran %d times, want once per binary", len(scanner.calls))
	}
	for i, call := range scanner.calls {
		want := []string{"-mode", "binary", "-format", "json", bins[i]}
		if !slices.Equal(call, want) {
			t.Errorf("govulncheck call %q, want %q", call, want)
		}
	}
	assertContains(t, out, "GO-2025-3488", "GO-2024-2611", "called", "jws.Verify",
		"fixed in v0.27.0", filepath.Base(bins[1]), "2 to fix", "FAIL")
}

func TestCalledVulnerabilityCannotBeAccepted(t *testing.T) {
	bins := binaries(t, 1)
	scanner := &fakeScanner{results: map[string]result{
		bins[0]: {stdout: fixture(t, "binary-called.json")},
	}}
	accept := acceptFile(t,
		"GO-2025-3488 binary a reason that does not cover called code",
		"GO-2024-2611 binary a reason that does not cover called code")
	args := []string{"-mode", "binary", "-binaries", "1", "-accept", accept, bins[0]}
	code, out := runWith(t, scanner, args...)
	if code != exitFindings {
		t.Fatalf("exit %d, want %d", code, exitFindings)
	}
	assertContains(t, out, "called code cannot be accepted", "in: all 1 binaries")
}

func TestGovulncheckExit3CountsAsFindings(t *testing.T) {
	scanner := &fakeScanner{results: map[string]result{
		"./...": {stdout: fixture(t, "source-clean.json"), code: 3},
	}}
	code, out := runWith(t, scanner, "-mode", "source", "./...")
	if code != exitFindings {
		t.Fatalf("exit %d, want %d", code, exitFindings)
	}
	assertContains(t, out, "exit 3")
}

// The workflows run vulnscan without -min-modules, so the defaults are the module
// minimums CI enforces.
func TestDefaultModuleMinimum(t *testing.T) {
	bins := binaries(t, 1)
	tests := map[string]struct {
		mode    string
		target  string
		fixture string
		modules int
		want    int
		output  string
	}{
		"source at the minimum": {
			mode: modeSource, target: "./...", fixture: "source-clean.json",
			modules: 50, want: exitClean, output: "50 modules (at least 50 expected)",
		},
		"source below the minimum": {
			mode: modeSource, target: "./...", fixture: "source-clean.json",
			modules: 49, want: exitDidNotRun, output: "49 modules, fewer than the 50 expected",
		},
		"binary at the minimum": {
			mode: modeBinary, target: bins[0], fixture: "binary-clean.json",
			modules: 25, want: exitClean, output: "25 modules (at least 25 expected)",
		},
		"binary below the minimum": {
			mode: modeBinary, target: bins[0], fixture: "binary-clean.json",
			modules: 24, want: exitDidNotRun, output: "24 modules, fewer than the 25 expected",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			scanner := &fakeScanner{results: map[string]result{
				tt.target: {stdout: withModules(t, tt.fixture, tt.modules)},
			}}
			args := []string{"-mode", tt.mode}
			if tt.mode == modeBinary {
				args = append(args, "-binaries", "1")
			}
			code, out := runWith(t, scanner, append(args, tt.target)...)
			if code != tt.want {
				t.Fatalf("exit %d, want %d", code, tt.want)
			}
			assertContains(t, out, tt.output)
		})
	}
}

func TestScanThatDidNotCompleteExits2(t *testing.T) {
	clean := fixture(t, "source-clean.json")
	configAndSBOM := clean[:bytes.Index(clean, []byte(`"osv"`))]
	configAndSBOM = configAndSBOM[:bytes.LastIndexByte(configAndSBOM, '{')]
	withoutSBOM := rewrite(t, clean, func(message map[string]any) bool {
		_, isSBOM := message["SBOM"]
		return !isSBOM
	})
	moduleLevel := rewrite(t, clean, func(message map[string]any) bool {
		if config, ok := message["config"].(map[string]any); ok {
			config["scan_level"] = "module"
		}
		return true
	})
	findingWithout := func(field string) []byte {
		return rewrite(t, fixture(t, "source-uncalled.json"), func(message map[string]any) bool {
			if finding, ok := message["finding"].(map[string]any); ok {
				delete(finding, field)
			}
			return true
		})
	}
	tests := map[string]struct {
		result result
		args   []string
		want   string
	}{
		"govulncheck fails": {
			result: result{stdout: clean, stderr: "govulncheck: loading packages: boom", code: 1},
			want:   "loading packages: boom",
		},
		"govulncheck killed": {result: result{stdout: clean, code: -1}, want: "exit -1"},
		"govulncheck not started": {
			result: result{err: errors.New(`exec: "go": executable file not found`)},
			want:   "executable file not found",
		},
		"empty output":     {result: result{}, want: "no config"},
		"truncated output": {result: result{stdout: clean[:len(clean)/2]}, want: "unexpected EOF"},
		"output cut after the module list": {
			result: result{stdout: configAndSBOM},
			want:   "no vulnerability database entries",
		},
		"not govulncheck output": {
			result: result{stdout: []byte("No vulnerabilities found.\n")},
			want:   "invalid",
		},
		"too few modules": {
			result: result{stdout: clean},
			args:   []string{"-min-modules", "60"},
			want:   "59 modules, fewer than the 60 expected",
		},
		"no module list":     {result: result{stdout: withoutSBOM}, want: "no module list"},
		"module level scan":  {result: result{stdout: moduleLevel}, want: `scan level "module"`},
		"finding without ID": {result: result{stdout: findingWithout("osv")}, want: "without an ID"},
		"finding without a trace": {
			result: result{stdout: findingWithout("trace")},
			want:   "without an ID or a trace",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			scanner := &fakeScanner{results: map[string]result{"./...": tt.result}}
			args := append([]string{"-mode", "source"}, tt.args...)
			code, out := runWith(t, scanner, append(args, "./...")...)
			if code != exitDidNotRun {
				t.Fatalf("exit %d, want %d", code, exitDidNotRun)
			}
			assertContains(t, out, tt.want, "DID NOT COMPLETE")
		})
	}
}

func TestBinaryScanOfTheWrongModeExits2(t *testing.T) {
	bins := binaries(t, 1)
	scanner := &fakeScanner{results: map[string]result{
		bins[0]: {stdout: fixture(t, "source-clean.json")},
	}}
	code, out := runWith(t, scanner, "-mode", "binary", "-binaries", "1", bins[0])
	if code != exitDidNotRun {
		t.Fatalf("exit %d, want %d", code, exitDidNotRun)
	}
	assertContains(t, out, `scan mode "source", want "binary"`)
}

func TestIncompleteBinaryScanOutranksFindings(t *testing.T) {
	bins := binaries(t, 2)
	scanner := &fakeScanner{results: map[string]result{
		bins[0]: {stdout: fixture(t, "binary-called.json")},
		bins[1]: {code: 1, stderr: "not a Go binary"},
	}}
	args := append([]string{"-mode", "binary", "-binaries", "2"}, bins...)
	code, out := runWith(t, scanner, args...)
	if code != exitDidNotRun {
		t.Fatalf("exit %d, want %d", code, exitDidNotRun)
	}
	assertContains(t, out, "GO-2025-3488", "not a Go binary")
}

func TestBadInputExits2WithoutScanning(t *testing.T) {
	bins := binaries(t, 2)
	missing := filepath.Join(t.TempDir(), "missing")
	tests := map[string]struct {
		args []string
		want string
	}{
		"no mode":             {args: []string{"./..."}, want: "-mode"},
		"unknown mode":        {args: []string{"-mode", "module", "./..."}, want: "-mode"},
		"source without args": {args: []string{"-mode", "source"}, want: "package pattern"},
		"source with -binaries": {
			args: []string{"-mode", "source", "-binaries", "2", "./..."},
			want: "-binaries",
		},
		"binary without -binaries": {
			args: append([]string{"-mode", "binary"}, bins...),
			want: "-binaries",
		},
		"fewer binaries than expected": {
			args: append([]string{"-mode", "binary", "-binaries", "3"}, bins...),
			want: "2 binaries, want 3",
		},
		"missing binary": {
			args: []string{"-mode", "binary", "-binaries", "2", bins[0], missing},
			want: "missing",
		},
		"binary is a directory": {
			args: []string{"-mode", "binary", "-binaries", "2", bins[0], t.TempDir()},
			want: "not a regular file",
		},
		"negative module minimum": {
			args: []string{"-mode", "source", "-min-modules", "-1", "./..."},
			want: "-min-modules",
		},
		"missing accept file": {
			args: []string{"-mode", "source", "-accept", missing, "./..."},
			want: "accept file",
		},
		"accept entry without a reason": {
			args: []string{"-mode", "source", "-accept",
				acceptFile(t, "GO-2026-5932 source"), "./..."},
			want: "line 3",
		},
		"accept entry with a bad ID": {
			args: []string{"-mode", "source", "-accept",
				acceptFile(t, "CVE-2026-1 source why"), "./..."},
			want: "not a Go vulnerability ID",
		},
		"accept entry with a bad mode": {
			args: []string{"-mode", "source", "-accept",
				acceptFile(t, "GO-2026-5932 module why"), "./..."},
			want: "mode",
		},
		"duplicate accept entry": {
			args: []string{"-mode", "source", "-accept",
				acceptFile(t, "GO-2026-5932 source why", "GO-2026-5932 source why again"), "./..."},
			want: "listed twice",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			scanner := &fakeScanner{}
			code, out := runWith(t, scanner, tt.args...)
			if code != exitDidNotRun {
				t.Fatalf("exit %d, want %d", code, exitDidNotRun)
			}
			if len(scanner.calls) != 0 {
				t.Errorf("govulncheck ran %d times, want none", len(scanner.calls))
			}
			assertContains(t, out, tt.want)
		})
	}
}
