// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

func aggregated(
	ctx context.Context, t *testing.T, tenant shared.ID, search repository.ItemSearch, raw map[string]any,
) repository.ItemAggregateResult {
	t.Helper()
	aggregation, err := view.ParseAggregation(raw, "/aggregate")
	if err != nil {
		t.Fatal(err)
	}
	aggregation, err = aggregation.Shaped(nil)
	if err != nil {
		t.Fatal(err)
	}
	var result repository.ItemAggregateResult
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		result, err = itemRepo().Aggregate(ctx, search, aggregation, "Europe/Berlin")
		return err
	}); err != nil {
		t.Fatalf("aggregating: %v", err)
	}
	return result
}

// An aggregation counts what the query keeps, per group and per period, and never another
// tenant's entries (S-11).
func TestAnAggregationCountsWhatTheQueryKeeps(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	seedTask(ctx, t, tenantA, authorA, collection)
	seedTask(ctx, t, tenantA, authorA, collection)

	search := searchIn(t, collection, nil, view.Spec{})
	result := aggregated(ctx, t, tenantA, search, map[string]any{
		"function": "COUNT", "group_by": "is_completed",
		"bucket": map[string]any{"field": "created_at", "period": "day"},
	})
	if len(result.Rows) != 1 || result.Rows[0].Count != 2 || result.Rows[0].Group != "false" ||
		result.Rows[0].Value != nil || result.Truncated {
		t.Fatalf("the aggregation answered %+v, want two open entries on one day", result)
	}
	day, err := time.Parse(time.DateOnly, result.Rows[0].Period)
	if err != nil {
		t.Fatalf("the period is not a day: %q", result.Rows[0].Period)
	}

	monthly := aggregated(ctx, t, tenantA, search, map[string]any{
		"function": "COUNT", "bucket": map[string]any{"field": "created_at", "period": "month"},
	})
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	if len(monthly.Rows) != 1 || monthly.Rows[0].Period != first || monthly.Rows[0].Group != "" {
		t.Errorf("the month answered %+v, want %s", monthly, first)
	}

	other := aggregated(ctx, t, tenantB, search, map[string]any{"function": "COUNT"})
	if len(other.Rows) != 1 || other.Rows[0].Count != 0 {
		t.Errorf("another tenant counted %+v", other)
	}
}

// A Sunday week begins on a Sunday and a Monday week on a Monday; a workspace summary counts the
// bound collections and nothing else; an open entry holds no lead time (S-11).
func TestAWorkspaceSummaryCountsTheBoundCollectionsByWeek(t *testing.T) {
	ctx := context.Background()
	kept := collectionFor(ctx, t, tenantA, authorA)
	left := collectionFor(ctx, t, tenantA, authorA)
	seedTask(ctx, t, tenantA, authorA, kept)
	seedTask(ctx, t, tenantA, authorA, left)

	search := searchIn(t, kept, nil, view.Spec{})
	search.Anchor = repository.Anchor{Kind: repository.AnchorCollections, CollectionIDs: []shared.ID{kept}}
	for start, weekday := range map[string]time.Weekday{"monday": time.Monday, "sunday": time.Sunday} {
		result := aggregated(ctx, t, tenantA, search, map[string]any{
			"function": "COUNT",
			"bucket":   map[string]any{"field": "created_at", "period": "week", "week_start": start},
		})
		if len(result.Rows) != 1 || result.Rows[0].Count != 1 {
			t.Fatalf("the %s week answered %+v, want the one entry of the bound collection", start, result)
		}
		day, err := time.Parse(time.DateOnly, result.Rows[0].Period)
		if err != nil || day.Weekday() != weekday {
			t.Errorf("the %s week begins on %q", start, result.Rows[0].Period)
		}
	}

	lead := aggregated(ctx, t, tenantA, search, map[string]any{"function": "AVG", "field": "lead_time"})
	if len(lead.Rows) != 1 || lead.Rows[0].Value != nil {
		t.Errorf("an open entry held a lead time: %+v", lead)
	}

	search.Anchor.CollectionIDs = nil
	none := aggregated(ctx, t, tenantA, search, map[string]any{"function": "COUNT"})
	if len(none.Rows) != 1 || none.Rows[0].Count != 0 {
		t.Errorf("no collection counted %+v", none)
	}
}

// A links summary counts a link only where the scope keeps both its ends, per type, and never
// another tenant's (F-9): a link into a collection outside the scope is not counted.
func TestALinksSummaryCountsTheLinksBetweenTheKeptEntries(t *testing.T) {
	ctx := context.Background()
	kept := collectionFor(ctx, t, tenantA, authorA)
	outside := collectionFor(ctx, t, tenantA, authorA)
	first := seedTask(ctx, t, tenantA, authorA, kept)
	second := seedTask(ctx, t, tenantA, authorA, kept)
	away := seedTask(ctx, t, tenantA, authorA, outside)

	pool := adminPool(ctx, t)
	for _, link := range [][3]string{
		{"blocks", first.String(), second.String()},
		{"duplicates", first.String(), away.String()},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO entry_link (id, tenant_id, link_type, source_id, target_id, created_at)
			VALUES ($1, $2, $3, $4, $5, now())`,
			freshID(t).String(), tenantA.String(), link[0], link[1], link[2]); err != nil {
			t.Fatalf("linking: %v", err)
		}
	}

	byType := map[string]any{"source": "links", "function": "COUNT", "group_by": "link_type"}
	inside := aggregated(ctx, t, tenantA, searchIn(t, kept, nil, view.Spec{}), byType)
	if len(inside.Rows) != 1 || inside.Rows[0].Group != "blocks" || inside.Rows[0].Count != 1 {
		t.Errorf("the collection's links answered %+v, want the one blocks link inside it", inside)
	}

	both := searchIn(t, kept, nil, view.Spec{})
	both.Anchor = repository.Anchor{Kind: repository.AnchorCollections, CollectionIDs: []shared.ID{kept, outside}}
	across := aggregated(ctx, t, tenantA, both, byType)
	if len(across.Rows) != 2 {
		t.Errorf("both collections' links answered %+v, want blocks and duplicates", across)
	}

	if other := aggregated(ctx, t, tenantB, both, byType); len(other.Rows) != 0 {
		t.Errorf("another tenant counted %+v", other)
	}
}
