// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import "testing"

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
