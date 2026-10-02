//go:build unix

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scratchModule writes a module with one package, greet, whose Greeting has body,
// and a test that asserts what Greeting returns.
func scratchModule(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":         "module example.com/m\n\ngo 1.21\n",
		"greet/greet.go": "package greet\n\nfunc Greeting(name string) string { " + body + " }\n",
		"greet/greet_test.go": "package greet\n\nimport \"testing\"\n\n" +
			"func TestGreeting(t *testing.T) {\n" +
			"\tif got := Greeting(\"x\"); got != \"hello, x!\" {\n\t\tt.Fatal(got)\n\t}\n}\n",
	}
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runShim runs the go shim the gate writes, with this test binary as mutationgate,
// in dir with args, and returns its exit code, its output and the event log.
func runShim(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	realGo, err := exec.LookPath("go")
	if err == nil {
		realGo, err = filepath.Abs(realGo)
	}
	if err != nil {
		t.Fatalf("the go shim tests need go on PATH: %v", err)
	}
	self := testExecutable(t)
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "events.log")
	for what, p := range map[string]string{"test binary": self, "go": realGo, "log": logPath} {
		if err := checkPlainPath(what, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeGoShim(bin, self, realGo, logPath); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(bin, "go"), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), testMainEnv+"=main", "GOFLAGS=", "GOTOOLCHAIN=local",
		"GOWORK=off", "GOPROXY=off")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatal(err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return cmd.ProcessState.ExitCode(), out.String(), string(log)
}

var mutantRunArgs = []string{"test", "-timeout", "1m", "-failfast", "example.com/m/greet"}

func TestGoShimTestsAMutantThatCompiles(t *testing.T) {
	dir := scratchModule(t, `return "hello, " + name + "!"`)
	exit, out, log := runShim(t, dir, mutantRunArgs...)
	if exit != 0 || !strings.Contains(out, "ok  \texample.com/m/greet") || log != "" {
		t.Errorf("exit %d, log %q, output:\n%s", exit, log, out)
	}
	dir = scratchModule(t, `return "hello, " + name + "?"`)
	exit, out, log = runShim(t, dir, mutantRunArgs...)
	if exit != 1 || !strings.Contains(out, "FAIL\texample.com/m/greet") || log != "" {
		t.Errorf("a mutant the test detects: exit %d, log %q, output:\n%s", exit, log, out)
	}
}

// go test exits 1 when a mutant does not compile, which gremlins would count as
// killed; the shim must exit 2, which gremlins counts as not viable.
func TestGoShimReportsAMutantThatDoesNotCompile(t *testing.T) {
	dir := scratchModule(t, `return "hello, " - name + "!"`)
	exit, out, log := runShim(t, dir, mutantRunArgs...)
	if exit != exitNotViable || !strings.Contains(out, "does not compile with this mutant") ||
		log != "" {
		t.Errorf("exit %d, log %q, output:\n%s", exit, log, out)
	}
}

func TestGoShimFailsTheGateWhenThePackageCannotLoad(t *testing.T) {
	dir := scratchModule(t, `return "hello, " + name + "!"`)
	args := []string{"test", "-timeout", "1m", "-failfast", "example.com/m/missing"}
	exit, out, log := runShim(t, dir, args...)
	want := "error\tgo\tgo build example.com/m/missing failed, and go list cannot load the package"
	if exit != exitFailed || !strings.HasPrefix(log, want) {
		t.Errorf("exit %d, log %q, output:\n%s", exit, log, out)
	}
}

func TestGoShimRunsOtherCommandsAsIs(t *testing.T) {
	dir := scratchModule(t, `return "hello, " - name + "!"`)
	exit, out, log := runShim(t, dir, "env", "GOOS")
	if exit != 0 || strings.TrimSpace(out) != runtime.GOOS || log != "" {
		t.Errorf("exit %d, log %q, output:\n%s", exit, log, out)
	}
	exit, out, _ = runShim(t, dir, "test", "-count=1", "./...")
	if exit != 1 || !strings.Contains(out, "[build failed]") {
		t.Errorf("a go test that is not a mutant run: exit %d, output:\n%s", exit, out)
	}
}
