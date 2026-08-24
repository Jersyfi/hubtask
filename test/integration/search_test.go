// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// The acceptance of C-08, against a real database. Every sentence of the task that is about
// *finding* something is here, because nothing below PostgreSQL can answer it: which lexemes a
// German document holds, whether a trigram index serves a substring of a Japanese title, and what
// `ts_rank_cd` thinks a title is worth against a note.

// searchFixture writes one collection of entries in four languages, and answers the collection and
// the hub above it.
type searchFixture struct {
	hub, collection shared.ID
	// elsewhere is a second collection under the same hub, which is what a scoped search has to
	// leave out and an unscoped one has to reach. Its one entry carries a word of its own, because
	// the suite shares a database: an unanchored search spans every fixture every other test in
	// this file wrote, and "the entry beside it" has to be one entry rather than all of them.
	elsewhere     shared.ID
	elsewhereWord string
}

func newSearchFixture(ctx context.Context, t *testing.T) searchFixture {
	t.Helper()

	hub, collection := hubWithCollection(ctx, t, tenantA, authorA)
	built := searchFixture{hub: hub, collection: collection}

	// Each entry is one language, and each one is a question the task asks. The notes on the first
	// carry the word the ranking test looks for, so that "in a title" and "in a note" are the same
	// word in two places.
	seed := []struct {
		title, notes, language string
		into                   shared.ID
	}{
		{"Hausaufgabenbetreuung", "Bäume gießen im Hof", "de-AT", collection},
		{"Running the numbers", "The quarterly report is due", "en", collection},
		{"会議の議事録", "", "ja", collection},
		{"Quarterly report", "", "", collection},
		{"Der Bericht über die Bäume", "", "de", collection},
	}

	previous := ""
	items := itemRepo()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, entry := range seed {
			key, err := service.OrderKeyAfter(previous)
			if err != nil {
				return err
			}
			previous = key

			task := taskIn(tenantA, authorA, entry.into, freshID(t), entry.title, key)
			task.Notes, task.ContentLanguage = entry.notes, entry.language
			if err := items.Insert(ctx, task); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the search fixture: %v", err)
	}

	// The second collection, with one entry nothing else in this file mentions.
	built.elsewhere, built.elsewhereWord = collectionBeside(ctx, t, tenantA, authorA, hub), shortSuffix(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return items.Insert(ctx, taskIn(
			tenantA, authorA, built.elsewhere, freshID(t), "Report "+built.elsewhereWord, "a0"))
	}); err != nil {
		t.Fatalf("seeding the second collection: %v", err)
	}
	return built
}

// found runs one search as the use case runs it, and answers the titles in the order they ranked.
func found(
	ctx context.Context, t *testing.T, tenant shared.ID, search repository.TextSearch,
) []string {
	t.Helper()

	page := searched(ctx, t, tenant, search)
	titles := make([]string, 0, len(page.Hits))
	for _, hit := range page.Hits {
		titles = append(titles, hit.Item.Title)
	}
	return titles
}

func searched(
	ctx context.Context, t *testing.T, tenant shared.ID, search repository.TextSearch,
) repository.ItemHitPage {
	t.Helper()

	if search.Request.Size == 0 {
		search.Request.Size = 50
	}

	var page repository.ItemHitPage
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		page, err = itemRepo().Search(ctx, search)
		return err
	}); err != nil {
		t.Fatalf("searching: %v", err)
	}
	return page
}

// searchIn builds a search over one collection.
func searchWithin(collection shared.ID, words, language string) repository.TextSearch {
	return repository.TextSearch{
		Anchor: repository.Anchor{
			Kind: repository.AnchorCollection, CollectionID: collection, IncludeDescendants: true,
		},
		Request: view.Search{Words: words, Language: language, Size: 50},
	}
}

// The first acceptance sentence: a German query finds a compound word. The query is the plural of
// a compound noun, which `simple` - what the generated column indexed everything as - matches
// nothing at all for.
func TestAGermanQueryFindsACompoundWord(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	titles := found(ctx, t, tenantA, searchWithin(f.collection, "Hausaufgabenbetreuungen", "de-AT"))
	if len(titles) != 1 || titles[0] != "Hausaufgabenbetreuung" {
		t.Errorf("a German searcher found %v", titles)
	}

	// And the umlaut in the notes of that entry, from its stem: the German configuration folds
	// `Bäume` to `baum`, which is a match no exact-word index can make.
	titles = found(ctx, t, tenantA, searchWithin(f.collection, "Baum", "de"))
	if len(titles) != 2 {
		t.Errorf("a search for the stem found %v, want both German entries", titles)
	}
}

// The second: an English query finds a stemmed form.
func TestAnEnglishQueryFindsAStemmedForm(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	titles := found(ctx, t, tenantA, searchWithin(f.collection, "run", "en"))
	if len(titles) != 1 || titles[0] != "Running the numbers" {
		t.Errorf("a search for the stem found %v", titles)
	}
}

// The third: a CJK query matches a substring through the trigram index.
//
// It is the branch of the statement that exists for scripts without word boundaries. A run of such
// characters is one token, so the tsquery half of the predicate cannot match part of it - which
// the second half of this test proves rather than assumes, by asking the same question of an entry
// whose script does have boundaries.
func TestACJKQueryMatchesASubstring(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	titles := found(ctx, t, tenantA, searchWithin(f.collection, "議事", "ja"))
	if len(titles) != 1 || titles[0] != "会議の議事録" {
		t.Errorf("a Japanese searcher found %v", titles)
	}

	// A Latin-script query is not given the substring branch, so a search for "report" does not
	// also drag in every entry that merely contains those letters somewhere.
	titles = found(ctx, t, tenantA, searchWithin(f.collection, "quarterl", "en"))
	if len(titles) != 0 {
		t.Errorf("a partial word matched %v - the substring branch is not confined to the "+
			"scripts that need it", titles)
	}
}

// The ranking: a hit in a title outranks one buried in a note. The document carries the weights
// (migration 0019), so `ts_rank_cd` answers it without the statement knowing which column a lexeme
// came from.
func TestATitleOutranksANote(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	page := searched(ctx, t, tenantA, searchWithin(f.collection, "quarterly", "en"))
	if len(page.Hits) != 2 {
		t.Fatalf("the search found %d entries, want the title and the note", len(page.Hits))
	}
	if page.Hits[0].Item.Title != "Quarterly report" {
		t.Errorf("the first hit is %q, want the entry with the word in its title",
			page.Hits[0].Item.Title)
	}
	if !(page.Hits[0].Rank > page.Hits[1].Rank) {
		t.Errorf("the ranks are %v and %v, want the title's to be higher",
			page.Hits[0].Rank, page.Hits[1].Rank)
	}
}

// Every hit carries the hub its collection sits under, which is what the use case builds the
// permission path from. Read back afterwards it would be a query per collection in the page.
func TestAHitCarriesTheHubItSitsUnder(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	page := searched(ctx, t, tenantA, searchWithin(f.collection, "quarterly", "en"))
	for _, hit := range page.Hits {
		if hit.HubID != f.hub {
			t.Errorf("%q says its hub is %s, want %s", hit.Item.Title, hit.HubID, f.hub)
		}
	}
}

// The scope narrows where the search looks: a collection sees its own entries, the tenant anchor
// sees the ones in the collection beside it as well.
func TestTheScopeDecidesWhereASearchLooks(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	within := found(ctx, t, tenantA, searchWithin(f.collection, f.elsewhereWord, "en"))
	if len(within) != 0 {
		t.Errorf("a search in one collection found %v in another", within)
	}

	everywhere := found(ctx, t, tenantA, repository.TextSearch{
		Anchor:  repository.Anchor{Kind: repository.AnchorTenant},
		Request: view.Search{Words: f.elsewhereWord, Language: "en", Size: 50},
	})
	if len(everywhere) != 1 || everywhere[0] != "Report "+f.elsewhereWord {
		t.Errorf("an unanchored search found %v", everywhere)
	}
}

// The narrowing the use case decides reaches the statement rather than the page: filtered
// afterwards, a page would come back short and its cursor would skip (C-04).
func TestASearchIsRestrictedToTheEntriesTheCallerMaySee(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	all := searched(ctx, t, tenantA, searchWithin(f.collection, "quarterly", "en"))
	if len(all.Hits) != 2 {
		t.Fatalf("the search found %d entries, want two to choose between", len(all.Hits))
	}

	restricted := searchWithin(f.collection, "quarterly", "en")
	restricted.RestrictTo = []shared.ID{all.Hits[1].Item.ID}

	titles := found(ctx, t, tenantA, restricted)
	if len(titles) != 1 || titles[0] != all.Hits[1].Item.Title {
		t.Errorf("the restricted search found %v, want only the entry it was restricted to", titles)
	}
}

// The walk: a page of one, continued by its cursor, reaches the second hit and stops. The keyset is
// over the rank, so this is the one walk in the product whose boundary is computed rather than
// stored - and a boundary that lost a digit on the way through the cursor would repeat a row or
// skip one.
func TestASearchWalksItsPagesInRankOrder(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	first := searchWithin(f.collection, "quarterly", "en")
	first.Request.Size = 1

	page := searched(ctx, t, tenantA, first)
	if len(page.Hits) != 1 || !page.Info.HasMore || page.Info.NextCursor == "" {
		t.Fatalf("the first page is %+v", page.Info)
	}

	next := first
	next.Request.Cursor = page.Info.NextCursor
	second := searched(ctx, t, tenantA, next)

	if len(second.Hits) != 1 {
		t.Fatalf("the second page holds %d hits", len(second.Hits))
	}
	if second.Hits[0].Item.ID == page.Hits[0].Item.ID {
		t.Errorf("the walk repeated %q", second.Hits[0].Item.Title)
	}
	if second.Info.HasMore {
		t.Errorf("the walk did not end: %+v", second.Info)
	}
}

// A trashed or archived entry is out of a search unless it is asked for, which is what makes a
// plain search mean "the work that is live".
func TestASearchLeavesTheTrashAndTheArchiveOut(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	page := searched(ctx, t, tenantA, searchWithin(f.collection, "quarterly", "en"))
	archived, _, err := page.Hits[0].Item.Archived(created)
	if err != nil {
		t.Fatalf("archiving: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetArchived(ctx, archived, page.Hits[0].Item.Version)
	}); err != nil {
		t.Fatalf("archiving: %v", err)
	}

	if titles := found(ctx, t, tenantA, searchWithin(f.collection, "quarterly", "en")); len(titles) != 1 {
		t.Errorf("an archived entry is still in the answer: %v", titles)
	}

	widened := searchWithin(f.collection, "quarterly", "en")
	widened.Request.IncludeArchived = true
	if titles := found(ctx, t, tenantA, widened); len(titles) != 2 {
		t.Errorf("include_archived answered %v, want both", titles)
	}
}

// The cross-tenant negative test the new repository method owes (gate SG-3, security.md §6).
//
// The search is the second place where the statement is assembled at run time, and the first that
// is unanchored: `AnchorTenant` writes no identifier at all, so what keeps tenant B out of tenant
// A's entries is the transaction the caller opened and nothing else (ADR-0010).
func TestASearchNeverCrossesTheTenantBoundary(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	for _, search := range []struct {
		name   string
		search repository.TextSearch
	}{
		{"through the collection", searchWithin(f.collection, "quarterly", "en")},
		{"through the hub", repository.TextSearch{
			Anchor: repository.Anchor{
				Kind: repository.AnchorHub, HubID: f.hub, IncludeDescendants: true,
			},
			Request: view.Search{Words: "quarterly", Language: "en", Size: 50},
		}},
		{"unanchored", repository.TextSearch{
			Anchor:  repository.Anchor{Kind: repository.AnchorTenant},
			Request: view.Search{Words: "quarterly", Language: "en", Size: 50},
		}},
		{"unanchored, with everything widened", repository.TextSearch{
			Anchor: repository.Anchor{Kind: repository.AnchorTenant},
			Request: view.Search{
				Words: "quarterly", Language: "en", Size: 50,
				IncludeArchived: true, IncludeTrashed: true,
			},
		}},
		{"through the substring branch, which reads the columns rather than the index",
			repository.TextSearch{
				Anchor:  repository.Anchor{Kind: repository.AnchorTenant},
				Request: view.Search{Words: "議事", Language: "ja", Size: 50},
			}},
	} {
		t.Run(search.name, func(t *testing.T) {
			if titles := found(ctx, t, tenantB, search.search); len(titles) != 0 {
				t.Errorf("tenant B read %v out of tenant A", titles)
			}
		})
	}
}

// A language this installation has no configuration for is searched word by word rather than
// refused: `hubtask_text_config` answers `simple` for it, and a search is not where somebody
// discovers that PostgreSQL was built without Welsh (ADR-0034).
func TestASearchInALanguageWithNoConfigurationStillAnswers(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	titles := found(ctx, t, tenantA, searchWithin(f.collection, "quarterly", "cy"))
	if len(titles) == 0 {
		t.Error("a Welsh searcher found nothing at all, rather than the exact words")
	}
}

// The words are a value, whatever they look like. `%` is a wildcard to LIKE and nothing to a
// person typing it, and the substring branch escapes it in the *value* rather than dropping it -
// which is what keeps a search for "50%" from quietly becoming a search for everything.
func TestTheWildcardsOfASearchAreEscapedRatherThanHonoured(t *testing.T) {
	ctx := context.Background()
	f := newSearchFixture(ctx, t)

	// A Japanese character puts the search on the substring branch; the wildcard rides along.
	if titles := found(ctx, t, tenantA, searchWithin(f.collection, "議%", "ja")); len(titles) != 0 {
		t.Errorf("a wildcard matched %v", titles)
	}
}

// collectionBeside writes a second collection under a hub the fixture already has.
func collectionBeside(ctx context.Context, t *testing.T, tenant, author, hub shared.ID) shared.ID {
	t.Helper()

	id := freshID(t)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return containerRepo().Insert(ctx,
			collectionIn(tenant, author, id, hub, "Beside it "+id.String()[:8], "a1"))
	}); err != nil {
		t.Fatalf("seeding a second collection: %v", err)
	}
	return id
}
