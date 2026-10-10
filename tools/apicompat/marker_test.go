// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
)

func TestMarkedBy(t *testing.T) {
	cases := []struct {
		name     string
		messages []string
		title    string
		body     string
		marked   bool
	}{
		{name: "nothing", messages: []string{"feat(api): a new field\n\nTask: #1"}},
		{name: "a bang in a commit title", messages: []string{"fix: other", "feat(api)!: remove /tasks"}, marked: true},
		{name: "a bang without a scope", messages: []string{"refactor!: drop the old route"}, marked: true},
		{name: "the footer in a commit body", messages: []string{"feat(api): rename\n\nBREAKING CHANGE: `name` is `title` now"}, marked: true},
		{name: "the hyphenated footer", messages: []string{"feat(api): rename\n\nBREAKING-CHANGE: gone"}, marked: true},
		{name: "a bang in the pull request's title", title: "feat(api)!: remove /tasks", marked: true},
		{name: "the footer in the description", title: "feat(api): x", body: "## What and why\n\nBREAKING CHANGE: gone\n", marked: true},
		// The template asks the question in mixed case; answering it is not the marker.
		{name: "the template's question answered", title: "feat(api): x", body: "- **Breaking change:** yes - the migration path"},
		{name: "the footer in lower case", messages: []string{"feat: x\n\nbreaking change: gone"}},
		{name: "the footer quoted inside a line", messages: []string{"docs: explain `BREAKING CHANGE: x` footers"}},
		{name: "a bang after the colon", messages: []string{"feat(api): remove /tasks!"}},
		{name: "a bang in a body line, not the title", messages: []string{"fix: y\n\nfeat(api)!: as a quote"}},
		{name: "a footer with nothing after it", messages: []string{"feat: x\n\nBREAKING CHANGE:"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := markedBy(c.messages, c.title, c.body); (got != "") != c.marked {
				t.Errorf("markedBy = %q, want marked %v", got, c.marked)
			}
		})
	}
}

func TestAddedKeepsOnlyTheBranchsOwnBreaks(t *testing.T) {
	onBase := change{ID: "response-property-became-optional", Fingerprint: "aaaaaaaaaaaa"}
	ours := change{ID: "api-path-removed-without-deprecation", Fingerprint: "bbbbbbbbbbbb"}

	got := added([]change{onBase, ours}, []change{onBase})
	if len(got) != 1 || got[0].Fingerprint != ours.Fingerprint {
		t.Fatalf("added = %v, want only the branch's own break", got)
	}
	// A break the branch repairs is gone from now; it is not reported as added.
	if got := added(nil, []change{onBase}); len(got) != 0 {
		t.Fatalf("added = %v, want nothing", got)
	}
}

func TestParseSplitsBreaksFromWarnings(t *testing.T) {
	report := `[
	  {"id":"api-path-removed-without-deprecation","text":"api path removed without deprecation","level":3,"operation":"GET","path":"/meta/health","fingerprint":"111111111111"},
	  {"id":"api-removed-before-sunset","text":"api removed before the sunset date","level":2,"operation":"GET","path":"/x","fingerprint":"222222222222"},
	  {"id":"response-property-enum-value-added","text":"added the new value","level":1,"operation":"GET","path":"/y","fingerprint":"333333333333"}
	]`
	breaking, warnings, err := parse([]byte(report))
	if err != nil {
		t.Fatal(err)
	}
	if len(breaking) != 1 || breaking[0].Path != "/meta/health" {
		t.Errorf("breaking = %v", breaking)
	}
	if len(warnings) != 1 || warnings[0].Path != "/x" {
		t.Errorf("warnings = %v", warnings)
	}
	if !strings.Contains(breaking[0].String(), "GET /meta/health") {
		t.Errorf("a break names its operation: %s", breaking[0])
	}

	if _, _, err := parse([]byte("No breaking changes")); err == nil {
		t.Error("a report that is not JSON must not read as no breaks")
	}
}
