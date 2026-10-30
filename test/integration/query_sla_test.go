// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A queue by what breaks first (UC-SVC-03 checks 1 to 3): sla_due orders by the next running
// goal, entries without one last, a page boundary continues the order, and sla_standing filters
// by the worst goal.
func TestTheQueueSortsByTheNextBreach(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	later := seedTask(ctx, t, tenantA, authorA, collection)
	sooner := seedTask(ctx, t, tenantA, authorA, collection)
	none := seedTask(ctx, t, tenantA, authorA, collection)
	breached := seedTask(ctx, t, tenantA, authorA, collection)

	now := time.Now().UTC().Truncate(time.Second)
	repo := postgres.NewServiceDeskRepository()
	record := func(item shared.ID, standing servicedesk.Standing, due time.Time) servicedesk.EntrySla {
		r := servicedesk.EntrySla{
			ID: freshID(t), TenantID: tenantA, ItemID: item, Goal: servicedesk.GoalResolution,
			PolicyKey: "incidents", Target: time.Hour, Standing: standing, ArmedAt: now, DueAt: &due,
		}
		if standing == servicedesk.StandingBreached {
			r.BreachedAt, r.DueAt = &due, nil
		}
		return r
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, r := range []servicedesk.EntrySla{
			record(later, servicedesk.StandingRunning, now.Add(5*time.Hour)),
			record(sooner, servicedesk.StandingRunning, now.Add(time.Hour)),
			record(breached, servicedesk.StandingBreached, now.Add(-time.Hour)),
		} {
			if err := repo.Records().Save(ctx, r, now); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("saving the goals: %v", err)
	}

	sort, err := view.ParseSort([]any{map[string]any{"field": "sla_due", "nulls": "LAST"}}, "/sort")
	if err != nil {
		t.Fatal(err)
	}
	search := searchIn(t, collection, nil, view.Spec{Sort: sort, Size: 1})
	var walked []shared.ID
	for range 5 {
		page := queried(ctx, t, tenantA, search)
		for _, item := range page.Items {
			walked = append(walked, item.ID)
		}
		if !page.Info.HasMore {
			break
		}
		search.Spec.Cursor = page.Info.NextCursor
	}
	if len(walked) != 4 || walked[0] != sooner || walked[1] != later {
		t.Fatalf("the queue walked %v, want %v then %v first", walked, sooner, later)
	}
	for _, id := range walked[2:] {
		if id != none && id != breached {
			t.Errorf("an entry without a running goal came before the end: %v", walked)
		}
	}

	worst := queried(ctx, t, tenantA, searchIn(t, collection,
		map[string]any{"field": "sla_standing", "op": "EQ", "value": "BREACHED"}, view.Spec{}))
	if len(worst.Items) != 1 || worst.Items[0].ID != breached {
		t.Errorf("sla_standing BREACHED answered %v", titlesOf(worst.Items))
	}
	// UC-SVC-03 check 3: a queue's count is its query's exact count.
	running := queried(ctx, t, tenantA, searchIn(t, collection,
		map[string]any{"field": "sla_standing", "op": "IN", "value": []any{"RUNNING", "AT_RISK"}},
		view.Spec{Count: view.CountExact}))
	if running.Total != 2 {
		t.Errorf("the queue of running goals counts %d, want 2", running.Total)
	}
	unmeasured := queried(ctx, t, tenantA, searchIn(t, collection,
		map[string]any{"field": "sla_standing", "op": "IS_NULL"}, view.Spec{}))
	if len(unmeasured.Items) != 1 || unmeasured.Items[0].ID != none {
		t.Errorf("sla_standing IS_NULL answered %v", titlesOf(unmeasured.Items))
	}
}
