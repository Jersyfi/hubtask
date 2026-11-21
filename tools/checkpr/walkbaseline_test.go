// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
	"time"
)

const baselineText = "# known answers\nbody-mismatch GET /x 200\nstatus-undeclared GET /y 500\n"

func TestAddedBaselineEntries(t *testing.T) {
	removed := "# known answers\nbody-mismatch GET /x 200\n"
	if added := addedBaselineEntries(baselineText, removed); len(added) != 0 {
		t.Fatalf("a removed entry was read as added: %v", added)
	}
	commented := baselineText + "# another comment\n\n"
	if added := addedBaselineEntries(baselineText, commented); len(added) != 0 {
		t.Fatalf("a comment was read as an entry: %v", added)
	}
	grown := baselineText + "body-mismatch GET /z 200\n"
	if added := addedBaselineEntries(baselineText, grown); len(added) != 1 || added[0] != "body-mismatch GET /z 200" {
		t.Fatalf("want the one added entry, got %v", added)
	}
}

// The rule reads the history: an entry the branch adds is refused, one it removes is not, and the
// branch that creates the baseline adds nothing to one.
func TestReadHistoryWalkBaseline(t *testing.T) {
	r := newScratchRepo(t)
	r.write("README.md", "x\n")
	r.commit("docs: a start")
	r.run("checkout", "-q", "-b", "creates")
	r.write(walkBaselinePath, baselineText)
	r.commit("test: the baseline")
	created, err := readHistory(r.dir, "main", "creates", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if p := historyProblems("", created); len(p) != 0 {
		t.Fatalf("the branch that creates the baseline was refused: %v", p)
	}

	r.run("checkout", "-q", "main")
	r.run("merge", "-q", "--ff-only", "creates")
	r.run("checkout", "-q", "-b", "removes")
	r.write(walkBaselinePath, "# known answers\nbody-mismatch GET /x 200\n")
	r.commit("test: one answer corrected")
	removed, err := readHistory(r.dir, "main", "removes", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if p := historyProblems("", removed); len(p) != 0 {
		t.Fatalf("a removed entry was refused: %v", p)
	}

	r.run("checkout", "-q", "-b", "adds", "main")
	r.write(walkBaselinePath, baselineText+"body-mismatch GET /z 200\n")
	r.commit("test: one answer excused")
	added, err := readHistory(r.dir, "main", "adds", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if p := strings.Join(historyProblems("", added), "\n"); !strings.Contains(p, "gains 1 entries") {
		t.Fatalf("an added entry was not refused:\n%s", p)
	}
	before := added
	before.opened = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if p := strings.Join(historyProblems("", before), "\n"); strings.Contains(p, "gains") {
		t.Fatalf("a pull request opened before the rule was held to it:\n%s", p)
	}
}
