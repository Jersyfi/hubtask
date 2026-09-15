// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"strings"
	"testing"
)

// One matcher over the catalogues present (M-04, i18n-l10n.md §2). The cases the hand-rolled
// chain got wrong are the ones this exists for: a header whose first entry nobody serves, and a
// region that should land on its language.
func TestTheMatcherLandsATagOnTheCatalogueThatServesIt(t *testing.T) {
	embedded := directory(t, map[string]string{
		"en.json":    `{"a.one": "One"}`,
		"de.json":    `{"a.one": "Eins"}`,
		"pt.json":    `{"a.one": "Um"}`,
		"pt-BR.json": `{"a.one": "Um (BR)"}`,
	})
	renderer, err := newRenderer(embedded, nil)
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}

	for _, tc := range []struct{ locale, want string }{
		{"de", "Eins"},
		{"de-AT", "Eins"},
		{"de-CH-1996", "Eins"},
		{"pt-BR", "Um (BR)"},
		{"pt", "Um"},
		{"pt-PT", "Um"},
		{"en-GB", "One"},
		{"fr", "One"},
		{"", "One"},
		{"nonsense tag", "One"},
		{"zh-Hans", "One"},
	} {
		if got := renderer.Render(tc.locale, "a.one", nil); got != tc.want {
			t.Errorf("%q rendered %q, want %q", tc.locale, got, tc.want)
		}
	}
}

func TestNegotiateAnswersTheClientsTagThatThisInstallationServesBest(t *testing.T) {
	embedded := directory(t, map[string]string{
		"en.json": `{"a.one": "One"}`,
		"de.json": `{"a.one": "Eins"}`,
	})
	renderer, err := newRenderer(embedded, nil)
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}

	for _, tc := range []struct{ header, want string }{
		// The header meant "German if you cannot do French", and the hand-rolled parser could
		// not know that: it picked the highest weight and never asked what exists.
		{"fr, de;q=0.8", "de"},
		{"fr-CA, de-AT;q=0.8, en;q=0.5", "de-AT"},
		// The client's tag, region and all - not the catalogue's.
		{"de-AT", "de-AT"},
		{"de-AT;q=0.9, en;q=0.4", "de-AT"},
		// Nothing served: the client's first preference as it stands, so that an unserved
		// language still names itself on the actor.
		{"fr-CH, it;q=0.7", "fr-CH"},
		{"*", ""},
		{"", ""},
		{"   ", ""},
		{"not a header ;;", ""},
		{strings.Repeat("de,", 200), ""},
		// q=0 is "not acceptable".
		{"de;q=0, en", "en"},
	} {
		if got := renderer.Negotiate(tc.header); got != tc.want {
			t.Errorf("%q negotiated %q, want %q", tc.header, got, tc.want)
		}
	}
}
