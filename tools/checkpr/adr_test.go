// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
	"time"
)

const adrText = "# ADR-0081 — How work is organised\n\n**Status:** accepted · **Date:** 2026-10-07\n\n" +
	"**Rule lives in:** [AGENTS.md](../../AGENTS.md),\n[README.md](../backlog/README.md)\n\n" +
	"## Context\n\nWork stalled, see [the notes](../notes.md).\n\n## Decision\n\n1. One place per kind of knowledge.\n"

func TestADRRecord(t *testing.T) {
	same := []struct{ name, after string }{
		{"the status line moves", strings.Replace(adrText, "**Status:** accepted", "**Status:** superseded by ADR-0078", 1)},
		{"the rule moves", strings.Replace(adrText, "[README.md](../backlog/README.md)", "[ci-cd.md](../architecture/ci-cd.md) §3", 1)},
		{"a link target moves", strings.Replace(adrText, "(../notes.md)", "(../archive/notes.md)", 1)},
		{"a paragraph is reflowed", strings.Replace(adrText, "Work stalled, see", "Work stalled,\nsee", 1)},
	}
	for _, c := range same {
		if adrRecord(adrText) != adrRecord(c.after) {
			t.Errorf("%s: read as a change of the record", c.name)
		}
	}
	changed := []struct{ name, after string }{
		{"a decision reworded", strings.Replace(adrText, "One place per kind", "Two places per kind", 1)},
		{"an addendum", adrText + "\n## Amendment\n\nAnd also this.\n"},
		{"a link's words", strings.Replace(adrText, "[the notes]", "[other notes]", 1)},
	}
	for _, c := range changed {
		if adrRecord(adrText) == adrRecord(c.after) {
			t.Errorf("%s: not seen as a change of the record", c.name)
		}
	}
	if adrStatus(adrText) != "accepted" || adrStatus("**Status:** proposed · x") != "proposed" {
		t.Error("the status was not read")
	}
}

func TestReadHistorySettledADR(t *testing.T) {
	r := newScratchRepo(t)
	proposed := strings.Replace(adrText, "**Status:** accepted", "**Status:** proposed", 1)
	r.write("docs/adr/ADR-0081-accepted.md", adrText)
	r.write("docs/adr/ADR-0078-proposed.md", proposed)
	r.write("docs/adr/ADR-0079-status-only.md", adrText)
	r.write("docs/adr/ADR-0080-deleted.md", adrText)
	r.commit("docs: ADRs")
	r.run("checkout", "-q", "-b", "work")
	r.write("docs/adr/ADR-0081-accepted.md", adrText+"\n## Amendment\n\nLater.\n")
	r.write("docs/adr/ADR-0078-proposed.md", proposed+"\nRewritten while proposed.\n")
	r.write("docs/adr/ADR-0079-status-only.md", strings.Replace(adrText, "accepted", "superseded by ADR-0081", 1))
	r.remove("docs/adr/ADR-0080-deleted.md")
	r.commit("docs: edits")

	facts, err := readHistory(r.dir, "main", "work", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(facts.adrEdited, " ")
	if got != "docs/adr/ADR-0080-deleted.md docs/adr/ADR-0081-accepted.md" {
		t.Fatalf("want the amended and the deleted ADR, got %q", got)
	}
	problems := strings.Join(historyProblems("Readiness: n/a — x", facts, files(nil)), "\n")
	if !strings.Contains(problems, "ADR-0081-accepted.md is settled and this branch changes its text") {
		t.Fatalf("the amendment was not refused:\n%s", problems)
	}
	before := facts
	before.opened = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if p := historyProblems("", before, files(nil)); strings.Contains(strings.Join(p, "\n"), "is settled") {
		t.Fatalf("a pull request opened before the rule was held to it: %v", p)
	}
}
