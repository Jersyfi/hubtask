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
