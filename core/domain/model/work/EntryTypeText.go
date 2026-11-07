// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// FieldTranslations names a change of a type's texts per language in its record.
const FieldTranslations = "translations"

// MaxEntryTypeLanguages bounds the further languages one type's texts are written in.
const MaxEntryTypeLanguages = 20

// entryTypeLanguagePattern takes a language, an optional script and an optional region - `de`,
// `pt-BR`, `zh-Hans` - the tags a locale catalogue is named by (i18n-l10n.md §2).
var entryTypeLanguagePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z][a-z]{3})?(-[A-Z]{2})?$`)

// EntryTypeText is a type's name and description in one language. An empty member reads the
// default text, so a language may name a type without describing it.
type EntryTypeText struct {
	Name        string
	Description string
}

// TextIn answers the type's words for a reader of locale (UC-SVC-05 check 7): the exact tag's,
// then its language's alone (`de` for `de-AT`), then the default text - field by field, so a
// language that only renames the type keeps the default description.
func (t EntryType) TextIn(locale string) EntryTypeText {
	out := EntryTypeText{Name: t.Name, Description: t.Description}
	candidates := []string{locale}
	if language, _, regional := strings.Cut(locale, "-"); regional {
		candidates = append(candidates, language)
	}
	for _, candidate := range slices.Backward(candidates) {
		for tag, words := range t.Translations {
			if !strings.EqualFold(tag, candidate) {
				continue
			}
			if words.Name != "" {
				out.Name = words.Name
			}
			if words.Description != "" {
				out.Description = words.Description
			}
		}
	}
	return out
}

// entryTypeDescription is a type's description: one paragraph no longer than a form's help.
func entryTypeDescription(raw string, form text.Normalizer, at string) (string, error) {
	description, err := shared.NFC(strings.TrimSpace(raw), form)
	if err != nil {
		return "", err
	}
	if len([]rune(description)) > MaxFormHelpLength {
		return "", shared.ErrValidation.WithDetail("entry_types.description_too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxFormHelpLength)}).
			WithFields(shared.FieldError{Path: at, Code: "entry_types.description_too_long"})
	}
	return description, nil
}

// entryTypeTranslations holds the texts per language to the name's and the description's rules;
// a language whose texts are all empty is dropped.
func entryTypeTranslations(raw map[string]EntryTypeText, form text.Normalizer) (map[string]EntryTypeText, error) {
	if len(raw) > MaxEntryTypeLanguages {
		return nil, shared.ErrValidation.WithDetail("entry_types.too_many_languages").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxEntryTypeLanguages)}).
			WithFields(shared.FieldError{Path: "/translations", Code: "entry_types.too_many_languages"})
	}
	out := map[string]EntryTypeText{}
	for tag, words := range raw {
		at := "/translations/" + tag
		if !entryTypeLanguagePattern.MatchString(tag) {
			return nil, shared.ErrValidation.WithDetail("entry_types.language_malformed").
				WithParams(map[string]string{"language": tag}).
				WithFields(shared.FieldError{Path: at, Code: "entry_types.language_malformed"})
		}
		var text EntryTypeText
		if strings.TrimSpace(words.Name) != "" {
			name, err := structureName(words.Name, entryTypeNameCodes, form)
			if err != nil {
				return nil, atFormPath(err, at+"/name")
			}
			text.Name = name
		}
		description, err := entryTypeDescription(words.Description, form, at+"/description")
		if err != nil {
			return nil, err
		}
		text.Description = description
		if text != (EntryTypeText{}) {
			out[tag] = text
		}
	}
	return out, nil
}

// translationsCode is the texts as one comparable value, for a change record.
func translationsCode(translations map[string]EntryTypeText) string {
	tags := make([]string, 0, len(translations))
	for tag := range translations {
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	var out strings.Builder
	for _, tag := range tags {
		words := translations[tag]
		out.WriteString(tag + "\x1f" + words.Name + "\x1f" + words.Description + "\x1e")
	}
	return out.String()
}
