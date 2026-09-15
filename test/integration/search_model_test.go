// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Between a reconfiguration and the end of the re-embedding pass the table holds two models'
// vectors, and two models' vectors are not comparable (ADR-0049 decision 4). The search reads only
// the rows of the model its query vector came from; an entry still embedded under the old name is
// found by its words alone until the pass reaches it (#568).
//
// Two entries, both pointing exactly where the query points, one embedded under each model. Before
// this, the old model's entry was a semantic hit for a query from the new one - a perfect match by
// arithmetic on numbers that mean nothing to each other.
func TestTheSearchReadsOnlyTheVectorsOfTheModelThatEmbeddedTheQuery(t *testing.T) {
	ctx := context.Background()
	requirePgvector(ctx, t)
	seedContainerTenants(ctx, t)

	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	current, stale := freshID(t), freshID(t)
	axis := freshAxis()
	const oldModel, newModel = "test-embed-old", "test-embed-new"

	items := itemRepo()
	embeddings := postgres.NewEmbeddingRepository()
	previous := ""
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, entry := range []struct {
			id    shared.ID
			title string
			model string
		}{
			// Neither title carries a searched word: whatever is found is found by its vector.
			{current, "Zwiebelkuchen für den Herbstmarkt", newModel},
			{stale, "Fahrradschlauch flicken", oldModel},
		} {
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
				ItemID: entry.id, Model: entry.model, Vector: meaningOf(axis),
				SourceDigest: suggestion.Digest(entry.title, ""), UpdatedAt: time.Now().UTC(),
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding two models' vectors: %v", err)
	}

	page := searched(ctx, t, tenantA, repository.TextSearch{
		Anchor: repository.Anchor{
			Kind: repository.AnchorCollection, CollectionID: collection, IncludeDescendants: true,
		},
		Request: view.Search{Words: "zzz-nothing-carries-this", Language: "de", Size: 50},
		Meaning: meaningOf(axis), MeaningModel: newModel,
	})

	found := map[shared.ID]bool{}
	for _, hit := range page.Hits {
		found[hit.Item.ID] = true
	}
	if !found[current] {
		t.Errorf("the entry embedded under the query's model was not found: %v", hitTitles(page))
	}
	if found[stale] {
		t.Errorf("an entry embedded under another model was ranked against the query's vector: %v",
			hitTitles(page))
	}
}
