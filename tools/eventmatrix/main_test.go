// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"regexp"
	"testing"
)

// The matrix's text above the tables holds no number that moves with the catalogue: a count there
// is the one line every pull request adding a use case changes, so two of them merged without
// catching up conflict on it, or - each having changed it alike - leave a sum `make generate`
// disagrees with (ADR-0083).
func TestThePreambleCarriesNoCount(t *testing.T) {
	count := regexp.MustCompile(`\d+\s+(use cases|distinct action codes|action codes|of them|rows)\b`)
	if n := count.FindString(preamble()); n != "" {
		t.Errorf("the preamble counts %q; the tables are the count", n)
	}
}

// A use case that declares no obligation on success - a read, or a write gate SG-13 names as an
// exception - is recorded when refused, by the authorisation service; nothing else is true of it.
func TestAnEntryWithoutAnObligationIsRecordedWhenRefused(t *testing.T) {
	for _, c := range []struct {
		entry row
		want  string
	}{
		{row{required: true}, "Every time"},
		{row{required: false}, "When refused"},
	} {
		if got := recorded(c.entry); got != c.want {
			t.Errorf("recorded(%+v) = %q, want %q", c.entry, got, c.want)
		}
	}
}
