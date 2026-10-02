package main

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMutantTestRun(t *testing.T) {
	tests := []struct {
		args      []string
		wantPkg   string
		wantFlags []string
		wantOK    bool
	}{
		{
			args:    []string{"test", "-timeout", "5m2s", "-failfast", "m/pkg"},
			wantPkg: "m/pkg", wantOK: true,
		},
		{
			args:    []string{"test", "-tags", "e2e", "-timeout", "5m2s", "-failfast", "-cpu 2", "m/pkg"},
			wantPkg: "m/pkg", wantFlags: []string{"-tags", "e2e"}, wantOK: true,
		},
		{args: []string{"test", "-timeout", "5m2s", "-failfast", "./..."}},
		{args: []string{"test", "-cover", "-coverprofile", "/w/coverage", "./..."}},
		{args: []string{"test", "m/pkg"}},
		{args: []string{"test", "-failfast"}},
		{args: []string{"mod", "download"}},
		{args: []string{"test"}},
		{},
	}
	for _, tt := range tests {
		pkg, flags, ok := mutantTestRun(tt.args)
		if pkg != tt.wantPkg || !slices.Equal(flags, tt.wantFlags) || ok != tt.wantOK {
			t.Errorf("%q: got %q %q %v", tt.args, pkg, flags, ok)
		}
	}
}

func TestParseGoShimArgs(t *testing.T) {
	realGo := filepath.Join(t.TempDir(), "go")
	opts, err := parseGoShimArgs(
		[]string{"-real-go=" + realGo, "-log=/w/events.log", "--", "test", "-v"},
		&bytes.Buffer{})
	if err != nil || opts.realGo != realGo || opts.logPath != "/w/events.log" ||
		strings.Join(opts.args, " ") != "test -v" {
		t.Errorf("got %+v, %v", opts, err)
	}
	for _, args := range [][]string{{"--", "env"}, {"-real-go=go", "--", "env"}, {"-nope"}} {
		if _, err := parseGoShimArgs(args, &bytes.Buffer{}); err == nil {
			t.Errorf("%q: no error", args)
		}
	}
}
