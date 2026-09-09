// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The acceptance of J-10, against a real database - which is the only place it can be asked. What a
// vector is near, what `ts_rank_cd` thinks a title is worth beside it, and whether the two halves
// come back as one ordered page are all PostgreSQL's answers, and a fake would only ever agree with
// whatever this file assumed.

// embeddingDimensions is the width migration 0075 fixed the column at. A vector of any other length
// is refused by the database, which is the point of fixing it: an index is built for one geometry.
const embeddingDimensions = 1536

// axes hands each fixture a direction of its own.
//
// The suite shares one database and the semantic branch reads the whole workspace's store, so two
// fixtures pointing at the same axis would be each other's nearest neighbours - and a test would
// then be measuring the fixture the test before it wrote. A direction per fixture is the vector
// equivalent of the ring keys the rest of this package uses: assert by identity, and name something
// nobody else names.
var axes atomic.Int32

func freshAxis() int { return int(axes.Add(1)) }

// meaningOf builds a unit vector pointing at one axis, which is all the geometry these tests need:
// two vectors on the same axis are identical, two on different axes are orthogonal, and a mixture
// sits at a similarity this file can state exactly.
func meaningOf(axis int) []float32 {
	vector := make([]float32, embeddingDimensions)
	vector[axis] = 1
	return vector
}

// meaningNear builds a vector whose cosine similarity with meaningOf(axis) is `similarity`, by
// putting the rest of the length on an axis nothing else uses.
func meaningNear(axis int, similarity float32) []float32 {
	vector := make([]float32, embeddingDimensions)
	vector[axis] = similarity
	vector[embeddingDimensions-1] = float32(sqrt(float64(1 - similarity*similarity)))
	return vector
}

func sqrt(f float64) float64 {
	// Newton's method, so this file needs no import for one line of arithmetic.
	if f <= 0 {
		return 0
	}
	guess := f
	for range 40 {
		guess = (guess + f/guess) / 2
	}
	return guess
}

// hybridFixture is one collection holding three entries: one that carries the searched word, one
// that carries the *meaning* and shares no word with it, and one that is neither.
type hybridFixture struct {
	collection              shared.ID
	word                    string
	named, meant, unrelated shared.ID
	// subject is the direction this fixture's query points at, and `elsewhere` a direction nothing
	// in it is about.
	subject, elsewhere int
}

func newHybridFixture(ctx context.Context, t *testing.T) hybridFixture {
	t.Helper()
	requirePgvector(ctx, t)
	seedContainerTenants(ctx, t)

	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	built := hybridFixture{
		collection: collection, word: shortSuffix(t),
		named: freshID(t), meant: freshID(t), unrelated: freshID(t),
		subject: freshAxis(), elsewhere: freshAxis(),
	}

	// The titles are the whole experiment. Only the first carries the searched word; the second
	// shares no character of it, and is found - if it is found - because its vector says what it
	// is about.
	seed := []struct {
		id     shared.ID
		title  string
		vector []float32
	}{
		{built.named, "Invoice " + built.word, meaningOf(freshAxis())},
		{built.meant, "Zwiebelkuchen für den Herbstmarkt", meaningNear(built.subject, 0.9)},
		{built.unrelated, "Fahrradschlauch flicken", meaningOf(freshAxis())},
	}

	previous := ""
	items := itemRepo()
	embeddings := postgres.NewEmbeddingRepository()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, entry := range seed {
			key, err := service.OrderKeyAfter(previous)
			if err != nil {
				return err
			}
			previous = key

			task := taskIn(tenantA, authorA, collection, entry.id, entry.title, key)
			task.ContentLanguage = "de"
			if err := items.Insert(ctx, task); err != nil {
				return err
			}
			if err := embeddings.Store(ctx, repository.StoredEmbedding{
				ItemID: entry.id, Model: "test-embed-1", Vector: entry.vector,
				SourceDigest: suggestion.Digest(entry.title, ""),
				UpdatedAt:    time.Now().UTC(),
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the hybrid fixture: %v", err)
	}
	return built
}

func requirePgvector(ctx context.Context, t *testing.T) {
	t.Helper()
	var present bool
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT to_regclass('public.item_embedding') IS NOT NULL`).Scan(&present); err != nil {
		t.Fatalf("asking for the embedding store: %v", err)
	}
	if !present {
		t.Skip("this database has no pgvector, so there is no semantic half to prove (ADR-0050)")
	}
}

func hybridSearch(f hybridFixture, meaning []float32) repository.TextSearch {
	return repository.TextSearch{
		Anchor: repository.Anchor{
			Kind: repository.AnchorCollection, CollectionID: f.collection, IncludeDescendants: true,
		},
		Request: view.Search{Words: f.word, Language: "de", Size: 50},
		Meaning: meaning,
	}
}

// The sentence the whole task is for: an entry that shares no word with the query is found, and the
// entry that carries the word is still first. Both halves matter - a ranking that let meaning win
// would stop finding an exact identifier, which is the regression F3-18 had to soften in the client.
func TestASemanticHitIsFoundAndTheExactWordStillWins(t *testing.T) {
	ctx := context.Background()
	f := newHybridFixture(ctx, t)

	page := searched(ctx, t, tenantA, hybridSearch(f, meaningOf(f.subject)))

	ids := make([]shared.ID, 0, len(page.Hits))
	for _, hit := range page.Hits {
		ids = append(ids, hit.Item.ID)
	}
	if len(ids) != 2 {
		t.Fatalf("the search found %v, want the named entry and the meant one", hitTitles(page))
	}
	if ids[0] != f.named {
		t.Errorf("the page begins with %q, want the entry carrying the searched word",
			page.Hits[0].Item.Title)
	}
	if ids[1] != f.meant {
		t.Errorf("the second hit is %q, want the entry that shares no word with the query",
			page.Hits[1].Item.Title)
	}
	for _, hit := range page.Hits {
		if hit.Item.ID == f.unrelated {
			t.Error("an entry that is neither named nor meant was returned")
		}
	}
}

// The same search without a vector is the search this product had before J-10: the entry carrying
// the word, and nothing else. Which is what says the hit above came from the vector rather than
// from some accident of the German configuration.
func TestWithoutAVectorTheSearchFindsOnlyTheWords(t *testing.T) {
	ctx := context.Background()
	f := newHybridFixture(ctx, t)

	titles := found(ctx, t, tenantA, hybridSearch(f, nil))

	if len(titles) != 1 || !strings.Contains(titles[0], f.word) {
		t.Errorf("a lexical search found %v, want only the entry carrying the word", titles)
	}
}

// Near enough, and not merely nearest. In a small workspace everything is among the nearest
// neighbours, so a search bounded only by `LIMIT` would answer the whole collection for any query -
// which is a worse answer than none. The floor is what stops it.
func TestAnEntryThatIsMerelyTheNearestIsNotAHit(t *testing.T) {
	ctx := context.Background()
	f := newHybridFixture(ctx, t)

	// A direction nothing in the fixture points at: every entry is orthogonal to it or nearly so,
	// so every one of them is "the nearest" and none of them is near.
	page := searched(ctx, t, tenantA, hybridSearch(f, meaningOf(f.elsewhere)))

	for _, hit := range page.Hits {
		if hit.Item.ID != f.named {
			t.Errorf("%q came back for a query it is not about", hit.Item.Title)
		}
	}
}

// The walk still works with both halves in the rank: one page, one cursor, and a boundary that
// neither repeats a row nor skips one. The rank is now a sum rather than a `greatest`, and a cursor
// that lost a digit of it would do both.
func TestAHybridSearchStillWalksItsPagesOnce(t *testing.T) {
	ctx := context.Background()
	f := newHybridFixture(ctx, t)

	first := hybridSearch(f, meaningOf(f.subject))
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

// The tenant boundary, on the half that is new. A vector is content: an embedding of somebody's
// notes that another workspace could reach would be T-04 with a different verb, and the semantic
// branch is a subquery over the whole store rather than over the anchored rows - so it is exactly
// the shape that would leak if row level security were not carrying it.
func TestTheSemanticHalfNeverCrossesTheTenantBoundary(t *testing.T) {
	ctx := context.Background()
	f := newHybridFixture(ctx, t)

	// The same query, run by the workspace next door, unanchored so that nothing but row level
	// security is narrowing it.
	page := searched(ctx, t, tenantB, repository.TextSearch{
		Anchor:  repository.Anchor{Kind: repository.AnchorTenant},
		Request: view.Search{Words: f.word, Language: "de", Size: 50},
		Meaning: meaningOf(f.subject),
	})
	for _, hit := range page.Hits {
		if hit.Item.TenantID == tenantA {
			t.Errorf("the workspace next door found %q", hit.Item.Title)
		}
	}

	// And the write side: a vector for another workspace's entry is refused rather than stored
	// against it. The composite foreign key is what refuses it - an embedding belongs to an item
	// *of this workspace*, and the pair is what says so.
	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return postgres.NewEmbeddingRepository().Store(ctx, repository.StoredEmbedding{
			ItemID: f.named, Model: "test-embed-1", Vector: meaningOf(f.subject),
			SourceDigest: suggestion.Digest("Invoice", ""), UpdatedAt: time.Now().UTC(),
		})
	})
	if err == nil {
		t.Error("a workspace stored a vector against another workspace's entry")
	}
}

// What the pass reads: the entries whose vector is missing or made from text that has since moved.
// An entry that was embedded and has not changed is not owed one - which is what makes the job
// converge rather than embed the same batch for ever - and an entry whose title moves is owed one
// again without anything having told the pass so.
func TestOnlyTheEntriesWhoseTextMovedAreOwedAVector(t *testing.T) {
	ctx := context.Background()
	f := newHybridFixture(ctx, t)

	if owed := owedIn(ctx, t, "test-embed-1"); owedHolds(owed, f.named) {
		t.Error("an entry that was just embedded is owed another vector")
	}

	// The same entries under a different model are all owed one: vectors are only comparable
	// within one model's space, so a reconfiguration is a re-index.
	if owed := owedIn(ctx, t, "test-embed-2"); !owedHolds(owed, f.named) {
		t.Error("a change of model left the old vectors standing")
	}

	// And a title that moves owes one again, because the fingerprint the database computes no
	// longer matches the one the vector was stored against.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		item, err := itemRepo().Find(ctx, f.named)
		if err != nil {
			return err
		}
		expected := item.Version
		item.Title, item.Version, item.UpdatedAt = "Invoice, revised", expected+1, created
		return itemRepo().SetAttributes(ctx, item, expected)
	}); err != nil {
		t.Fatalf("renaming the entry: %v", err)
	}

	owed := owedIn(ctx, t, "test-embed-1")
	if !owedHolds(owed, f.named) {
		t.Error("an entry whose title moved is not owed a new vector")
	}
	for _, entry := range owed {
		if entry.ItemID == f.named && entry.Title != "Invoice, revised" {
			t.Errorf("the pass would embed %q, which is not what the entry says now", entry.Title)
		}
	}
}

func owedIn(ctx context.Context, t *testing.T, model string) []repository.OwedEmbedding {
	t.Helper()
	var owed []repository.OwedEmbedding
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		owed, err = postgres.NewEmbeddingRepository().Owed(ctx, model, 200)
		return err
	}); err != nil {
		t.Fatalf("reading what is owed: %v", err)
	}
	return owed
}

func owedHolds(owed []repository.OwedEmbedding, id shared.ID) bool {
	for _, entry := range owed {
		if entry.ItemID == id {
			return true
		}
	}
	return false
}

func hitTitles(page repository.ItemHitPage) []string {
	titles := make([]string, 0, len(page.Hits))
	for _, hit := range page.Hits {
		titles = append(titles, hit.Item.Title)
	}
	return titles
}
