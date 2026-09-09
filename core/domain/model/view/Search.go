// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Search is one full-text search: the words, where to look, and how much of the answer to return
// (C-08, domain-model.md §5).
//
// A sibling of Spec rather than a filter inside it, and the difference is what it answers with. A
// query says which entries satisfy a condition and returns them in an order somebody chose; a
// search says which entries are *about* something and returns them in the order the database
// thinks they are about it. Ranking is the whole of that difference, and it is not a sort a client
// could ask for - `ts_rank_cd` reads the same lexemes the match did.
//
// The scope is optional, which is the second difference. A query is anchored because an unanchored
// one is a question authorisation cannot answer in one step (Scope). A search is the one read
// where that is the question being asked - "where is this, anywhere" - so it is answered the way
// the trash is: read, then narrowed to what the actor may see (ListTrash, C-04).
type Search struct {
	// Words are what the caller is looking for, as they typed them. They reach the database as a
	// bound parameter and are parsed there, by the text search configuration the language names:
	// the syntax of `websearch_to_tsquery` - quoted phrases, `or`, a leading `-` - is therefore
	// the caller's to use, and nothing here interprets it.
	Words string
	// ContainerID narrows the search to one hub or one collection. Empty searches everything the
	// actor may see.
	ContainerID shared.ID
	// Language is the tag the words are read under - the searcher's, not the entries'. Filled in
	// by the use case from the request or the actor's locale (ADR-0034).
	Language string
	// IncludeArchived and IncludeTrashed widen what the search sees. Both default to false, which
	// is what makes a plain search mean "the work that is live".
	IncludeArchived bool
	IncludeTrashed  bool
	Cursor          string
	Size            int
	// Mode is how much of the search to use (J-10, ADR-0050). Empty means SearchAuto, so a caller
	// that predates the field - and a client that never sends it - keeps getting the whole search.
	Mode SearchMode
}

// SearchMode says whether a search may also ask what the words mean.
//
// Two values and not three. There is no SEMANTIC, because semantic search is optional four times
// over - the database may not carry pgvector, the workspace may have configured no provider or not
// consented to one, and the provider may not answer while somebody waits - and a mode the server
// cannot promise is a mode that would have to fail. AUTO uses what is there; LEXICAL is the caller
// saying it would rather not wait on anybody else's machine.
type SearchMode string

const (
	// SearchAuto searches by words and, where the installation has it, by meaning. The default.
	SearchAuto SearchMode = "AUTO"
	// SearchLexical searches by words only: no provider is asked, no budget is spent, nothing is
	// waited on. What an automation, an import or a type-ahead wants.
	SearchLexical SearchMode = "LEXICAL"
)

// Semantic reports whether this mode may ask what the words mean.
func (m SearchMode) Semantic() bool { return m != SearchLexical }

// ParseSearchMode reads the mode a caller sent, defaulting an empty one to AUTO.
func ParseSearchMode(raw string, path string) (SearchMode, error) {
	switch SearchMode(raw) {
	case "":
		return SearchAuto, nil
	case SearchAuto:
		return SearchAuto, nil
	case SearchLexical:
		return SearchLexical, nil
	}
	return "", fieldError(path, "search.mode_unknown", map[string]string{"value": raw})
}

// MaxSearchWordsLength bounds the query text.
//
// Long enough for a sentence somebody pasted, short enough that no request turns into a tsquery
// with a thousand terms - which is a plan the database has to build before it can decide the query
// matches nothing.
const MaxSearchWordsLength = 200

// ParseSearch reads a search request and refuses one that asks for nothing.
//
// The words are trimmed and nothing else. Lower-casing, stemming and stop word removal are the
// text search configuration's, and doing any of them here would be a second opinion about a
// language this layer does not know the entries are written in.
func ParseSearch(words string, path string) (string, error) {
	trimmed := strings.TrimSpace(words)

	switch {
	case trimmed == "":
		// An empty search is not "everything": that is what the query endpoint answers, with a
		// scope and an order somebody chose. Ranking a whole collection by how well it matches
		// nothing would be a list in an arbitrary order.
		return "", fieldError(path, "search.words_required", nil)

	case utf8.RuneCountInString(trimmed) > MaxSearchWordsLength:
		return "", fieldError(path, "search.words_too_long", map[string]string{
			"maximum": strconv.Itoa(MaxSearchWordsLength),
		})
	}
	return trimmed, nil
}

// WithoutWordBoundaries reports whether the words are written in a script that does not separate
// them with spaces - Han, Hiragana, Katakana, Hangul, Thai, Lao, Khmer, Myanmar.
//
// It decides one thing: whether the search also asks the trigram index (i18n-l10n.md §5). A text
// search parser splits on boundaries, so a run of such characters becomes one token, and a tsquery
// for part of that token matches nothing at all - not less well, nothing. For those scripts the
// substring match is not an optimisation of the search, it is the search.
//
// Asked of the *query* rather than of the entries, because it is the query that has to be matchable:
// an entry written in Japanese is found by a Japanese query through the trigram index, and by a
// Latin-script query - a product name in the same title - through the ordinary one.
//
// Any such character is enough. A search mixing scripts is exactly the case where leaving the
// substring branch out would silently drop half the question.
func (s Search) WithoutWordBoundaries() bool {
	for _, r := range s.Words {
		if unicode.In(r,
			unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul,
			unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar,
		) {
			return true
		}
	}
	return false
}

// Validate holds the parts of a search that are not the words.
func (s Search) Validate(path string) error {
	if s.Words == "" {
		return fieldError(path+"/q", "search.words_required", nil)
	}
	return nil
}
