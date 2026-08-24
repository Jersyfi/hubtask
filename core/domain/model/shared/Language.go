// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import (
	"strings"
	"unicode/utf8"
)

// MaxLanguageTagLength is the bound a well-formed tag stays inside. Long enough for the longest
// tag anybody writes - `zh-Hant-HK-x-private` and its like - and short enough that a field holding
// one cannot carry a paragraph.
const MaxLanguageTagLength = 35

// LanguageTag checks the shape of a BCP 47 tag - `de`, `de-AT`, `pt-BR`, `zh-Hans` - and answers it
// trimmed. An empty string is well formed and means "not stated"; anything else that is not a tag
// is refused.
//
// Structural on purpose, in both places that use it. Which locales this product has translations
// for is a catalogue (i18n-l10n.md §2), and which languages it can *index* is what the installation's
// PostgreSQL has (ADR-0034) - neither of those is a reason to refuse a tag. A language with no
// translation falls back, and one with no text search configuration is indexed word by word. Both
// are answers; a validation error would not be.
//
// It lives here rather than in one of the two contexts because both need exactly this and neither
// owns it: an account's locale is identity's, an entry's content language is work's, and the
// grammar of a tag is neither's (Errors.go, "the vocabulary every bounded context needs").
func LanguageTag(raw string) (string, bool) {
	tag := strings.TrimSpace(raw)
	if tag == "" {
		return "", true
	}
	if utf8.RuneCountInString(tag) > MaxLanguageTagLength {
		return "", false
	}

	for index, part := range strings.Split(tag, "-") {
		if part == "" || len(part) > 8 {
			return "", false
		}
		for _, r := range part {
			isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
			// The first subtag is a language and is letters only; the ones after it may be digits
			// (a region like 419, the UN code for Latin America).
			isRegionDigit := r >= '0' && r <= '9' && index > 0
			if !isLetter && !isRegionDigit {
				return "", false
			}
		}
	}
	return tag, true
}
