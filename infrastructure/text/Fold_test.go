// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package text_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/text"
)

func TestFoldedComparesNamesAsTheUniqueIndexesDo(t *testing.T) {
	for _, pair := range [][2]string{
		{"Grün", "grun"},
		{" Doing ", "doing"},
		{"ÉTÉ", "ete"},
		{"Grün", "grun"},
	} {
		if got := text.Folded(pair[0]); got != pair[1] {
			t.Errorf("Folded(%q) = %q, want %q", pair[0], got, pair[1])
		}
	}
	if text.Folded("Done") == text.Folded("Doing") {
		t.Error("two names stay two")
	}
}
