package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func m(file string, line int, status string) mutation {
	return mutation{file: file, line: line, column: 1, kind: "CONDITIONALS_BOUNDARY", status: status}
}

func TestEvaluate(t *testing.T) {
	diffScope := scope{base: "main", changed: map[string][]lineRange{"a.go": {{10, 12}}}}
	tests := []struct {
		name       string
		scope      scope
		threshold  float64
		mutations  []mutation
		events     []event
		wantExit   int
		wantNotice string
	}{
		{
			name: "efficacy equal to the threshold passes", scope: diffScope, threshold: 75,
			mutations: []mutation{m("a.go", 10, statusKilled), m("a.go", 10, statusKilled),
				m("a.go", 11, statusKilled), m("a.go", 12, statusLived), m("a.go", 3, statusSkipped),
				m("b.go", 1, statusSkipped)},
			wantExit: exitPassed, wantNotice: "detected 3 of the 4 mutants that ran: 75.00%",
		},
		{
			name: "efficacy below the threshold fails", scope: diffScope, threshold: 75.01,
			mutations: []mutation{m("a.go", 10, statusKilled), m("a.go", 10, statusKilled),
				m("a.go", 11, statusKilled), m("a.go", 12, statusLived)},
			wantExit: exitFailed, wantNotice: "75.00%, threshold 75.01%",
		},
		{
			name: "timed-out mutants count as detected", scope: diffScope, threshold: 90,
			mutations: []mutation{m("a.go", 10, statusTimedOut), m("a.go", 11, statusKilled),
				m("a.go", 12, statusNotCovered), m("a.go", 12, statusNotViable)},
			wantExit: exitPassed, wantNotice: "detected 2 of the 2 mutants that ran: 100.00%",
		},
		{
			name: "changed lines with nothing to mutate pass", scope: diffScope, threshold: 90,
			mutations: []mutation{m("a.go", 3, statusSkipped), m("b.go", 1, statusSkipped)},
			wantExit:  exitPassed, wantNotice: "hold nothing gremlins mutates",
		},
		{
			name: "no test covers the changed lines", scope: diffScope, threshold: 90,
			mutations: []mutation{m("a.go", 10, statusNotCovered), m("a.go", 11, statusNotCovered),
				m("a.go", 3, statusSkipped)},
			wantExit:   exitDidNotRun,
			wantNotice: "No mutant ran: of the 2 mutants in scope, 2 are on lines no test",
		},
		{
			name: "gremlins skipped a changed line", scope: diffScope, threshold: 90,
			mutations: []mutation{m("a.go", 10, statusKilled), m("a.go", 11, statusSkipped)},
			wantExit:  exitDidNotRun, wantNotice: "differ from git diff's for 1 mutants",
		},
		{
			name: "gremlins mutated an unchanged line", scope: diffScope, threshold: 90,
			mutations: []mutation{m("a.go", 10, statusKilled), m("a.go", 13, statusLived)},
			wantExit:  exitDidNotRun, wantNotice: "differ from git diff's for 1 mutants",
		},
		{
			name: "a mutant was left unrun", scope: diffScope, threshold: 90,
			mutations: []mutation{m("a.go", 10, statusKilled), m("a.go", 11, statusRunnable)},
			wantExit:  exitDidNotRun, wantNotice: "left 1 mutants unrun",
		},
		{
			name: "exec could not apply a limit", scope: diffScope, threshold: 90,
			mutations: []mutation{m("a.go", 10, statusKilled)},
			events: []event{
				{kind: eventCap, binary: "a.test", detail: "ran long"},
				{kind: eventError, binary: "a.test", detail: "EPERM"},
			},
			wantExit: exitDidNotRun, wantNotice: "could not do its part: a.test: EPERM",
		},
		{
			name: "mutants that do not compile are not counted", scope: diffScope, threshold: 50,
			mutations: []mutation{m("a.go", 10, statusKilled), m("a.go", 11, statusLived),
				m("a.go", 11, statusNotViable), m("a.go", 12, statusNotViable),
				m("a.go", 12, statusNotViable)},
			wantExit: exitPassed,
			wantNotice: "detected 1 of the 2 mutants that ran: 50.00%, threshold 50%. " +
				"3 mutants that do not compile are not counted.",
		},
		{
			name:  "a weak test fails once mutants that do not compile are left out",
			scope: diffScope, threshold: 50,
			mutations: []mutation{m("a.go", 10, statusKilled), m("a.go", 11, statusLived),
				m("a.go", 12, statusLived), m("a.go", 12, statusNotViable)},
			wantExit: exitFailed, wantNotice: "detected 1 of the 3 mutants that ran: 33.33%",
		},
		{
			name: "changed lines whose mutants do not compile pass", scope: diffScope, threshold: 90,
			mutations: []mutation{m("a.go", 10, statusNotViable), m("a.go", 11, statusNotViable),
				m("a.go", 3, statusSkipped)},
			wantExit: exitPassed, wantNotice: "None of the 2 mutants on the lines changed",
		},
		{
			name: "no mutant ran, and some are on lines no test covers", scope: diffScope,
			threshold: 90,
			mutations: []mutation{m("a.go", 10, statusNotViable), m("a.go", 11, statusNotCovered)},
			wantExit:  exitDidNotRun,
			wantNotice: "No mutant ran: of the 2 mutants in scope, 1 are on lines no test covers " +
				"and 1 do not compile",
		},
		{
			name: "whole module where no mutant compiles", scope: scope{all: true}, threshold: 50,
			mutations: []mutation{m("a.go", 1, statusNotViable)},
			wantExit:  exitDidNotRun, wantNotice: "No mutant ran",
		},
		{
			name: "stopped test processes do not fail a run", scope: diffScope, threshold: 90,
			mutations: []mutation{m("a.go", 10, statusKilled)},
			events:    []event{{kind: eventMemory, binary: "a.test", detail: "used 1700 MiB"}},
			wantExit:  exitPassed, wantNotice: "100.00%",
		},
		{
			name: "whole module", scope: scope{all: true}, threshold: 50,
			mutations: []mutation{m("a.go", 1, statusKilled), m("b.go", 2, statusLived)},
			wantExit:  exitPassed, wantNotice: "detected 1 of the 2 mutants that ran: 50.00%",
		},
		{
			name: "whole module with a skipped mutant", scope: scope{all: true}, threshold: 50,
			mutations: []mutation{m("a.go", 1, statusKilled), m("b.go", 2, statusSkipped)},
			wantExit:  exitDidNotRun, wantNotice: "differ from git diff's",
		},
		{
			name: "whole module with nothing to mutate", scope: scope{all: true}, threshold: 50,
			wantExit: exitDidNotRun, wantNotice: "found nothing to mutate in the module",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := report{opts: gateOptions{threshold: tt.threshold}, scope: tt.scope, mutations: tt.mutations,
				events: tt.events}
			o := r.evaluate()
			if o.exit != tt.wantExit || !strings.Contains(o.notice, tt.wantNotice) {
				t.Errorf("got exit %d %q, want exit %d and %q", o.exit, o.notice, tt.wantExit, tt.wantNotice)
			}
		})
	}
}

func TestReadResults(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("good.json", `{"files":[`+
		`{"file_name":"b.go","mutations":[{"type":"T","status":"LIVED","line":2,"column":1}]},`+
		`{"file_name":"a.go","mutations":[{"type":"T","status":"TIMED OUT","line":9,"column":4},`+
		`{"type":"T","status":"KILLED","line":3,"column":7}]}]}`)
	got, err := readResults(good)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, mu := range got {
		order = append(order, mu.String()+" "+mu.status)
	}
	want := "a.go:3:7 KILLED, a.go:9:4 TIMED OUT, b.go:2:1 LIVED"
	if strings.Join(order, ", ") != want {
		t.Errorf("got %s, want %s", strings.Join(order, ", "), want)
	}

	oneMutation := func(file, mutation string) string {
		return `{"files":[{` + file + `"mutations":[` + mutation + `]}]}`
	}
	bad := map[string]string{
		"missing":  filepath.Join(dir, "missing.json"),
		"not JSON": write("bad.json", `{"files":`),
		"unknown status": write("status.json",
			oneMutation(`"file_name":"a.go",`, `{"status":"DEAD","line":1}`)),
		"no line": write("line.json",
			oneMutation(`"file_name":"a.go",`, `{"status":"KILLED"}`)),
		"no file name": write("name.json",
			oneMutation("", `{"status":"KILLED","line":1}`)),
	}
	for name, p := range bad {
		if got, err := readResults(p); err == nil {
			t.Errorf("%s: got %v and no error", name, got)
		}
	}
}

func TestReadEvents(t *testing.T) {
	dir := t.TempDir()
	if events, err := readEvents(filepath.Join(dir, "none.log")); err != nil || events != nil {
		t.Errorf("no log: %v, %v", events, err)
	}
	p := filepath.Join(dir, "events.log")
	if err := os.WriteFile(p, []byte("cap\ta.test\tran long\tstill detail\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	events, err := readEvents(p)
	want := event{eventCap, "a.test", "ran long\tstill detail"}
	if err != nil || len(events) != 1 || events[0] != want {
		t.Errorf("got %v, %v", events, err)
	}
	if err := os.WriteFile(p, []byte("cap only\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if events, err := readEvents(p); err == nil {
		t.Errorf("malformed log: got %v and no error", events)
	}
}
