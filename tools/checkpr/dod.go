// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The Definition of Done in a description is the template's list, each item ticked or marked n/a
// (engineering-guidelines.md §3). check() holds every item that is there to that; this holds the
// list itself to the template, because an item deleted or reworded is an item nobody answered -
// and "mark it n/a, do not delete it" is the template's own instruction.

// dodSince is when the comparison took effect; a description opened before it was written under
// the earlier rule and is not held to it.
var dodSince = readinessSince

var dodItem = regexp.MustCompile(`(?m)^- \[(?: |x|X)\] (?:(\d+)\.\s+)?(.*)$`)

type dodEntry struct {
	number string
	text   string
}

// templateDoD reads the template's Definition of Done items, in order.
func templateDoD(root string) ([]dodEntry, error) {
	raw, err := os.ReadFile(filepath.Join(root, ".github", "PULL_REQUEST_TEMPLATE.md")) //nolint:gosec // G304: a fixed path under the repository root
	if err != nil {
		return nil, fmt.Errorf("reading the template: %w", err)
	}
	return dodEntries(comment.ReplaceAllString(string(raw), "")), nil
}

func dodEntries(body string) []dodEntry {
	var out []dodEntry
	for _, s := range sectionsOf(strings.ReplaceAll(body, "\r\n", "\n")) {
		if s.title != "Definition of Done" {
			continue
		}
		for _, m := range dodItem.FindAllStringSubmatch(s.text, -1) {
			out = append(out, dodEntry{number: m[1], text: strings.TrimSpace(m[2])})
		}
	}
	return out
}

// dodProblems compares the description's items with the template's, by number. An item keeps the
// template's words; what follows them - "n/a", "— none found", the findings item 19 asks for - is
// the author's. A template item ending in "…" asks for that continuation, so only the words before
// it are held.
func dodProblems(body string, template []dodEntry) []string {
	body = comment.ReplaceAllString(body, "")
	want := map[string]string{}
	for _, item := range template {
		want[item.number] = strings.TrimSpace(strings.TrimSuffix(item.text, "…"))
	}
	var problems []string
	got := map[string]bool{}
	for _, item := range dodEntries(body) {
		short := item.text
		if len(short) > 60 {
			short = short[:60] + "…"
		}
		prefix, known := want[item.number]
		switch {
		case item.number == "" || !known:
			problems = append(problems, fmt.Sprintf("Definition of Done: %q is not an item of the template - the list is the template's, items are marked n/a rather than added or renumbered", short))
		case got[item.number]:
			problems = append(problems, fmt.Sprintf("Definition of Done: item %s is there twice", item.number))
		case !strings.HasPrefix(item.text, prefix):
			problems = append(problems, fmt.Sprintf("Definition of Done: item %s does not read as the template's %q - keep its words, add after them", item.number, prefix))
		}
		got[item.number] = true
	}
	for _, item := range template {
		if !got[item.number] {
			problems = append(problems, fmt.Sprintf("Definition of Done: item %s (%q) is deleted - mark it n/a instead", item.number, item.text))
		}
	}
	return problems
}

// heldTo is whether a pull request opened at opened answers to a rule that took effect at since.
// An unknown opening - a local run - is always held: the rule applies to what is written now.
func heldTo(opened, since time.Time) bool {
	return opened.IsZero() || !opened.Before(since)
}
