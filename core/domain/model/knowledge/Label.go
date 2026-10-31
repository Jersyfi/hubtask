// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The bounds of a page's labels (UC-DOC-01 check 9).
const (
	MaxLabelLength = 40
	MaxPageLabels  = 20
)

// Labels brings a page's labels to their one written form: lower case, each once, alphabetical.
//
// One word rather than free text, so that a label is one token in a link, a filter and a facet; and
// lower case, so that "Onboarding" and "onboarding" are not two facets of the same space. A space's
// vocabulary is whatever its pages carry - there is no list to define first, as a collection's
// labels have, because a page's labels are found rather than kept.
func Labels(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))
	for i, value := range raw {
		label := strings.ToLower(strings.TrimSpace(value))
		path := "/labels/" + strconv.Itoa(i)
		switch {
		case label == "":
			return nil, shared.ErrValidation.WithDetail("pages.label_empty").
				WithFields(shared.FieldError{Path: path, Code: "pages.label_empty"})
		case utf8.RuneCountInString(label) > MaxLabelLength:
			return nil, shared.ErrValidation.WithDetail("pages.label_too_long").
				WithParams(map[string]string{"maximum": strconv.Itoa(MaxLabelLength)}).
				WithFields(shared.FieldError{Path: path, Code: "pages.label_too_long"})
		case strings.IndexFunc(label, notLabelRune) >= 0:
			return nil, shared.ErrValidation.WithDetail("pages.label_malformed").
				WithFields(shared.FieldError{Path: path, Code: "pages.label_malformed"})
		}
		if !slices.Contains(out, label) {
			out = append(out, label)
		}
	}
	if len(out) > MaxPageLabels {
		return nil, shared.ErrValidation.WithDetail("pages.too_many_labels").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxPageLabels)}).
			WithFields(shared.FieldError{Path: "/labels", Code: "pages.too_many_labels"})
	}
	slices.Sort(out)
	return out, nil
}

// Label answers one label in its written form, for a filter.
func Label(raw string) (string, error) {
	labels, err := Labels([]string{raw})
	if err != nil {
		return "", err
	}
	return labels[0], nil
}

func notLabelRune(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '.'
}
