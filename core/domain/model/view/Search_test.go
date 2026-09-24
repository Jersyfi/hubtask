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

// Nothing to look for and nothing to narrow by is the one request a search refuses. Which of the
// two is missing is Search.Validate's question rather than the parser's, because only the whole
// request can see whether a filter came with the words (ADR-0064).
func TestASearchNeedsWordsOrAFilter(t *testing.T) {
	for _, raw := range []string{"", "   ", "\t\n"} {
		words, err := ParseSearch(raw, "/q")
		if err != nil || words != "" {
			t.Fatalf("the parser refused %q on its own: %q, %v", raw, words, err)
		}

		err = Search{Words: words}.Validate("")

		var domainErr *shared.Error
		if !errors.As(err, &domainErr) || domainErr.DetailCode != "search.words_required" {
			t.Errorf("a search for %q answered %v", raw, err)
		}
	}
}

// And a filter alone is a search: a work list, ordered rather than ranked.
func TestAFilterIsEnoughOnItsOwn(t *testing.T) {
	filter := &Node{Op: OpEq, Field: Field{Name: FieldIsCompleted, Kind: KindBool}, Values: []Value{{Kind: KindBool, Bool: false}}}
	if err := (Search{Filter: filter}).Validate(""); err != nil {
		t.Fatalf("a filtered search with no words was refused: %v", err)
	}
	if (Search{Filter: filter}).IsRanked() {
		t.Error("a search with no words claims a ranking")
	}
}

// A sort beside words is refused rather than ignored: with words there is a ranking and it is the
// ranking, so an order asked for beside them is an order the answer cannot be in.
func TestASortBesideWordsIsRefused(t *testing.T) {
	err := Search{Words: "tiles", Sort: DefaultSearchSort()}.Validate("")

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "search.sort_with_words" {
		t.Errorf("a sort beside words answered %v", err)
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

// The mode: two values, an empty one that means the default, and everything else refused. The
// refusal is the point - a client asking for a mode this server does not offer is a client that
// believes it will get something.
// The last word is the one that may still be being typed, and four things say it is not.
func TestTheLastWordIsTakenAsABeginningUnlessItIsFinished(t *testing.T) {
	for _, each := range []struct {
		name, words, head, prefix string
	}{
		{"one word, still being typed", "Ann", "", "Ann"},
		{"the finished ones, and the last", "Momente Chall", "Momente", "Chall"},
		{"a single letter is a beginning too", "A", "", "A"},

		// The four that finish a word.
		{"a trailing space is somebody saying they are done", "Anna ", "Anna ", ""},
		{"a phrase is a phrase", `"Anna's moments"`, `"Anna's moments"`, ""},
		{"the beginning of an exclusion would exclude too much", "moments -Ann", "moments -Ann", ""},
		{"or is the grammar's word, not one to find", "Anna or", "Anna or", ""},

		// And anything a text search parser would not keep inside a token, because the prefix is
		// handed to a function that has operators of its own.
		{"an apostrophe", "Anna's", "Anna's", ""},
		{"a colon", "Anna:", "Anna:", ""},
		{"an operator", "Anna&Momente", "Anna&Momente", ""},
		{"a digit is a word character", "Q3", "", "Q3"},
		{"a mark is part of a letter", "Bäum", "", "Bäum"},

		// A script without word boundaries has the substring branch already.
		{"Japanese is left to the trigram index", "会議", "会議", ""},

		{"nothing typed", "", "", ""},
	} {
		t.Run(each.name, func(t *testing.T) {
			head, prefix := Search{Words: each.words}.PrefixTerm()
			if head != each.head || prefix != each.prefix {
				t.Errorf("%q split into (%q, %q), want (%q, %q)", each.words, head, prefix, each.head, each.prefix)
			}
		})
	}
}

func TestParseSearchModeOffersTwoAndRefusesTheRest(t *testing.T) {
	for _, c := range []struct {
		raw     string
		want    SearchMode
		refused bool
	}{
		{raw: "", want: SearchAuto},
		{raw: "AUTO", want: SearchAuto},
		{raw: "LEXICAL", want: SearchLexical},
		{raw: "SEMANTIC", refused: true},
		{raw: "auto", refused: true},
		{raw: "hybrid", refused: true},
	} {
		mode, err := ParseSearchMode(c.raw, "/mode")
		if c.refused {
			if err == nil {
				t.Errorf("%q was accepted as %q", c.raw, mode)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q was refused: %v", c.raw, err)
		}
		if mode != c.want {
			t.Errorf("%q read as %q, want %q", c.raw, mode, c.want)
		}
	}
}

// What the mode decides, in one line: whether the words may also be asked about by meaning. The
// zero value has to answer yes, or a caller that never set the field would silently lose half the
// search.
func TestOnlyTheLexicalModeRefusesMeaning(t *testing.T) {
	for mode, want := range map[SearchMode]bool{
		"": true, SearchAuto: true, SearchLexical: false,
	} {
		if got := mode.Semantic(); got != want {
			t.Errorf("%q.Semantic() = %v, want %v", mode, got, want)
		}
	}
}
