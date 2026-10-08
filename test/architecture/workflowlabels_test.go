// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	// formLabels is an issue form's label line: labels: ["bug", "finding"].
	formLabels = regexp.MustCompile(`(?m)^labels:\s*\[([^\]]*)\]`)
	// The three ways a workflow's script names a label: filing under it, matching one of a list,
	// and matching one.
	scriptLabelList  = regexp.MustCompile(`labels:\s*\[([^\]]*)\]`)
	scriptLabelAny   = regexp.MustCompile(`\[([^\]]*)\]\.includes\(\s*label`)
	scriptLabelEqual = regexp.MustCompile(`label\)?\s*===\s*['"]([^'"]+)['"]`)
	quoted           = regexp.MustCompile(`['"]([^'"]+)['"]`)
)

// A workflow that files an issue, or closes only the issues it filed, does so by a label. The
// labels that exist are the ones the issue forms declare; a workflow naming another one files under
// a label nobody triages, or keeps accepting one that was retired.
func TestWorkflowsNameOnlyKnownLabels(t *testing.T) {
	known := map[string]bool{}
	forms, err := filepath.Glob("../../.github/ISSUE_TEMPLATE/*.yml")
	if err != nil || len(forms) == 0 {
		t.Fatalf("no issue form found: %v", err)
	}
	for _, form := range forms {
		for _, m := range formLabels.FindAllStringSubmatch(string(readFile(t, form)), -1) {
			for _, q := range quoted.FindAllStringSubmatch(m[1], -1) {
				known[q[1]] = true
			}
		}
	}
	workflows, err := filepath.Glob("../../.github/workflows/*.yml")
	if err != nil || len(workflows) == 0 {
		t.Fatalf("no workflow found: %v", err)
	}
	named := 0
	for _, workflow := range workflows {
		for _, label := range scriptLabels(string(readFile(t, workflow))) {
			named++
			if !known[label] {
				t.Errorf("%s names the label %q, which no issue form declares", filepath.Base(workflow), label)
			}
		}
	}
	if named == 0 {
		t.Fatal("no workflow names a label - the patterns no longer match how the scripts name them")
	}
}

// A list of accepted labels is read entry by entry, so one retired label among current ones is
// still found.
func TestScriptLabels(t *testing.T) {
	script := `
              await github.rest.issues.create({ ...context.repo, title, body, labels: ['finding'] });
              if (!issue.labels.some((label) => ['finding', 'claude:task'].includes(label.name ?? label))) {
              if (!issue.labels.some((label) => (label.name ?? label) === "blocked")) {
          echo "the committed label tokens match"
`
	got := strings.Join(scriptLabels(script), " ")
	if want := "blocked claude:task finding finding"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func scriptLabels(script string) []string {
	var out []string
	for _, re := range []*regexp.Regexp{scriptLabelList, scriptLabelAny} {
		for _, m := range re.FindAllStringSubmatch(script, -1) {
			for _, q := range quoted.FindAllStringSubmatch(m[1], -1) {
				out = append(out, q[1])
			}
		}
	}
	for _, m := range scriptLabelEqual.FindAllStringSubmatch(script, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}
