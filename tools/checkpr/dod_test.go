// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
	"time"
)

// filledDoD is the template's Definition of Done as an author fills it: every item ticked or n/a,
// the findings named.
func filledDoD(t *testing.T, template []dodEntry) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("## Definition of Done\n\n")
	for _, item := range template {
		text := item.number + ". " + item.text
		switch {
		case strings.HasSuffix(item.text, "…"):
			b.WriteString("- [x] " + strings.TrimSuffix(text, "…") + "none\n")
		case item.number == "4":
			b.WriteString("- [ ] " + text + " — n/a\n")
		default:
			b.WriteString("- [x] " + text + "\n")
		}
	}
	return b.String()
}

func TestDoDMatchesTheTemplate(t *testing.T) {
	template, err := templateDoD(rootForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(template) < 10 {
		t.Fatalf("the template's Definition of Done was not read: %v", template)
	}
	body := filledDoD(t, template)
	if p := dodProblems(body, template); len(p) != 0 {
		t.Fatalf("the template's own list was refused:\n%s", strings.Join(p, "\n"))
	}

	first := "1. " + template[0].text
	cases := []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{"an item deleted", func(s string) string {
			return strings.Replace(s, "- [x] "+first+"\n", "", 1)
		}, "item 1 (" + `"` + template[0].text[:10]},
		{"an item renamed", func(s string) string {
			return strings.Replace(s, first, "1. Tests green somewhere", 1)
		}, "item 1 does not read as the template's"},
		{"an item added", func(s string) string {
			return s + "- [x] 20. Something else\n"
		}, "is not an item of the template"},
		{"an item without its number", func(s string) string {
			return strings.Replace(s, first, template[0].text, 1)
		}, "is not an item of the template"},
		{"an item twice", func(s string) string {
			return s + "- [x] " + first + "\n"
		}, "item 1 is there twice"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(dodProblems(c.mutate(body), template), "\n")
			if !strings.Contains(got, c.want) {
				t.Fatalf("want a problem containing %q, got:\n%s", c.want, got)
			}
		})
	}
}

func TestHeldTo(t *testing.T) {
	since := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	if !heldTo(time.Time{}, since) {
		t.Error("a local run, with no opening date, was not held to the rule")
	}
	if heldTo(since.Add(-time.Hour), since) {
		t.Error("a pull request opened before the rule was held to it")
	}
	if !heldTo(since, since) {
		t.Error("a pull request opened when the rule took effect was not held to it")
	}
}
