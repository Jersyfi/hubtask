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
//
// And because it is that read, it is the one that may also be **narrowed** (ADR-0064). It takes
// the same filter tree Spec does - the same closed vocabulary, the same bounds, the same cost cap
// - and its words become optional beside one. What a filtered search with no words is, is a work
// list: there is nothing to rank, so it is ordered rather than ranked, and Sort is what orders it.
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
	// Filter narrows the hits, in Spec's grammar and with Spec's bounds. Nil is no narrowing.
	Filter *Node
	// Sort orders a search that has no words to rank. Meaningless with words and refused
	// there: with words there is a ranking and it *is* the ranking, so a sort beside them
	// would be a caller asking for an order the answer cannot be in.
	Sort []SortTerm
}

// IsRanked reports whether this search has words, and therefore a relevance to order by.
func (s Search) IsRanked() bool { return s.Words != "" }

// DefaultSearchSort is the order of a filtered search with no words: a work list, by when it
// is due, with the entries that have no date last. Completed by the identifier wherever it is
// used, as every ordering in this model is, so that a cursor is unambiguous.
func DefaultSearchSort() []SortTerm {
	return []SortTerm{{Field: Field{Name: FieldDueAt, Kind: KindTimestamp}}}
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
		// Empty is allowed *here*, and Search.Validate decides it - the one place that can see
		// whether a filter came with it (ADR-0064). Neither still means "everything", which is not
		// a question this API answers: ranking a whole workspace by how well it matches nothing
		// would be a list in an arbitrary order.
		return "", nil

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

// PrefixTerm splits the words into the ones that are finished and the one that may still be being
// typed: `Momente Chall` is `Momente` and the beginning `Chall`.
//
// **It exists because the bar answers while somebody types** (ADR-0066 decision 4). A tsquery
// compares whole lexemes, so `Ann` is not a worse match for *Anna* - it is no match at all, and
// `A` and `An` are English stop words besides, which a configuration drops entirely. A menu that
// shows nothing for the first four keystrokes of a five-letter name reads as a menu that does not
// work.
//
// Only the **last** word, and only when it is one plain word. Four things finish a word and are
// therefore matched whole:
//
//   - a trailing space - somebody who typed one has finished;
//   - a quote, because a phrase is a phrase;
//   - a leading minus, because the beginning of something to exclude excludes too much;
//   - `or`, which is the grammar's own word rather than one to find.
//
// Anything but a letter, a digit or a mark also finishes it. That is not tidiness: the prefix is
// handed to `to_tsquery`, whose own operators (`&`, `|`, `!`, `:`, brackets) would otherwise turn
// somebody's punctuation into a syntax error on a read - and it keeps the value a *word* rather
// than something that has to be escaped (rule 9, T-06).
//
// A script without word boundaries is left alone: its search is the substring branch already
// (WithoutWordBoundaries), and a prefix of a one-token run would be a second answer to the same
// question.
func (s Search) PrefixTerm() (head, prefix string) {
	if s.Words == "" || s.WithoutWordBoundaries() {
		return s.Words, ""
	}
	// A trailing space is the one signal that costs nothing to read and means exactly what it
	// looks like: this word is done.
	if trailing := strings.TrimRightFunc(s.Words, unicode.IsSpace); trailing != s.Words {
		return s.Words, ""
	}

	fields := strings.Fields(s.Words)
	if len(fields) == 0 {
		return s.Words, ""
	}
	last := fields[len(fields)-1]

	if strings.EqualFold(last, "or") || strings.HasPrefix(last, "-") || !isPlainWord(last) {
		return s.Words, ""
	}
	return strings.Join(fields[:len(fields)-1], " "), last
}

// isPlainWord reports whether every rune is one a text search parser would keep inside a token.
func isPlainWord(word string) bool {
	for _, r := range word {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r) {
			return false
		}
	}
	return word != ""
}

// Validate holds the parts of a search that are not the words.
func (s Search) Validate(path string) error {
	if s.Words == "" && s.Filter == nil {
		return fieldError(path+"/q", "search.words_required", nil)
	}
	// Refused rather than ignored: silent ignoring is what this project has a rule against, and a
	// caller who sent both asked for an order the answer cannot be in.
	if s.Words != "" && len(s.Sort) > 0 {
		return fieldError(path+"/sort", "search.sort_with_words", nil)
	}
	return nil
}
