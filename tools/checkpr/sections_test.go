// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
	"time"
)

const subjectText = "# Audit\n\n## 1. Purpose\n\nWhy.\n\n## 2. The chain\n\n### 2.1 Hashing\n\nSHA-256.\n\n### 2.2 Order\n\nBy sequence.\n\n" +
	"```text\n## 9. Not a section\n```\n\n## 3. Retention\n\nKept.\n"

func TestLostSections(t *testing.T) {
	cases := []struct {
		name  string
		after string
		want  string // "" = nothing lost
	}{
		{"unchanged", subjectText, ""},
		{"retitled", strings.Replace(subjectText, "## 2. The chain", "## 2. The hash chain", 1), ""},
		{"rewritten", strings.Replace(subjectText, "SHA-256.", "SHA-512, since the second key.", 1), ""},
		{"a new section at the next number", subjectText + "\n## 4. Export\n\nNew.\n", ""},
		{"a heading kept with a sentence", strings.Replace(subjectText, "Kept.", "Moved to data-retention.md.", 1), ""},
		{"a parent emptied but its subsections kept", strings.Replace(subjectText, "## 2. The chain\n\n", "## 2. The chain\n", 1), ""},
		{"a section removed", strings.Replace(subjectText, "## 3. Retention\n\nKept.\n", "", 1), "3"},
		{"a subsection removed", strings.Replace(subjectText, "### 2.2 Order\n\nBy sequence.\n\n", "", 1), "2.2"},
		{"renumbered after a removal", strings.NewReplacer("## 1. Purpose\n\nWhy.\n\n", "", "## 2.", "## 1.", "### 2.1", "### 1.1", "### 2.2", "### 1.2", "## 3.", "## 2.").Replace(subjectText), "2.1 2.2 3"},
		{"a heading left bare", strings.Replace(subjectText, "\n\nKept.\n", "\n", 1), "3 (bare)"},
		{"the document deleted", "", "1 2 2.1 2.2 3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, l := range lostSections("docs/architecture/audit.md", subjectText, c.after) {
				n := l.number
				if l.bare {
					n += " (bare)"
				}
				got = append(got, n)
			}
			if strings.Join(got, " ") != c.want {
				t.Fatalf("want %q lost, got %q", c.want, strings.Join(got, " "))
			}
		})
	}
}

// A document may carry two sections of one number, both cited; dropping either is a lost section.
func TestLostSectionOfARepeatedNumber(t *testing.T) {
	twice := subjectText + "\n## 3. Retention, again\n\nAlso cited.\n"
	lost := lostSections("docs/architecture/audit.md", twice, strings.Replace(twice, "## 3. Retention\n\nKept.\n", "", 1))
	if len(lost) != 1 || lost[0].number != "3" || lost[0].bare {
		t.Fatalf("want §3 lost, got %+v", lost)
	}
}

// The rule reads the history: a subject document the branch changes, at the merge base and the
// head; anything outside docs/architecture and docs/design is not a subject document.
func TestReadHistoryRenumberedSection(t *testing.T) {
	r := newScratchRepo(t)
	r.write("docs/architecture/audit.md", subjectText)
	r.write("docs/design/voice.md", subjectText)
	r.write("docs/backlog/milestone-x.md", subjectText)
	r.commit("docs: documents")
	r.run("checkout", "-q", "-b", "work")
	r.write("docs/architecture/audit.md", strings.Replace(subjectText, "### 2.2 Order\n\nBy sequence.\n\n", "", 1))
	r.write("docs/design/voice.md", strings.Replace(subjectText, "## 1. Purpose", "## 1. What it is for", 1))
	r.write("docs/backlog/milestone-x.md", "# Gone\n")
	r.commit("docs: edits")

	facts, err := readHistory(r.dir, "main", "work", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	problems := strings.Join(historyProblems("", facts, files(nil)), "\n")
	if !strings.Contains(problems, "docs/architecture/audit.md no longer has §2.2") {
		t.Fatalf("the removed section was not refused:\n%s", problems)
	}
	if strings.Contains(problems, "voice.md") || strings.Contains(problems, "milestone-x.md") {
		t.Fatalf("a retitled section or a document outside the subject documents was refused:\n%s", problems)
	}
	before := facts
	before.opened = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if p := strings.Join(historyProblems("", before, files(nil)), "\n"); strings.Contains(p, "no longer has") {
		t.Fatalf("a pull request opened before the rule was held to it:\n%s", p)
	}
}
