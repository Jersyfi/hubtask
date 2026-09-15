// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package text_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/text"
)

// The two spellings of one visible character become one string, and a string already in normal
// form comes back byte-identical - the property every constructor's normalisation rests on.
func TestNFCComposesAndLeavesComposedTextAlone(t *testing.T) {
	forms := text.Forms{}

	cases := map[string]struct{ in, want string }{
		"a combining acute":         {"Cafe\u0301", "Caf\u00e9"},
		"a combining diaeresis":     {"Ba\u0308ume gießen", "B\u00e4ume gießen"},
		"Hangul jamo":               {"\u1112\u1161\u11ab", "\ud55c"},
		"already composed":          {"Caf\u00e9", "Caf\u00e9"},
		"ASCII":                     {"Buy milk", "Buy milk"},
		"empty":                     {"", ""},
		"a compatibility character": {"\ufb01le", "\ufb01le"},
	}
	for name, tc := range cases {
		if got := forms.NFC(tc.in); got != tc.want {
			t.Errorf("%s: %q → %q, want %q", name, tc.in, got, tc.want)
		}
	}
}
