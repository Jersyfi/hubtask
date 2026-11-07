// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-DOC-13 check 4: a reader's language is looked for as its exact tag, then as its language
// alone; the source page answers when neither is written.
func TestALanguageIsPreferredExactThenAlone(t *testing.T) {
	for _, tc := range []struct {
		locale string
		want   []string
	}{
		{"de-AT", []string{"de-at", "de"}},
		{"DE", []string{"de"}},
		{"zh-Hant-HK", []string{"zh-hant-hk", "zh"}},
		{" ", nil},
		{"not a tag!", nil},
	} {
		if got := LanguagePreference(tc.locale); !slices.Equal(got, tc.want) {
			t.Errorf("LanguagePreference(%q) = %v, want %v", tc.locale, got, tc.want)
		}
	}
	variants := []LanguageVariant{{PageID: pageB, Language: "de"}, {PageID: pageC, Language: "de-AT"}}
	for _, tc := range []struct {
		locale string
		want   []shared.ID
	}{
		{"de-AT", []shared.ID{pageC, pageB}},
		{"de-CH", []shared.ID{pageB}},
		{"de", []shared.ID{pageB}},
		{"fr", nil},
	} {
		if got := VariantsFor(tc.locale, variants); !slices.Equal(got, tc.want) {
			t.Errorf("VariantsFor(%q) = %v, want %v", tc.locale, got, tc.want)
		}
	}
}

// UC-DOC-13 check 4: a variant sits in its source's space under a tag nobody else of the source
// holds; it is no source itself, its source is no variant, and a source holds at most 20.
func TestALanguageLinkKeepsOneLevelAndOneTagEach(t *testing.T) {
	source := Page{ID: pageA, HubID: hub, ContentLanguage: "en"}
	variant := Page{ID: pageB, HubID: hub}
	full := make([]LanguageVariant, MaxLanguageVariants)
	for i := range full {
		full[i] = LanguageVariant{PageID: shared.ID(fmt.Sprintf("00000000-0000-0000-0000-%012d", 900+i)), Language: fmt.Sprintf("q%c", rune('a'+i))}
	}
	for _, tc := range []struct {
		name            string
		page            Page
		language        string
		sourceIsVariant bool
		ownVariants     int
		taken           []LanguageVariant
		want            string
	}{
		{name: "linked", page: variant, language: " de-AT "},
		{name: "the same tag again replaces", page: variant, language: "de", taken: []LanguageVariant{{PageID: pageB, Language: "de"}}},
		{name: "a full source takes a variant it holds", page: variant, language: "qa", taken: append(slices.Clone(full[1:]), LanguageVariant{PageID: pageB, Language: "qa"})},
		{name: "no tag", page: variant, language: "", want: "pages.language_malformed"},
		{name: "no language", page: variant, language: "not a tag!", want: "pages.language_malformed"},
		{name: "itself", page: source, language: "de", want: "pages.variant_of_itself"},
		{name: "another space", page: Page{ID: pageB, HubID: other}, language: "de", want: "pages.variant_other_space"},
		{name: "of a variant", page: variant, language: "de", sourceIsVariant: true, want: "pages.variant_of_a_variant"},
		{name: "a source itself", page: variant, language: "de", ownVariants: 1, want: "pages.variant_has_variants"},
		{name: "the source's own language", page: variant, language: "EN", want: "pages.variant_language_taken"},
		{name: "another variant's", page: variant, language: "de", taken: []LanguageVariant{{PageID: pageC, Language: "DE"}}, want: "pages.variant_language_taken"},
		{name: "past the bound", page: variant, language: "fr", taken: full, want: "pages.too_many_variants"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			link, err := NewLanguageLink(tc.page, source, tc.sourceIsVariant, tc.ownVariants, tc.taken, tc.language)
			if detail(err) != tc.want {
				t.Fatalf("NewLanguageLink: %+v %v, want %q", link, err, tc.want)
			}
			if tc.want == "" && (link.PageID != tc.page.ID || link.SourceID != source.ID || link.Language == "" || link.Language[0] == ' ') {
				t.Fatalf("the link: %+v", link)
			}
		})
	}
}
