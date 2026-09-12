// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The constant that mirrors the migration is a number kept twice, and this is what makes the second
// copy honest (ADR-0054): the port says how wide the index is, migration 0075 built the column, and
// the two are compared against the migrated database rather than against each other's source.
func TestTheIndexIsAsWideAsThePortSays(t *testing.T) {
	ctx := context.Background()
	requirePgvector(ctx, t)

	var declared string
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT format_type(a.atttypid, a.atttypmod)
		  FROM pg_attribute a
		 WHERE a.attrelid = 'public.item_embedding'::regclass
		   AND a.attname = 'embedding'
		   AND NOT a.attisdropped`).Scan(&declared); err != nil {
		t.Fatalf("reading the column's type: %v", err)
	}
	if want := fmt.Sprintf("vector(%d)", repository.EmbeddingWidth); declared != want {
		t.Fatalf("item_embedding.embedding is %s and the port says %s: change the port, or "+
			"write the migration ADR-0054 says a wider index needs", declared, want)
	}
}

// narrowWidth is the width of the narrower of the two Ollama embedding models in common use, and
// the width no test in this repository had ever stored before ADR-0054: every fixture synthesised
// the index's own 1536.
const narrowWidth = 768

// narrowAxis hands out an axis a 768-wide vector can point at. The suite's counter is shared with
// the full-width fixtures, which is what keeps a narrow fixture and a wide one from being each
// other's nearest neighbours by accident - a narrow vector on axis k, padded, *is* the wide vector
// on axis k.
func narrowAxis(t *testing.T) int {
	t.Helper()
	axis := freshAxis()
	if axis >= narrowWidth-1 {
		t.Fatalf("the suite has used %d axes; this test needs one below %d", axis, narrowWidth-1)
	}
	return axis
}

func narrowOn(axis int) []float32 {
	vector := make([]float32, narrowWidth)
	vector[axis] = 1
	return vector
}

func narrowNear(axis int, similarity float32) []float32 {
	vector := make([]float32, narrowWidth)
	vector[axis] = similarity
	vector[narrowWidth-1] = float32(sqrt(float64(1 - similarity*similarity)))
	return vector
}

// The test that was missing: a vector narrower than the index is stored, and the entry it belongs
// to is found by a query of the same width - through the same padding on both sides - with the
// similarity the floor is compared against being the number the model produced (ADR-0054).
func TestANarrowerVectorIsStoredAndFoundAtTheSimilarityItWasGiven(t *testing.T) {
	ctx := context.Background()
	requirePgvector(ctx, t)
	seedContainerTenants(ctx, t)

	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	subject, twin, stranger := freshID(t), freshID(t), freshID(t)
	axis := narrowAxis(t)
	const model = "narrow-embed-768"

	items := itemRepo()
	embeddings := postgres.NewEmbeddingRepository()
	previous := ""
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, entry := range []struct {
			id     shared.ID
			title  string
			vector []float32
		}{
			{subject, "Renew the domain registration", narrowOn(axis)},
			{twin, "Renew the domain name before it lapses", narrowNear(axis, 0.95)},
			{stranger, "Buy oat milk", narrowOn(narrowAxis(t))},
		} {
			key, err := service.OrderKeyAfter(previous)
			if err != nil {
				return err
			}
			previous = key
			if err := items.Insert(ctx, taskIn(tenantA, authorA, collection, entry.id, entry.title, key)); err != nil {
				return err
			}
			if err := embeddings.Store(ctx, repository.StoredEmbedding{
				ItemID: entry.id, Model: model, Vector: entry.vector,
				SourceDigest: suggestion.Digest(entry.title, ""), UpdatedAt: time.Now().UTC(),
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("storing 768-wide vectors: %v", err)
	}

	var near repository.Nearby
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		// Three rows are a sequential scan; asking the planner to avoid one is what makes the
		// HNSW index answer for padded vectors rather than the operator alone.
		tx, err := postgres.FromContext(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
			return err
		}
		near, err = embeddings.Near(ctx, subject, 0.9, 10)
		return err
	}); err != nil {
		t.Fatalf("reading the neighbourhood: %v", err)
	}
	if !near.Embedded || near.Model != model {
		t.Fatalf("the subject came back as %+v", near)
	}
	if len(near.Candidates) != 1 || near.Candidates[0].ItemID != twin {
		t.Fatalf("the neighbours are %+v, want the twin alone", near.Candidates)
	}
	// The number the floor is compared against is the one the vector was built with: padding
	// changed neither the dot product nor either norm. float32 in, float8 out, so a hair's width.
	if got := near.Candidates[0].Similarity; got < 0.949 || got > 0.951 {
		t.Errorf("the similarity is %v, want the 0.95 the vector was built with", got)
	}

	// And the search's query vector goes through the same padding, so a 768-wide query finds the
	// 768-wide entry.
	page := searched(ctx, t, tenantA, repository.TextSearch{
		Anchor: repository.Anchor{
			Kind: repository.AnchorCollection, CollectionID: collection, IncludeDescendants: true,
		},
		Request: view.Search{Words: "zzz-nothing-carries-this", Language: "en", Size: 50},
		Meaning: narrowOn(axis),
	})
	hits := map[shared.ID]bool{}
	for _, hit := range page.Hits {
		hits[hit.Item.ID] = true
	}
	if !hits[subject] || !hits[twin] || hits[stranger] {
		t.Errorf("a 768-wide query found %v, want the subject and its twin and not the stranger",
			hitTitles(page))
	}
}

// A vector wider than the index is refused by the store before any statement runs, with the typed
// error and not a database one.
func TestAWiderVectorIsRefusedBeforeTheDatabaseSeesIt(t *testing.T) {
	ctx := context.Background()
	requirePgvector(ctx, t)
	seedContainerTenants(ctx, t)

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewEmbeddingRepository().Store(ctx, repository.StoredEmbedding{
			ItemID: freshID(t), Model: "wide-embed", Vector: make([]float32, repository.EmbeddingWidth+1),
			SourceDigest: suggestion.Digest("x", ""), UpdatedAt: time.Now().UTC(),
		})
	})
	if !repository.IsEmbeddingTooWide(err) {
		t.Fatalf("a %d-wide vector answered %v, want the typed refusal", repository.EmbeddingWidth+1, err)
	}
}
