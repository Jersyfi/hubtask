// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"strings"

	port "github.com/Jersyfi/hubtask/core/port/i18n"
)

// SourceLocale is the language the catalogue is written in and the end of every fallback chain
// (i18n-l10n.md §2, §3).
const SourceLocale = "en"

// Renderer is the adapter for the i18n port: the catalogues this installation has, and the
// fallback between them.
//
// One catalogue today, which is the honest state - locales/ holds en.json and nothing else. The
// shape is what matters: a locale is an argument, so the day de.json arrives the rendering of an
// email in German is a file in a directory rather than a change here.
type Renderer struct {
	// catalogues is keyed by the lower-cased BCP 47 tag. Immutable once built and safe to share:
	// nothing writes to it after New returns.
	catalogues map[string]Catalogue
	source     Catalogue
}

// NewRenderer builds the renderer over the embedded catalogues.
func NewRenderer() (Renderer, error) {
	english, err := LoadEnglish()
	if err != nil {
		return Renderer{}, err
	}
	return Renderer{
		catalogues: map[string]Catalogue{SourceLocale: english},
		source:     english,
	}, nil
}

var _ port.Renderer = Renderer{}

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

// catalogue resolves a BCP 47 tag down its fallback chain: `de-AT` to `de-at`, then `de`, then
// nothing - and the caller falls back to the source language.
//
// By hand rather than through golang.org/x/text/language.NewMatcher, which i18n-l10n.md §2 names
// for the day there is more than one catalogue to match against. A matcher over a set of one
// always answers the one, so it would be a dependency doing arithmetic on an empty set.
func (r Renderer) catalogue(locale string) (Catalogue, bool) {
	tag := strings.ToLower(strings.TrimSpace(locale))
	for tag != "" {
		if catalogue, known := r.catalogues[tag]; known {
			return catalogue, true
		}
		cut := strings.LastIndex(tag, "-")
		if cut < 0 {
			break
		}
		tag = tag[:cut]
	}
	return Catalogue{}, false
}
