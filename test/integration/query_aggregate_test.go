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

// The status history is written as an entry enters a status, and a summary reads cycle time,
// WIP age and time in status from it (migration 0250, UC-GOL-04 check 11).
func TestAFlowQuantityReadsTheStatusHistory(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	done := seedTask(ctx, t, tenantA, authorA, collection)
	open := seedTask(ctx, t, tenantA, authorA, collection)
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	pool := adminPool(ctx, t)
	step := func(entry shared.ID, status string, at time.Time, completed bool) {
		t.Helper()
		var completedAt any
		if completed {
			completedAt = at
		}
		if _, err := pool.Exec(ctx, `UPDATE work_item SET status = $1, updated_at = $2, completed_at = $3
			WHERE tenant_id = $4 AND id = $5`, status, at, completedAt, tenantA.String(), entry.String()); err != nil {
			t.Fatalf("stepping: %v", err)
		}
	}
	step(done, "todo", start, false)
	step(done, "in_progress", start.Add(time.Hour), false)
	step(done, "done", start.Add(3*time.Hour), true)
	step(open, "in_progress", start, false)
	step(open, "review", start.Add(30*time.Minute), false)

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM entry_status_change WHERE tenant_id = $1 AND item_id = $2`,
		tenantA.String(), done.String()).Scan(&rows); err != nil || rows != 3 {
		t.Fatalf("the history holds %d rows, want 3: %v", rows, err)
	}

	measure := func(quantity string) *string {
		t.Helper()
		aggregation, err := view.ParseAggregation(map[string]any{"function": "MAX", "field": quantity}, "/aggregate")
		if err != nil {
			t.Fatal(err)
		}
		aggregation.Started, aggregation.Now = []string{"in_progress"}, start.Add(2*time.Hour)
		var result repository.ItemAggregateResult
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			result, err = itemRepo().Aggregate(ctx, searchIn(t, collection, nil, view.Spec{}), aggregation, "UTC")
			return err
		}); err != nil || len(result.Rows) != 1 {
			t.Fatalf("aggregating %s: %+v %v", quantity, result, err)
		}
		return result.Rows[0].Value
	}
	for quantity, want := range map[string]string{"cycle_time": "120", "wip_age": "120", "time_in_status": "90"} {
		if got := measure(quantity); got == nil || *got != want {
			t.Errorf("%s = %v, want %s", quantity, got, want)
		}
	}
}

// A time summary sums the records on the kept entries that the reader sees - their own, and
// another person's only in a shown collection - and withholds a small group where asked (F-9).
func TestATimeSummarySumsTheRecordsTheReaderSees(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	entry := seedTask(ctx, t, tenantA, authorA, collection)
	other := freshID(t)
	pool := adminPool(ctx, t)
	for _, record := range []struct {
		account shared.ID
		minutes int
	}{{authorA, 30}, {other, 45}} {
		if _, err := pool.Exec(ctx, `INSERT INTO time_record (id, tenant_id, account_id, entry_id, day, minutes, source,
			created_at, updated_at) VALUES ($1, $2, $3, $4, '2026-11-02', $5, 'MANUAL', now(), now())`,
			freshID(t).String(), tenantA.String(), record.account.String(), entry.String(), record.minutes); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}
	sum := func(tenant shared.ID, sight view.TimeSight) repository.ItemAggregateResult {
		t.Helper()
		aggregation, err := view.ParseAggregation(map[string]any{"source": "time", "function": "SUM", "field": "minutes",
			"bucket": map[string]any{"field": "day", "period": "week"}}, "/aggregate")
		if err != nil {
			t.Fatal(err)
		}
		aggregation.Sight = sight
		var result repository.ItemAggregateResult
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			result, err = itemRepo().Aggregate(ctx, searchIn(t, collection, nil, view.Spec{}), aggregation, "UTC")
			return err
		}); err != nil {
			t.Fatalf("aggregating: %v", err)
		}
		return result
	}
	value := func(result repository.ItemAggregateResult) string {
		if len(result.Rows) != 1 || result.Rows[0].Value == nil {
			return ""
		}
		return *result.Rows[0].Value
	}

	if own := sum(tenantA, view.TimeSight{Reader: authorA}); value(own) != "30" || own.Rows[0].Period != "2026-11-02" {
		t.Errorf("the reader's own time answered %+v, want 30 minutes in the week of 2 November", own)
	}
	if all := sum(tenantA, view.TimeSight{Reader: authorA, Shown: []shared.ID{collection}}); value(all) != "75" {
		t.Errorf("the shown collection's time answered %+v, want 75", all)
	}
	if few := sum(tenantA, view.TimeSight{Reader: authorA, Shown: []shared.ID{collection}, Withhold: true}); len(few.Rows) != 0 {
		t.Errorf("a group of two people answered %+v, want it withheld", few)
	}
	if foreign := sum(tenantB, view.TimeSight{Reader: authorA, Shown: []shared.ID{collection}}); len(foreign.Rows) != 0 {
		t.Errorf("another tenant counted %+v", foreign)
	}
}

// A pages summary counts the live pages of the bound spaces, never a trashed one nor another
// tenant's (F-9).
func TestAPagesSummaryCountsTheLivePagesOfTheBoundSpaces(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	pool := adminPool(ctx, t)
	var hub string
	if err := pool.QueryRow(ctx, `SELECT parent_id::text FROM container WHERE tenant_id = $1 AND id = $2`,
		tenantA.String(), collection.String()).Scan(&hub); err != nil {
		t.Fatalf("reading the hub: %v", err)
	}
	for i, trashed := range []bool{false, false, true} {
		var deleted any
		if trashed {
			deleted = time.Now()
		}
		if _, err := pool.Exec(ctx, `INSERT INTO page (id, tenant_id, hub_id, title, order_key, created_by, created_at,
			updated_by, updated_at, deleted_at) VALUES ($1, $2, $3, 'P', $4, $5, now(), $5, now(), $6)`,
			freshID(t).String(), tenantA.String(), hub, string(rune('a'+i)), authorA.String(), deleted); err != nil {
			t.Fatalf("writing a page: %v", err)
		}
	}
	// A restricted page naming nobody yet: kept from every count until it names its reader.
	restricted := freshID(t).String()
	if _, err := pool.Exec(ctx, `INSERT INTO page (id, tenant_id, hub_id, title, order_key, created_by, created_at,
		updated_by, updated_at, restricted) VALUES ($1, $2, $3, 'R', 'z', $4, now(), $4, now(), true)`,
		restricted, tenantA.String(), hub, authorA.String()); err != nil {
		t.Fatalf("writing the restricted page: %v", err)
	}
	count := func(tenant shared.ID) repository.ItemAggregateResult {
		t.Helper()
		aggregation, err := view.ParseAggregation(map[string]any{"source": "pages", "function": "COUNT",
			"group_by": "verification"}, "/aggregate")
		if err != nil {
			t.Fatal(err)
		}
		aggregation.Hubs, aggregation.Now, aggregation.Reader = []shared.ID{shared.MustParseID(hub)}, time.Now(), authorA
		var result repository.ItemAggregateResult
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			result, err = itemRepo().Aggregate(ctx, repository.ItemSearch{}, aggregation, "UTC")
			return err
		}); err != nil {
			t.Fatalf("aggregating: %v", err)
		}
		return result
	}
	if mine := count(tenantA); len(mine.Rows) != 1 || mine.Rows[0].Group != "unverified" || mine.Rows[0].Count != 2 {
		t.Errorf("the space answered %+v, want two unverified pages", mine)
	}
	if other := count(tenantB); len(other.Rows) != 0 {
		t.Errorf("another tenant counted %+v", other)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO page_restriction (tenant_id, page_id, account_id, granted_at)
		VALUES ($1, $2, $3, now())`, tenantA.String(), restricted, authorA.String()); err != nil {
		t.Fatalf("naming the reader: %v", err)
	}
	if mine := count(tenantA); len(mine.Rows) != 1 || mine.Rows[0].Count != 3 {
		t.Errorf("the page naming its reader is not counted: %+v", mine)
	}
}
