// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
)

// A subject document that links a superseded ADR as the reason for today's rule is flagged; one
// that names it as superseded, in the same paragraph or table row, is not.
func TestASupersededADRIsNotCitedAsTheCurrentRule(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/adr/ADR-0040-old.md": "# ADR-0040 — Old\n\n**Status:** superseded · **Superseded by:** [ADR-0094](./ADR-0094-new.md)\n",
		"docs/adr/ADR-0094-new.md": "# ADR-0094 — New\n\n**Status:** accepted\n",
		"docs/architecture/a.md": "| Mail | webhook-only ([ADR-0040](../adr/ADR-0040-old.md)) |\n" +
			"| Mail | one intake ([ADR-0094](../adr/ADR-0094-new.md)) |\n\n" +
			"One intake ([ADR-0094](../adr/ADR-0094-new.md),\nsuperseding [ADR-0040](../adr/ADR-0040-old.md)).\n",
	})
	got := checkSupersededADRCitations(root)
	if len(got) != 1 || !strings.Contains(got[0], "docs/architecture/a.md:1") || !strings.Contains(got[0], "ADR-0040") {
		t.Errorf("problems = %q, want the one table row citing ADR-0040 as current", got)
	}
}
