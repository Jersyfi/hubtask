// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxLanguageVariants bounds the languages one page is written in beside its own, as a request
// type's texts are (work.MaxEntryTypeLanguages).
const MaxLanguageVariants = 20

// LanguageVariant is a page written in another language for the page it is a variant of
// (UC-DOC-13 check 4): where that page is published to a portal, a requester of the language
// reads this one in its place.
type LanguageVariant struct {
	PageID   shared.ID
	Language string
}

// LanguageLink makes a page the variant of its source under a tag.
type LanguageLink struct {
	PageID   shared.ID
	SourceID shared.ID
	Language string
}

// LanguagePreference answers the tags a reader of locale is answered in, preferred first and in
// lower case: the exact tag, then its language alone (`de` for `de-AT`). A locale that is no tag
// prefers none, and the source page answers.
func LanguagePreference(locale string) []string {
	tag, ok := shared.LanguageTag(locale)
	if !ok || tag == "" {
		return nil
	}
	tag = strings.ToLower(tag)
	out := []string{tag}
	if language, _, regional := strings.Cut(tag, "-"); regional {
		out = append(out, language)
	}
	return out
}

// VariantsFor answers the variants a reader of locale is answered in, preferred first; empty, the
// source page answers. The caller reads the first that is still live and unrestricted.
func VariantsFor(locale string, variants []LanguageVariant) []shared.ID {
	var out []shared.ID
	for _, tag := range LanguagePreference(locale) {
		for _, variant := range variants {
			if strings.EqualFold(variant.Language, tag) {
				out = append(out, variant.PageID)
			}
		}
	}
	return out
}

// NewLanguageLink checks a link of page to source under a tag. One level only - a variant is no
// source and its source no variant - so that a portal's choice stays one look; in the source's
// space, since the portal answers the variant wherever the source is published; and under a tag
// neither the source nor another of its variants holds. taken are the source's variants now.
func NewLanguageLink(page, source Page, sourceIsVariant bool, ownVariants int, taken []LanguageVariant, raw string) (LanguageLink, error) {
	tag, ok := shared.LanguageTag(raw)
	if !ok || tag == "" {
		return LanguageLink{}, invalid("/language", "pages.language_malformed")
	}
	if page.ID == source.ID {
		return LanguageLink{}, invalid("/source_page_id", "pages.variant_of_itself")
	}
	if page.HubID != source.HubID {
		return LanguageLink{}, invalid("/source_page_id", "pages.variant_other_space")
	}
	if sourceIsVariant {
		return LanguageLink{}, shared.ErrConflict.WithDetail("pages.variant_of_a_variant")
	}
	if ownVariants > 0 {
		return LanguageLink{}, shared.ErrConflict.WithDetail("pages.variant_has_variants")
	}
	if strings.EqualFold(source.ContentLanguage, tag) {
		return LanguageLink{}, shared.ErrConflict.WithDetail("pages.variant_language_taken").
			WithParams(map[string]string{"language": tag})
	}
	others := 0
	for _, variant := range taken {
		if variant.PageID == page.ID {
			continue
		}
		if strings.EqualFold(variant.Language, tag) {
			return LanguageLink{}, shared.ErrConflict.WithDetail("pages.variant_language_taken").
				WithParams(map[string]string{"language": tag})
		}
		others++
	}
	if others >= MaxLanguageVariants {
		return LanguageLink{}, shared.ErrConflict.WithDetail("pages.too_many_variants").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxLanguageVariants)})
	}
	return LanguageLink{PageID: page.ID, SourceID: source.ID, Language: tag}, nil
}
