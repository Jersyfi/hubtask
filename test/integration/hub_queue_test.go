// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func hubQueueIn(t *testing.T, tenant, owner, hub shared.ID, name string, position int, sharing view.Sharing) view.SavedView {
	t.Helper()
	saved, err := view.NewSavedView(view.NewSavedViewInput{
		ID: freshID(t), TenantID: tenant, OwnerID: owner,
		ScopeType: view.ViewScopeHub, ScopeID: hub,
		Name: name, Layout: "LIST_EXPANDED",
		Query:   map[string]any{"scope_container_id": hub.String()},
		Sharing: sharing, Queue: true, QueuePosition: position,
		Text: text.Composing{}, Now: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

// UC-SVC-03 check 10's store: a hub's views marked as queues are read back in their place - one
// named without a place after the hub's last, a view becoming one likewise, one taken out of the
// queues gone - the caller's own private queue with them and another's private one never; and
// the workspace next door neither reads nor places them (migrations 0391-0392).
func TestAHubsQueuesAreItsMarkedViewsInTheirPlace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, _ := hubWithCollection(ctx, t, tenantA, authorA)

	second := hubQueueIn(t, tenantA, authorA, hub, "Second", 2, view.SharingScope)
	appended := hubQueueIn(t, tenantA, authorA, hub, "Appended", 0, view.SharingScope)
	mine := hubQueueIn(t, tenantA, authorA, hub, "Mine", 1, view.SharingPrivate)
	theirs := hubQueueIn(t, tenantA, seedAccount(ctx, t, tenantA), hub, "Theirs", 1, view.SharingPrivate)
	plain := hubQueueIn(t, tenantA, authorA, hub, "Plain", 0, view.SharingScope)
	plain.Queue = false
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, saved := range []view.SavedView{second, appended, mine, theirs, plain} {
			if err := viewRepo().Insert(ctx, saved); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the queues: %v", err)
	}

	listed := func(tenant shared.ID) []view.SavedView {
		t.Helper()
		var out []view.SavedView
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			out, err = viewRepo().ListHubQueues(ctx, authorA, hub)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	got := listed(tenantA)
	if len(got) != 3 || got[0].ID != mine.ID || got[1].ID != second.ID || got[2].ID != appended.ID {
		t.Fatalf("the hub's queues came back as %v", namesOf(got))
	}
	if got[2].QueuePosition != 3 || !got[2].Queue {
		t.Errorf("a queue named without a place stands at %d, want after the last", got[2].QueuePosition)
	}

	yes, no := true, false
	becomes, _, err := plain.Updated(view.ViewAttributes{Queue: &yes}, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	leaves, _, err := second.Updated(view.ViewAttributes{Queue: &no}, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := viewRepo().SetAttributes(ctx, becomes, plain.Version); err != nil {
			return err
		}
		return viewRepo().SetAttributes(ctx, leaves, second.Version)
	}); err != nil {
		t.Fatal(err)
	}
	got = listed(tenantA)
	if len(got) != 3 || got[0].ID != mine.ID || got[1].ID != appended.ID || got[2].ID != plain.ID || got[2].QueuePosition != 4 {
		t.Fatalf("after one became a queue and one left: %v", namesOf(got))
	}

	if other := listed(tenantB); len(other) != 0 {
		t.Errorf("the workspace next door read the hub's queues: %v", namesOf(other))
	}
	nine := 9
	stolen, _, err := appended.Updated(view.ViewAttributes{QueuePosition: &nine}, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return viewRepo().SetAttributes(ctx, stolen, appended.Version)
	}); err == nil {
		t.Error("the workspace next door placed a queue")
	}
}

// UC-SVC-03 checks 3 and 10: a queue's count and how many of it are at risk or breached come from
// one statement with the query's own filter, and the workspace next door counts none of them.
func TestAQueueIsCountedWithItsEntriesAtRisk(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection := hubWithCollection(ctx, t, tenantA, authorA)
	running := seedTask(ctx, t, tenantA, authorA, collection)
	atRisk := seedTask(ctx, t, tenantA, authorA, collection)
	breached := seedTask(ctx, t, tenantA, authorA, collection)
	seedTask(ctx, t, tenantA, authorA, collection)

	now := time.Now().UTC().Truncate(time.Second)
	repo := postgres.NewServiceDeskRepository()
	record := func(item shared.ID, standing servicedesk.Standing) servicedesk.EntrySLA {
		due := now.Add(time.Hour)
		r := servicedesk.EntrySLA{
			ID: freshID(t), TenantID: tenantA, ItemID: item, Goal: servicedesk.GoalResolution,
			PolicyKey: "incidents", Target: time.Hour, Standing: standing, ArmedAt: now, DueAt: &due,
		}
		if standing == servicedesk.StandingBreached {
			r.BreachedAt, r.DueAt = &due, nil
		}
		return r
	}
	risky := record(atRisk, servicedesk.StandingRunning)
	risky.AtRiskAt = &now
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, r := range []servicedesk.EntrySLA{record(running, servicedesk.StandingRunning), risky, record(breached, servicedesk.StandingBreached)} {
			if err := repo.Records().Save(ctx, r, now); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("saving the goals: %v", err)
	}

	within, err := view.ParseFilter(map[string]any{"field": "sla_standing", "op": "IN", "value": []any{"AT_RISK", "BREACHED"}}, "/within")
	if err != nil {
		t.Fatal(err)
	}
	open := searchIn(t, collection, map[string]any{"field": "is_completed", "op": "EQ", "value": false}, view.Spec{})
	open.Anchor = repository.Anchor{Kind: repository.AnchorHub, HubID: hub, IncludeDescendants: true}
	counted := func(tenant shared.ID, search repository.ItemSearch) repository.ItemCount {
		t.Helper()
		var out repository.ItemCount
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			out, err = itemRepo().CountWithin(ctx, search, *within)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := counted(tenantA, open); got.Total != 4 || got.Within != 2 || got.Uncounted {
		t.Errorf("the open queue counts %+v, want 4 with 2 at risk", got)
	}
	if got := counted(tenantB, open); got.Total != 0 || got.Within != 0 {
		t.Errorf("the workspace next door counted %+v", got)
	}
}

func namesOf(views []view.SavedView) []string {
	out := make([]string, 0, len(views))
	for _, saved := range views {
		out = append(out, saved.Name)
	}
	return out
}
