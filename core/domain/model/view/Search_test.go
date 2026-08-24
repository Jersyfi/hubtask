// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestParseSearchTrimsAndNothingElse(t *testing.T) {
	// Case, stop words and inflection are the text search configuration's business, and a second
	// opinion here would be one taken in ignorance of the language the entries are written in.
	words, err := ParseSearch("  Quarterly Report or Bilanz  ", "/q")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if words != "Quarterly Report or Bilanz" {
		t.Errorf("the words became %q", words)
	}
}

func TestASearchNeedsWords(t *testing.T) {
	for _, raw := range []string{"", "   ", "\t\n"} {
		_, err := ParseSearch(raw, "/q")

		var domainErr *shared.Error
		if !errors.As(err, &domainErr) || domainErr.DetailCode != "search.words_required" {
			t.Errorf("a search for %q answered %v", raw, err)
		}
	}
}

func TestASearchIsBounded(t *testing.T) {
	_, err := ParseSearch(strings.Repeat("a", MaxSearchWordsLength+1), "/q")

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "search.words_too_long" {
		t.Fatalf("an over-long search answered %v", err)
	}
	if domainErr.Params["maximum"] != "200" {
		t.Errorf("the refusal does not say the bound: %v", domainErr.Params)
	}

	// The bound counts code points rather than bytes, so a search in a script whose characters take
	// three bytes each is not a third as long as one in Latin.
	if _, err := ParseSearch(strings.Repeat("議", MaxSearchWordsLength), "/q"); err != nil {
		t.Errorf("a search of exactly the maximum length was refused: %v", err)
	}
}

// Which scripts need the substring branch, and which do not. It is the one decision this type makes
// on its own, and getting it wrong is invisible: a Japanese search would simply find nothing.
func TestTheScriptsWithoutWordBoundariesAreRecognised(t *testing.T) {
	tests := []struct {
		name  string
		words string
		want  bool
	}{
		{"Japanese", "議事録", true},
		{"Chinese", "会议", true},
		{"Korean", "회의", true},
		{"Thai", "การประชุม", true},
		{"a product name inside a Japanese title", "会議 Hubtask", true},
		{"English", "quarterly report", false},
		{"German with umlauts", "Bäume gießen", false},
		{"Greek, which has spaces", "τριμηνιαία έκθεση", false},
		{"Russian, which has spaces", "квартальный отчёт", false},
		{"digits and punctuation", "50% - Q4", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := (Search{Words: test.words}).WithoutWordBoundaries(); got != test.want {
				t.Errorf("%q answered %v, want %v", test.words, got, test.want)
			}
		})
	}
}
