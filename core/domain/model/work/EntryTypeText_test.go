// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-SVC-05 check 7: a reader reads a type's words in their language where the type says them
// there - the exact tag, then its language alone - and the default text otherwise, field by field.
func TestATypeIsReadInTheReadersLanguage(t *testing.T) {
	in := bugType
	in.Description = "Something is broken"
	in.Translations = map[string]work.EntryTypeText{
		"de":    {Name: "Störung", Description: "Etwas ist kaputt"},
		"pt-BR": {Name: "Defeito"},
	}
	entryType := newEntryType(t, in)
	cases := []struct {
		locale, name, description string
	}{
		{"de", "Störung", "Etwas ist kaputt"},
		{"de-AT", "Störung", "Etwas ist kaputt"},
		{"pt-BR", "Defeito", "Something is broken"},
		{"pt", "Bug", "Something is broken"},
		{"fr", "Bug", "Something is broken"},
		{"", "Bug", "Something is broken"},
	}
	for _, tc := range cases {
		t.Run(tc.locale, func(t *testing.T) {
			got := entryType.TextIn(tc.locale)
			if got.Name != tc.name || got.Description != tc.description {
				t.Errorf("read %+v, want %q / %q", got, tc.name, tc.description)
			}
		})
	}
}

func TestATypesTextsAreHeldToTheirRules(t *testing.T) {
	many := map[string]work.EntryTypeText{}
	for _, tag := range strings.Fields("aa ab ae af ak am an ar as av ay az ba be bg bi bm bn bo br bs") {
		many[tag] = work.EntryTypeText{Name: "x"}
	}
	cases := []struct {
		name   string
		change func(*work.NewEntryTypeInput)
		code   string
	}{
		{"a description too long", func(in *work.NewEntryTypeInput) { in.Description = strings.Repeat("a", 501) }, "entry_types.description_too_long"},
		{"a tag that is no language", func(in *work.NewEntryTypeInput) {
			in.Translations = map[string]work.EntryTypeText{"German": {Name: "Störung"}}
		}, "entry_types.language_malformed"},
		{"a name on two lines", func(in *work.NewEntryTypeInput) {
			in.Translations = map[string]work.EntryTypeText{"de": {Name: "Stö\nrung"}}
		}, "entry_types.name_malformed"},
		{"too many languages", func(in *work.NewEntryTypeInput) { in.Translations = many }, "entry_types.too_many_languages"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := bugType
			tc.change(&in)
			_, err := work.NewEntryType(in)
			if !errors.Is(err, shared.ErrValidation) || detailOf(err) != tc.code {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
		})
	}
}

// A language whose texts are all empty is dropped rather than kept as a language reading the
// defaults; zh-Hans is a tag of the 1.0 set and is taken.
func TestATypesEmptyLanguageIsDropped(t *testing.T) {
	in := bugType
	in.Translations = map[string]work.EntryTypeText{"de": {Name: "  "}, "zh-Hans": {Name: "缺陷"}}
	entryType := newEntryType(t, in)
	if _, kept := entryType.Translations["de"]; kept || entryType.Translations["zh-Hans"].Name != "缺陷" {
		t.Errorf("translations %+v", entryType.Translations)
	}
}

// The texts are a person's words: a change of them is recorded under its own field, so the trail
// can hold it as sensitive (rule 10).
func TestChangingATypesTextsIsOneChange(t *testing.T) {
	entryType := newEntryType(t, bugType)
	description := "Something is broken"
	translations := map[string]work.EntryTypeText{"de": {Name: "Störung"}}
	next, changes, err := entryType.Changed(work.EntryTypeChange{
		Description: &description, Translations: &translations,
	}, entryTypeNow, bugType.Text)
	if err != nil {
		t.Fatal(err)
	}
	fields := []string{}
	for _, change := range changes {
		fields = append(fields, change.Field)
	}
	if strings.Join(fields, ",") != "description,translations" || next.TextIn("de").Name != "Störung" {
		t.Errorf("changes %v, next %+v", fields, next)
	}
}
