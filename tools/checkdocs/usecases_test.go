// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const useCaseBody = "\n## Goal\n\nA person keeps a hub private.\n\n## How to check\n\n1. One.\n2. Two.\n3. Three.\n\n## Where it ends\n\nHere.\n"

func TestToday(t *testing.T) {
	cases := []struct {
		name, state, today, want string
	}{
		{"partial with one line per unmet check", "partial", "* Check 2: not met — the screen\n* Check 3: not met in the web app — x\n", ""},
		{"built without Today", "built", "", ""},
		{"verified without Today", "verified", "", ""},
		{"partial without Today", "partial", "", "says in ## Today what is not met yet"},
		{"specified without Today", "specified", "", "says in ## Today what is not met yet"},
		{"built with a Today", "built", "* Check 2: not met — x\n", "a built use case has no ## Today"},
		{"verified with a Today", "verified", "* Check 2: not met — x\n", "a verified use case has no ## Today"},
		{"a line that names no check", "partial", "* Check 2: not met — x\nThe rest is fine.\n", `each line is "* Check n`},
		{"a check that does not exist", "partial", "* Check 7: not met — x\n", "lists check 7, which ## How to check does not have"},
		{"a check listed twice", "partial", "* Check 2: not met — x\n* Check 2: not met in hubctl — y\n", "lists check 2 twice"},
		{"a Today with no check", "partial", "Nothing yet.\n", "names no check"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := useCaseBody
			if c.today != "" {
				body += "\n## Today\n\n" + c.today
			}
			uc := useCase{path: "x.md", id: "UC-ID-16", fields: map[string]string{"state": c.state}, lists: map[string][]string{}, body: body}
			var got []string
			for _, p := range checkUseCase(t.TempDir(), uc, nil, nil, nil) {
				if strings.Contains(p, "Today") {
					got = append(got, p)
				}
			}
			joined := strings.Join(got, "\n")
			if c.want == "" && joined != "" {
				t.Fatalf("want nothing about Today, got:\n%s", joined)
			}
			if c.want != "" && !strings.Contains(joined, c.want) {
				t.Fatalf("want %q, got:\n%s", c.want, joined)
			}
		})
	}
}

// A verified use case's checks are each cited by a file of its checked_by, in either spelling;
// the one nobody cites is named.
func TestUncitedChecksOfAVerifiedUseCase(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a_test.go"), []byte("// UC-ID-16 check 1, and UC-ID-16/3\n// UC-ID-17 check 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := uncitedChecks(root, "UC-ID-16", numberedChecks(sectionsOf(useCaseBody)["How to check"]), []string{"a_test.go"})
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("uncited %v, want [2] - check 2 is cited only for another use case", got)
	}
}

// A file proving several checks may name them in one list - `checks 1, 2 and 3`; each counts.
func TestAListOfChecksCitesEachOfThem(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.test.mjs"), []byte("// (UC-ID-16 checks 1, 2 and 3, design.md)\n// UC-ID-17 checks 4 and 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := uncitedChecks(root, "UC-ID-16", map[int]bool{1: true, 2: true, 3: true, 4: true}, []string{"a.test.mjs"}); len(got) != 1 || got[0] != 4 {
		t.Fatalf("uncited %v, want [4] - checks 1 to 3 are named in one list, check 4 only for another use case", got)
	}
}

// A range in a list - `checks 1-3` or `checks 1–3` - cites every check in it.
func TestARangeOfChecksCitesEachOfThem(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a_test.go"), []byte("// UC-ID-16 checks 1-2 and 4\n// UC-ID-17 checks 1–3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := uncitedChecks(root, "UC-ID-16", map[int]bool{1: true, 2: true, 3: true, 4: true}, []string{"a_test.go"})
	if len(got) != 1 || got[0] != 3 {
		t.Fatalf("uncited %v, want [3] - checks 1 to 2 and 4 are cited, 3 only for another use case", got)
	}
}

// A citation a comment wraps between the use case and its check still cites it - in Go, script
// and a block comment alike; prose that only follows the use case on the next line does not.
func TestACitationWrappedOntoTheNextCommentLineCites(t *testing.T) {
	root := t.TempDir()
	content := "// as the screen draws it (UC-ID-16\n// check 1), and\n/* UC-ID-16,\n * checks 2 and 3 */\n// UC-ID-16\nthe check 4 below\n"
	if err := os.WriteFile(filepath.Join(root, "a.test.mjs"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got := uncitedChecks(root, "UC-ID-16", map[int]bool{1: true, 2: true, 3: true, 4: true}, []string{"a.test.mjs"})
	if len(got) != 1 || got[0] != 4 {
		t.Fatalf("uncited %v, want [4] - checks 1 to 3 are cited across a wrap, 4 is not a comment", got)
	}
}

// The built bound is a ratchet: more uncited checks than it allows fail, and fewer fail too until
// the bound is lowered, so a check cited once can never quietly fall back.
func TestTheBuiltCitationBoundIsARatchet(t *testing.T) {
	if got := builtCitationProblems(5, 5); len(got) != 0 {
		t.Errorf("at the bound: %v", got)
	}
	if got := builtCitationProblems(6, 5); len(got) != 1 || !strings.Contains(got[0], "at most 5") {
		t.Errorf("above the bound: %v", got)
	}
	if got := builtCitationProblems(4, 5); len(got) != 1 || !strings.Contains(got[0], "lower uncitedBuiltAtMost to 4") {
		t.Errorf("below the bound: %v", got)
	}
}
