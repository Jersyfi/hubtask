// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// What MATCHES reads changed with C-08: the document a trigger maintains under the entry's own
// language, rather than a generated column that was `simple` for everybody (ADR-0034).
//
// Two languages are in play at once and they are different questions - the entry's, which decided
// how it was indexed, and the searcher's, which decides how the words are read. These tests are
// where the pair is held to that, because nothing below the database can answer it.

// searchable writes three entries into a collection of their own: one German, one English, and one
// that states no language at all.
func searchableItems(ctx context.Context, t *testing.T) shared.ID {
	t.Helper()

	collection := collectionFor(ctx, t, tenantA, authorA)
	items := itemRepo()
	previous := ""

	seed := []struct {
		title, language string
	}{
		{"Hausaufgabenbetreuung", "de-AT"},
		{"Running the numbers", "en"},
		{"Quarterly report", ""},
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, entry := range seed {
			key, err := service.OrderKeyAfter(previous)
			if err != nil {
				return err
			}
			previous = key

			task := taskIn(tenantA, authorA, collection, freshID(t), entry.title, key)
			task.ContentLanguage = entry.language
			if err := items.Insert(ctx, task); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	return collection
}

// matching runs one MATCHES query as one searcher, and answers the titles it found.
func matching(
	ctx context.Context, t *testing.T, collection shared.ID, words, language string,
) []string {
	t.Helper()

	search := searchIn(t, collection, map[string]any{
		"field": "text", "op": "MATCHES", "value": words,
	}, view.Spec{})
	search.Language = language

	return titlesOf(queried(ctx, t, tenantA, search).Items)
}

// The point of the whole migration, asked through the grammar a client uses. An inflected German
// query finds the entry the German stemmer indexed; the `simple` configuration the generated column
// used matched only the word as it was written, which is what the control below is for.
func TestMatchesReadsTheEntrysOwnConfiguration(t *testing.T) {
	ctx := context.Background()
	collection := searchableItems(ctx, t)

	found := matching(ctx, t, collection, "Hausaufgabenbetreuungen", "de-AT")
	if len(found) != 1 || found[0] != "Hausaufgabenbetreuung" {
		t.Errorf("a German searcher found %v, want the German entry", found)
	}

	if found := matching(ctx, t, collection, "running", "en"); len(found) != 1 ||
		found[0] != "Running the numbers" {
		t.Errorf("an English searcher found %v, want the English entry", found)
	}
}

// The searcher's configuration is the searcher's, and this is what says so: the same words, the
// same entry, a different reader - and no match, because the English parser produces a lexeme the
// German document never held. It is also the negative half of the test above: without it, "the
// German query worked" could as well mean "everything matches everything".
func TestTheSearchersLanguageDecidesHowTheWordsAreRead(t *testing.T) {
	ctx := context.Background()
	collection := searchableItems(ctx, t)

	if found := matching(ctx, t, collection, "Hausaufgabenbetreuungen", "en"); len(found) != 0 {
		t.Errorf("an English searcher found %v with an inflected German query", found)
	}
}

// The `simple` branch, and why the predicate has two. An entry that states no language is indexed
// word by word, so the searcher's stemmer asks for a lexeme its document cannot hold - and without
// the second branch, an entry nobody declared a language for would be unfindable by anyone.
func TestAnEntryThatStatesNoLanguageIsStillFound(t *testing.T) {
	ctx := context.Background()
	collection := searchableItems(ctx, t)

	for _, language := range []string{"de-AT", "en", ""} {
		found := matching(ctx, t, collection, "quarterly", language)
		if len(found) != 1 || found[0] != "Quarterly report" {
			t.Errorf("a searcher reading %q found %v, want the undeclared entry", language, found)
		}
	}
}

// A tag no installation has a configuration for resolves to `simple` in the database rather than
// failing the query. A search is not where somebody discovers that PostgreSQL was built without
// Welsh (ADR-0034, hubtask_text_config).
func TestALanguageWithNoConfigurationSearchesRatherThanFails(t *testing.T) {
	ctx := context.Background()
	collection := searchableItems(ctx, t)

	found := matching(ctx, t, collection, "quarterly", "cy")
	if len(found) != 1 || found[0] != "Quarterly report" {
		t.Errorf("a Welsh searcher found %v, want the entry matched word by word", found)
	}
}
