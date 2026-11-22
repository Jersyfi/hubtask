// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/port/i18n"
)

// catalogues answers the catalogues an installation carries.
type catalogues []string

func (c catalogues) SupportedLocales() []i18n.LocaleInfo {
	infos := make([]i18n.LocaleInfo, 0, len(c))
	for _, tag := range c {
		infos = append(infos, i18n.LocaleInfo{Tag: tag})
	}
	return infos
}

// UC-ID-07/8: a language no catalogue serves, an empty one and one that is no tag name nothing and
// answer no language - the seed keeps the workspace's - without reading a single store (the stores
// are nil here, so reaching one would fail the test).
func TestASeedIsNamedOnlyInALanguageACatalogueServes(t *testing.T) {
	seed := LocaliseSeed{Locales: catalogues{"en", "de", "pt-BR"}}
	for _, tc := range []struct{ name, locale string }{
		{"none", ""},
		{"no catalogue", "ja"},
		{"no catalogue for the base either", "fr-CA"},
		{"a region of a language only served with another region", "pt"},
		{"not a tag", "de_DE!"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			named, err := seed.LocaliseSeed(t.Context(), identity.Account{}, tc.locale, time.Time{})
			if err != nil || named != "" {
				t.Errorf("named in %q (%v), want nothing named", named, err)
			}
		})
	}
}

// The catalogue is matched down the tag's subtags, as the renderer matches it.
func TestALanguageIsServedByItsOwnCatalogueOrItsBase(t *testing.T) {
	seed := LocaliseSeed{Locales: catalogues{"en", "de", "pt-BR"}}
	for _, tc := range []struct {
		locale string
		want   bool
	}{
		{"de", true}, {"de-AT", true}, {"DE-at", true}, {"pt-BR", true},
		{"pt", false}, {"ja", false}, {"en-Latn-US", true},
	} {
		if got := seed.served(tc.locale); got != tc.want {
			t.Errorf("%q served: %v, want %v", tc.locale, got, tc.want)
		}
	}
}
