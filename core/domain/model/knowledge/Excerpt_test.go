// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"strings"
	"testing"
)

// A search hit's passage is words, never the body's Markdown (UC-DOC-01/3): the passage is a
// fragment, so a mark may have lost its partner.
func TestAnExcerptCarriesNoMarkdown(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"heading and bold", "**Teilnehmende:** Anna, Ben\n\n## Beschlüsse\n\n1. «Lenkungskreis» tagt", "Teilnehmende: Anna, Ben Beschlüsse «Lenkungskreis» tagt"},
		{"a bold cut in half", "## Entscheidungen\n- **Passkey", "Entscheidungen Passkey"},
		{"table row", "| «Retro» | 45 min |\n|---|---|\n| Review | 30 min |", "«Retro» · 45 min Review · 30 min"},
		{"code keeps its backslashes", "Der «Drucker» `\\\\print-01\\OG2-Farbe` im OG", "Der «Drucker» \\\\print-01\\OG2-Farbe im OG"},
		{"link and reference", "see [the site](https://example.org) and ![](hubtask:view/00000000-0000-0000-0000-0000000000e2) now", "see the site and now"},
		{"emphasis, strike, escape and task", "- [x] *done* ~~gone~~ \\*star\\* a_b_c", "done gone *star* a_b_c"},
		{"quote, chips and containers", ":::info Hinweis\n> zitiert :date[2026-11-05] :status[Offen]{blue}\n:::", "Hinweis zitiert 2026-11-05 Offen"},
		{"plain stays", "Nur «Text» hier", "Nur «Text» hier"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PlainExcerpt(c.in); got != c.want {
				t.Errorf("PlainExcerpt(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// A tree row's excerpt is the body's first words, short, cut at a word.
func TestAnOpeningIsShortPlainAndCutAtAWord(t *testing.T) {
	long := "## Ziel\n\nDie App spricht **ausschließlich** mit der Public API v3. " + strings.Repeat("Es gibt keinen direkten Zugriff auf Datenbanken, ", 6)
	cases := []struct{ name, in, want string }{
		{"short stays whole", "# Titel\n\nEin *kurzer* Text.", "Titel Ein kurzer Text."},
		{"empty", "", ""},
		{"a mark cut by the head", "Der **Anfang", "Der Anfang"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Opening(c.in); got != c.want {
				t.Errorf("Opening(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
	got := Opening(long)
	runes := []rune(got)
	if len(runes) > MaxOpeningRunes || !strings.HasSuffix(got, "…") || strings.Contains(got, "**") {
		t.Fatalf("Opening(long) = %q (%d runes)", got, len(runes))
	}
	if !strings.HasPrefix(got, "Ziel Die App spricht ausschließlich mit der Public API v3.") {
		t.Errorf("Opening(long) = %q", got)
	}
	if words := strings.Fields(strings.TrimSuffix(got, "…")); !strings.Contains(long, words[len(words)-1]) {
		t.Errorf("Opening(long) cut inside a word: %q", got)
	}
}
