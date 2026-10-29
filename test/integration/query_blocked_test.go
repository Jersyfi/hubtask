// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// `blocked` traverses the links (F-9): an entry an open one blocks is blocked, its blocker is not,
// and an entry whose only blocker is completed is free again.
func TestTheQueryFindsTheEntriesAnOpenOneBlocks(t *testing.T) {
	ctx := context.Background()
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	blocker := seedTask(ctx, t, tenantA, authorA, collection)
	waiting := seedTask(ctx, t, tenantA, authorA, collection)

	registry := domain.NewLinkTypes(nil)
	blocks, _ := registry.Find(domain.LinkTypeBlocks)
	link, err := domain.NewEntryLink(domain.NewEntryLinkInput{
		ID: freshID(t), TenantID: tenantA, Type: blocks,
		SourceID: blocker, TargetID: waiting, CreatedBy: authorA, Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	links := postgres.NewEntryLinkRepository()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := links.Lock(ctx); err != nil {
			return err
		}
		return links.Insert(ctx, link)
	}); err != nil {
		t.Fatalf("drawing the link: %v", err)
	}

	result := queried(ctx, t, tenantA, searchIn(t, collection,
		map[string]any{"field": "blocked", "op": "EQ", "value": true}, view.Spec{}))
	if len(result.Items) != 1 || result.Items[0].ID != waiting {
		t.Errorf("blocked answered %v", titlesOf(result.Items))
	}

	free := queried(ctx, t, tenantA, searchIn(t, collection,
		map[string]any{"field": "blocked", "op": "EQ", "value": false}, view.Spec{}))
	if len(free.Items) != 1 || free.Items[0].ID != blocker {
		t.Errorf("not blocked answered %v", titlesOf(free.Items))
	}
}
