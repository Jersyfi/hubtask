// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-WRK-25 check 5 against PostgreSQL: a collection names its own labels as categories, they are
// stored in its layer and read back in order; another workspace's label is not found under row
// level security and is refused like a label that does not exist.
func TestACollectionsCategoriesAreItsOwnLabelsInOrder(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := structureCatalogue(t)
	actor := structureActor(tenantA, authorA)

	hub := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t)})
	collection := createIn(ctx, t, registry, usecase.Input{"type": "COLLECTION", "name": freshName(t), "parent_id": hub})
	collectionID := shared.MustParseID(collection)

	dairy, produce := freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := labelRepo().Insert(ctx, labelIn(tenantA, collectionID, dairy, freshName(t))); err != nil {
			return err
		}
		return labelRepo().Insert(ctx, labelIn(tenantA, collectionID, produce, freshName(t)))
	}); err != nil {
		t.Fatalf("seeding labels: %v", err)
	}
	foreign := freshID(t)
	foreignCollection := collectionFor(ctx, t, tenantB, authorB)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return labelRepo().Insert(ctx, labelIn(tenantB, foreignCollection, foreign, freshName(t)))
	}); err != nil {
		t.Fatalf("seeding the other workspace's label: %v", err)
	}

	set := func(categories ...shared.ID) error {
		ids := make([]any, 0, len(categories))
		for _, id := range categories {
			ids = append(ids, id.String())
		}
		_, err := registry.Invoke(ctx, "SetContainerStructure", actor, usecase.Input{
			"container_id": collection,
			"settings": map[string]any{"fields": map[string]any{
				"TASK": map[string]any{"capabilities": []any{"COMPLETION", "LABELS"}, "categories": ids},
			}},
		})
		return err
	}

	if err := set(produce, dairy); err != nil {
		t.Fatalf("naming categories: %v", err)
	}
	settings, _ := structureOf(ctx, t, registry, collection)["settings"].(map[string]any)
	fields, _ := settings["fields"].(map[string]any)
	task, _ := fields["TASK"].(map[string]any)
	got, _ := task["categories"].([]any)
	if len(got) != 2 || got[0] != produce.String() || got[1] != dairy.String() {
		t.Errorf("the read answers %v", task)
	}

	err := set(dairy, foreign)
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "structure.category_unknown" {
		t.Fatalf("another workspace's label answered %v", err)
	}
	if err := set(dairy, shared.MustParseID(hub)); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "structure.category_unknown" {
		t.Errorf("an identifier that is no label answered %v", err)
	}
}
