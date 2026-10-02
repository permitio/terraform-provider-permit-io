package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// diffFixture is git diff -U0 output: an edit and an addition in a.go, a deletion,
// a test file, a deleted file, a non-Go file, a new file, and a removed line that
// reads like a file header.
const diffFixture = `diff --git a/a.go b/a.go
index 1111111..2222222 100644
--- a/a.go
+++ b/a.go
@@ -3 +3 @@ func a() {
-	x := 1
+	x := 2
@@ -10,0 +11,2 @@ func b() {
+	y := 3
+	z := 4
@@ -20,2 +21,0 @@ func c() {
--- looks like a header
-	gone
diff --git a/a_test.go b/a_test.go
--- a/a_test.go
+++ b/a_test.go
@@ -1 +1 @@
-x
+y
diff --git a/old.go b/old.go
deleted file mode 100644
--- a/old.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package x
-
diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
-a
+b
diff --git a/pkg/new.go b/pkg/new.go
new file mode 100644
--- /dev/null
+++ b/pkg/new.go
@@ -0,0 +1,3 @@
+package pkg
+
+func n() {}
\ No newline at end of file
`

func TestParseAddedLines(t *testing.T) {
	got, err := parseAddedLines([]byte(diffFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]lineRange{
		"a.go":       {{3, 3}, {11, 12}},
		"pkg/new.go": {{1, 3}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for file, ranges := range want {
		if !slices.Equal(got[file], ranges) {
			t.Errorf("%s: got %v, want %v", file, got[file], ranges)
		}
	}
}

func TestParseAddedLinesErrors(t *testing.T) {
	tests := map[string]string{
		"quoted file name": "--- a/x.go\n+++ \"b/sp\\tace.go\"\n",
		"bad hunk header":  "--- a/x.go\n+++ b/x.go\n@@ bad @@\n",
		"hunk cut short":   "--- a/x.go\n+++ b/x.go\n@@ -1 +1,2 @@\n-a\n+b\n",
		"hunk ends early":  "--- a/x.go\n+++ b/x.go\n@@ -1 +1,2 @@\n-a\n+b\ndiff --git a/y b/y\n",
	}
	for name, diff := range tests {
		t.Run(name, func(t *testing.T) {
			if got, err := parseAddedLines([]byte(diff)); err == nil {
				t.Fatalf("got %v and no error", got)
			}
		})
	}
}

func TestMisattributedDirs(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo")
	pkg := func(dir, name string) goPackage {
		abs := filepath.Join(root, filepath.FromSlash(dir))
		return goPackage{dir: abs, importPath: "m/" + dir, name: name}
	}
	pkgs := []goPackage{
		{dir: root, importPath: "m", name: "main"},
		pkg("cmd/tool", "main"),
		pkg("internal/x", "x"),
		pkg("internal/foo_bar", "foobar"),
		pkg("internal/provider", "provider"),
		pkg("internal/provider/sub", "provider"),
	}
	got, err := misattributedDirs("m", root, pkgs)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cmd/tool", "internal/foo_bar", "internal/provider/sub"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitExcluded(t *testing.T) {
	changed := map[string][]lineRange{
		"cmd/tool/main.go":     {{1, 1}},
		"cmd/tool/sub/x.go":    {{1, 1}},
		"internal/x/x.go":      {{2, 3}},
		"cmd/toolbox/other.go": {{4, 4}},
	}
	kept, dropped := splitExcluded(changed, []string{"cmd/tool"})
	if !slices.Equal(dropped, []string{"cmd/tool/main.go"}) {
		t.Errorf("dropped %v", dropped)
	}
	if len(kept) != 3 || kept["cmd/tool/main.go"] != nil {
		t.Errorf("kept %v", kept)
	}
}

func TestParseGateArgs(t *testing.T) {
	good := [][]string{
		{"-base", "origin/main"},
		{"-all", "-workers", "1", "-timeout-coefficient", "1", "-threshold", "0"},
		{"-base", "HEAD", "-timeout-coefficient", "5", "-threshold", "100", "-memory-mib", "512"},
	}
	for _, args := range good {
		if _, err := parseGateArgs(args, &bytes.Buffer{}); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	bad := [][]string{
		{},
		{"-base", "main", "-all"},
		{"-base", "main", "extra"},
		{"-base", "main", "-workers", "0"},
		{"-base", "main", "-timeout-coefficient", "6"},
		{"-base", "main", "-timeout-coefficient", "0"},
		{"-base", "main", "-cap", "0s"},
		{"-base", "main", "-memory-mib", "0"},
		{"-base", "main", "-threshold", "100.5"},
		{"-base", "main", "-threshold", "-1"},
		{"-base", "main", "-nope"},
	}
	for _, args := range bad {
		if _, err := parseGateArgs(args, &bytes.Buffer{}); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
}

func TestParseGateArgsDefaults(t *testing.T) {
	t.Setenv("GITHUB_STEP_SUMMARY", "/tmp/summary.md")
	opts, err := parseGateArgs([]string{"-base", "main"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if opts.workers != 2 || opts.coefficient != 5 || opts.cap.Minutes() != 3 ||
		opts.memoryMiB != 4096 || opts.threshold != 90 || opts.summary != "/tmp/summary.md" {
		t.Errorf("defaults: %+v", opts)
	}
}

func TestGremlinsEnv(t *testing.T) {
	environ := []string{
		"HOME=/home/u", "PATH=/bin", "GOFLAGS=-mod=mod", "GREMLINS_UNLEASH_WORKERS=16",
		"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=a.b", "GIT_CONFIG_VALUE_0=c",
		"GIT_EXTERNAL_DIFF=true", "GIT_DIFF_OPTS=-u9", "GIT_CONFIG_GLOBAL=/home/u/.gitconfig",
		"GIT_CONFIG_NOSYSTEM=0", "GIT_CONFIG_PARAMETERS='diff.noprefix'='true'",
	}
	opts := gateOptions{workers: 2, cap: defaultCap, memoryMiB: 1536}
	w := workFiles{events: "/tmp/w/events.log", bin: "/tmp/w/bin"}
	got := gremlinsEnv(environ, "/opt/mutationgate", opts, w)
	want := []string{
		"HOME=/home/u", "GIT_CONFIG_KEY_0=a.b", "GIT_CONFIG_VALUE_0=c",
		"PATH=/tmp/w/bin" + string(os.PathListSeparator) + "/bin",
		"GOFLAGS=-mod=mod -count=1 -skip=^TestAcc -p=2 " +
			"'-exec=/opt/mutationgate exec -cap=3m0s -memory-mib=1536 -log=/tmp/w/events.log'",
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_KEY_2=diff.context", "GIT_CONFIG_VALUE_2=0", "GIT_CONFIG_COUNT=3",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}

	got = gremlinsEnv(nil, "/opt/mutationgate", gateOptions{workers: 1, cap: time.Minute,
		memoryMiB: 64}, w)
	if !slices.Contains(got, "PATH=/tmp/w/bin") || !slices.Contains(got, "GIT_CONFIG_COUNT=1") {
		t.Errorf("empty environment: got %q", got)
	}
}

func TestBuildGremlinsArgs(t *testing.T) {
	opts := gateOptions{workers: 2, coefficient: 5}
	s := scope{base: "origin/main", excludedDirs: []string{"internal/acctest/vulnscan"}}
	got := strings.Join(buildGremlinsArgs(opts, s, "/w/results.json", "/w/gremlins.yaml"), " ")
	want := "unleash --config /w/gremlins.yaml --workers 2 --timeout-coefficient 5 " +
		"--output /w/results.json --diff origin/main " +
		`--exclude-files ^internal/acctest/vulnscan/[^/]+\.go$`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	s.all = true
	got = strings.Join(buildGremlinsArgs(opts, s, "/r", "/c"), " ")
	if strings.Contains(got, "--diff") {
		t.Errorf("-all ran with --diff: %s", got)
	}
}

func TestIsolateGit(t *testing.T) {
	environ := []string{"HOME=/home/u", "GIT_EXTERNAL_DIFF=true", "GIT_CONFIG_GLOBAL=/home/u/g",
		"GIT_DIR=/r/.git"}
	got := isolateGit(environ)
	want := []string{"HOME=/home/u", "GIT_DIR=/r/.git", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCheckPlainPath(t *testing.T) {
	if err := checkPlainPath("gate", "/usr/local/bin/mutationgate"); err != nil {
		t.Error(err)
	}
	for _, p := range []string{"", "/my dir/mutationgate", "/it's/mutationgate", `C:\gate`} {
		if checkPlainPath("gate", p) == nil {
			t.Errorf("%q: no error", p)
		}
	}
}
