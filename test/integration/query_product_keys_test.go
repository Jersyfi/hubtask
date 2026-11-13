// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The product keys of the Hubtask query (ADR-0102) the entry compiler answers: each finds the
// entries it names in its workspace, and a value naming another workspace's row finds nothing.
func TestTheProductKeysFindTheirEntries(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection, planned := seedPlan(ctx, t, tenantA)
	admin := adminPool(ctx, t)
	exec := func(what, sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seeding %s: %v", what, err)
		}
	}
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	sprint, release, baseline := freshID(t), freshID(t), freshID(t)
	exec("a sprint", `INSERT INTO sprint (id, tenant_id, collection_id, name, start_on, end_on, state, created_at, updated_at)
		VALUES ($1, $2, $3, $4, '2026-11-02', '2026-11-13', 'ACTIVE', $5, $5)`, sprint.String(), tenantA.String(), collection.String(), freshName(t), at)
	exec("a sprint entry", `INSERT INTO sprint_entry (tenant_id, sprint_id, entry_id, added_at) VALUES ($1, $2, $3, $4)`,
		tenantA.String(), sprint.String(), planned[0].String(), at)
	exec("a release", `INSERT INTO release (id, tenant_id, hub_id, name, release_on, state, created_at, updated_at)
		VALUES ($1, $2, $3, $4, '2026-12-01', 'PLANNED', $5, $5)`, release.String(), tenantA.String(), hub.String(), freshName(t), at)
	exec("a release entry", `INSERT INTO release_entry (tenant_id, release_id, entry_id, added_at) VALUES ($1, $2, $3, $4)`,
		tenantA.String(), release.String(), planned[1].String(), at)
	exec("a baseline", `INSERT INTO plan_baseline (id, tenant_id, plan_id, name, taken_at) VALUES ($1, $2, $3, 'Kick-off', $4)`,
		baseline.String(), tenantA.String(), hub.String(), at)
	// The first entry was due two days earlier when the baseline was taken; the second kept its date.
	exec("the baseline's entries", `INSERT INTO plan_baseline_entry (tenant_id, baseline_id, entry_id, due_at)
		VALUES ($1, $2, $3, '2026-11-02T00:00:00Z'), ($1, $2, $4, '2026-11-06T00:00:00Z')`,
		tenantA.String(), baseline.String(), planned[0].String(), planned[1].String())

	_, ci, _ := seedCIs(ctx, t)
	exec("an affected item", `INSERT INTO cmdb_affected (id, tenant_id, item_id, ci_id, added_by, added_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		freshID(t).String(), tenantA.String(), planned[1].String(), ci.String(), authorA.String(), at)

	parent := seedTask(ctx, t, tenantA, authorA, collection)
	child := childrenUnder(ctx, t, tenantA, authorA, collection, parent, 1, 0)[0]
	blockLink(ctx, t, parent, planned[0])

	cases := []struct {
		name   string
		filter map[string]any
		want   []shared.ID
	}{
		{"hub", map[string]any{"field": "hub_id", "op": "EQ", "value": hub.String()}, []shared.ID{planned[0], planned[1], parent, child}},
		{"under", map[string]any{"field": "under", "op": "EQ", "value": parent.String()}, []shared.ID{child}},
		{"blocks", map[string]any{"field": "blocks", "op": "EQ", "value": planned[0].String()}, []shared.ID{parent}},
		{"affects", map[string]any{"field": "affects", "op": "EQ", "value": ci.String()}, []shared.ID{planned[1]}},
		{"plan.sprint", map[string]any{"field": "sprint_id", "op": "EQ", "value": sprint.String()}, []shared.ID{planned[0]}},
		{"plan.sprint: none", map[string]any{"op": "AND", "nodes": []any{
			map[string]any{"field": "sprint_id", "op": "IS_NULL"}, map[string]any{"field": "parent_id", "op": "IS_NULL"},
		}}, []shared.ID{planned[1], parent}},
		{"plan.release", map[string]any{"field": "release_id", "op": "EQ", "value": release.String()}, []shared.ID{planned[1]}},
		{"plan.slip", map[string]any{"field": "slip_days", "op": "GT", "value": 0}, []shared.ID{planned[0]}},
		{"plan.slip kept", map[string]any{"field": "slip_days", "op": "EQ", "value": 0}, []shared.ID{planned[1]}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := queried(ctx, t, tenantA, searchIn(t, collection, c.filter, view.Spec{}))
			if !sameIDs(idsOfItems(result.Items), c.want) {
				t.Errorf("found %v, want %v", idsOfItems(result.Items), c.want)
			}
		})
	}

	// Another workspace naming tenant A's rows finds none of its own entries by them.
	_, collectionB, _ := seedPlan(ctx, t, tenantB)
	for _, filter := range []map[string]any{
		{"field": "hub_id", "op": "EQ", "value": hub.String()},
		{"field": "sprint_id", "op": "EQ", "value": sprint.String()},
		{"field": "release_id", "op": "EQ", "value": release.String()},
		{"field": "affects", "op": "EQ", "value": ci.String()},
		{"field": "slip_days", "op": "IS_NULL"},
	} {
		got := queried(ctx, t, tenantB, searchIn(t, collection, filter, view.Spec{}))
		if len(got.Items) != 0 {
			t.Errorf("tenant B read %v out of tenant A by %v", titlesOf(got.Items), filter)
		}
		own := queried(ctx, t, tenantB, searchIn(t, collectionB, filter, view.Spec{}))
		if filter["op"] != "IS_NULL" && len(own.Items) != 0 {
			t.Errorf("tenant A's row %v matched tenant B's entries", filter)
		}
	}
}

// blockLink draws a `blocks` link from one entry to another of tenant A.
func blockLink(ctx context.Context, t *testing.T, source, target shared.ID) {
	t.Helper()
	blocks, _ := domain.NewLinkTypes(nil).Find(domain.LinkTypeBlocks)
	link, err := domain.NewEntryLink(domain.NewEntryLinkInput{
		ID: freshID(t), TenantID: tenantA, Type: blocks,
		SourceID: source, TargetID: target, CreatedBy: authorA, Now: time.Now().UTC(),
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
}

func sameIDs(got, want []shared.ID) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[shared.ID]bool{}
	for _, id := range got {
		seen[id] = true
	}
	for _, id := range want {
		if !seen[id] {
			return false
		}
	}
	return true
}
