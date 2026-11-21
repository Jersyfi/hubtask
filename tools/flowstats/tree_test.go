// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
)

func TestTreeValues(t *testing.T) {
	long := strings.Repeat("// a line\n", 20) + "package a\n"
	short := strings.Repeat("\t// a line\n", 19) + "package b\n"
	got := treeValues(snapshot{
		agentsPath:                        []byte("one\ntwo\nthree\n"),
		boundsPath:                        []byte(`{"fixed_waits": {"a.test.mjs": 2, "b.test.mjs": 3}, "timer_waits": {"a.test.mjs": 1}, "no_check_cited": ["a.test.mjs", "b.test.mjs"]}`),
		baselinePath:                      []byte("# a comment\nbody-mismatch GET /a 200\n\nbody-mismatch GET /b 200\n"),
		uncitedPath:                       []byte("package main\n\nconst uncitedBuiltAtMost = 313\n"),
		"docs/usecases/work/UC-WRK-01.md": []byte("---\nid: UC-WRK-01\nstate: built\n---\n\nstate: verified\n"),
		"docs/usecases/work/UC-WRK-02.md": []byte("---\nid: UC-WRK-02\nstate: verified\n---\n"),
		"docs/usecases/work/UC-WRK-03.md": []byte("---\nid: UC-WRK-03\nstate: verified\n---\n"),
		"docs/usecases/README.md":         []byte("# Use cases\n\nstate: built\n"),
		"core/a/a.go":                     []byte(long + long),
		"cmd/b/b.go":                      []byte(short),
	})
	for id, want := range map[string]string{
		"agents_lines":             "3",
		"fixed_waits":              "5",
		"timer_waits":              "1",
		"suites_without_use_case":  "2",
		"answers_outside_contract": "2",
		"uncited_checks_built":     "313",
		"use_cases_by_state":       "verified 2, built 1",
		"comment_blocks":           "2",
	} {
		if got[id].String() != want {
			t.Errorf("%s = %q, want %q", id, got[id], want)
		}
	}
}

// A file the commit does not have is not measured; it is not a zero.
func TestTreeValuesOfACommitWithoutTheFiles(t *testing.T) {
	for id, v := range treeValues(snapshot{}) {
		if v.why == "" || v.numeric {
			t.Errorf("%s reads %q from an empty tree", id, v)
		}
	}
}

// What `deadcode` prints, reduced: one function a line, and nothing else counts.
func TestUnreachableIn(t *testing.T) {
	out := "presentation/rest/Middleware.go:67:16: unreachable func: Chain.ServeHTTP\npresentation/worker/Runner.go:197:17: unreachable func: Runner.round\n\n"
	if got := unreachableIn([]byte(out)); got != 2 {
		t.Errorf("%d unreachable functions, want 2", got)
	}
	if got := unreachableIn(nil); got != 0 {
		t.Errorf("%d unreachable functions in no output", got)
	}
}

// The workspace tool's JSON report, reduced to two files' issues.
func TestUnusedIn(t *testing.T) {
	report := `{"issues": [
 {"file": "apps/webapp/src/lib/old.ts", "owners": [], "files": [{"name": "apps/webapp/src/lib/old.ts"}], "dependencies": [], "exports": [], "types": []},
 {"file": "packages/query/src/index.ts", "owners": [], "files": [], "dependencies": [{"name": "left"}],
  "exports": [{"name": "parseOld", "line": 4}, {"name": "format", "line": 9}], "types": [{"name": "Old", "line": 2}]}
]}`
	got, err := unusedIn([]byte(report))
	if err != nil {
		t.Fatal(err)
	}
	if !got.numeric || got.n != 4 || got.String() != "4 (1 files, 2 exports, 1 types)" {
		t.Errorf("unused: %v %q", got.n, got)
	}
	if _, err := unusedIn([]byte("ERROR: no report")); err == nil {
		t.Error("an error message reads as a report")
	}
}
