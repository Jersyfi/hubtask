// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/text/language"

	port "github.com/Jersyfi/hubtask/core/port/i18n"
)

// Renderer is the adapter for the i18n port: the catalogues this installation has, and the
// fallback between them.
//
// The catalogues are what the binary embeds - every `locales/<tag>.json` - with an operator's
// directory laid over them key by key (i18n-l10n.md §1). A locale is an argument to every call,
// so a language is a file in a directory and not a change here (arc42 QS-08).
type Renderer struct {
	// catalogues is keyed by the lower-cased BCP 47 tag. Immutable once built and safe to share:
	// nothing writes to it after the constructor returns.
	catalogues map[string]Catalogue
	source     Catalogue
	// locales is Locales() as built once: the source first, the rest sorted. The matcher's
	// supported list, in the same order, so that an index into one is an index into the other.
	locales []string
	matcher language.Matcher
}

// NewRenderer builds the renderer over the embedded catalogues alone.
func NewRenderer() (Renderer, error) {
	embedded, err := LoadEmbedded()
	if err != nil {
		return Renderer{}, err
	}
	return newRenderer(embedded, nil)
}

// NewRendererWithOverrides builds the renderer over the embedded catalogues with an operator's
// laid over them: a file for a tag the binary carries overrides that catalogue key by key, and a
// file for a new tag adds the locale (i18n-l10n.md §1).
//
// Key by key rather than file by file, because a partial override is the normal case - an
// operator correcting one sentence, or translating the ten a workspace sees first - and a whole
// catalogue replaced by a partial one would render the rest as codes.
func NewRendererWithOverrides(overrides map[string]Catalogue) (Renderer, error) {
	embedded, err := LoadEmbedded()
	if err != nil {
		return Renderer{}, err
	}
	return newRenderer(embedded, overrides)
}

func newRenderer(embedded, overrides map[string]Catalogue) (Renderer, error) {
	catalogues := make(map[string]Catalogue, len(embedded)+len(overrides))
	for tag, catalogue := range embedded {
		catalogues[tag] = catalogue
	}
	for tag, over := range overrides {
		tag = strings.ToLower(tag)
		if base, present := catalogues[tag]; present {
			catalogues[tag] = base.overlaid(over)
		} else {
			catalogues[tag] = over
		}
	}

	source, present := catalogues[SourceLocale]
	if !present {
		return Renderer{}, fmt.Errorf("building the renderer: no catalogue for the source language %s", SourceLocale)
	}

	// The matcher i18n-l10n.md §2 names, over exactly the catalogues present, the source first
	// because the first supported tag is the matcher's default: `de-AT` lands on `de` when that
	// is what there is, `pt-BR` prefers `pt-BR` over `pt` and takes `pt` otherwise, and a tag
	// nothing here serves lands on the source with no confidence - which is what the fallback
	// reads (ADR-0056).
	locales := sortedLocales(catalogues)
	supported := make([]language.Tag, 0, len(locales))
	for _, tag := range locales {
		supported = append(supported, language.Make(tag))
	}
	return Renderer{
		catalogues: catalogues, source: source,
		locales: locales, matcher: language.NewMatcher(supported),
	}, nil
}

func sortedLocales(catalogues map[string]Catalogue) []string {
	tags := make([]string, 0, len(catalogues))
	for tag := range catalogues {
		if tag != SourceLocale {
			tags = append(tags, tag)
		}
	}
	sort.Strings(tags)
	return append([]string{SourceLocale}, tags...)
}

var _ port.Renderer = Renderer{}

// Locales answers the tags this renderer has a catalogue for: the source language first, the
// rest sorted. Lower-cased, as they are keyed. It is what the manifest's `supported_locales`
// is derived from (i18n-l10n.md §2: "derived from the catalogue files present").
func (r Renderer) Locales() []string {
	return append([]string(nil), r.locales...)
}

// Render builds the sentence in the locale, or as close to it as this installation can get.
//
// It answers rather than failing, which is the port's contract and the fallback i18n-l10n.md §3
// prescribes: an unknown code renders as itself and a placeholder with no parameter is left
// standing. A missing translation must never become an undelivered email.
func (r Renderer) Render(locale, code string, params map[string]string) string {
	// A catalogue that has the locale but not the key falls through to the source language
	// rather than printing the key: a half-translated file is the normal state of a translation,
	// not an error (i18n-l10n.md §3).
	if catalogue, known := r.catalogue(locale); known && catalogue.Has(code) {
		message, _ := catalogue.Message(code, params)
		return message
	}
	message, _ := r.source.Message(code, params)
	return message
}

// For answers one catalogue that renders as Render would for the locale: the locale's messages
// over the source's. What a caller with many codes to render in one locale - the CLI - holds
// instead of a renderer and a tag.
func (r Renderer) For(locale string) Catalogue {
	catalogue, known := r.catalogue(locale)
	if !known {
		return r.source
	}
	return r.source.overlaid(catalogue)
}

// catalogue answers the catalogue a BCP 47 tag lands on, through the matcher: `de-AT` on `de`,
// `zh-Hant-HK` on `zh-Hant` where that exists, and nothing - the caller then falls back to the
// source language - when the matcher can offer only its default.
func (r Renderer) catalogue(locale string) (Catalogue, bool) {
	tag := strings.TrimSpace(locale)
	if tag == "" {
		return Catalogue{}, false
	}
	parsed, err := language.Parse(tag)
	if err != nil {
		return Catalogue{}, false
	}
	index, confidence := r.match(parsed)
	if confidence == language.No {
		return Catalogue{}, false
	}
	return r.catalogues[r.locales[index]], true
}
