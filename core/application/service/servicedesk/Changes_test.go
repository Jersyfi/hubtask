// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

type changeLog struct{ changes []changelog.Change }

func (l *changeLog) Record(_ context.Context, change changelog.Change) error {
	l.changes = append(l.changes, change)
	return nil
}

type hlcs struct{ reading shared.HLC }

func (h *hlcs) Next() shared.HLC {
	next, _ := h.reading.Tick(time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC), "server")
	h.reading = next
	return next
}

// The service desk offline (offline-sync.md §3.1): every SLA record, rating and approval the desk
// writes reaches the change log as it is now stored, filed under the entry's collection - an
// approval with its trail - and a row of an entry gone is told nobody.
func TestEveryDeskRecordOfAnEntryReachesTheChangeLog(t *testing.T) {
	ctx := context.Background()
	tenant, collection := shared.ID("t-1"), shared.ID("c-1")
	entry := work.WorkItem{ID: "i-1", TenantID: tenant, CollectionID: collection}
	log := &changeLog{}
	announcer := Announcer{Changes: log, HLC: &hlcs{}, Items: &itemStore{item: entry}}
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)

	records := announcer.Records(&recordStore{})
	record := domain.EntrySla{ID: "s-1", TenantID: tenant, ItemID: entry.ID, Goal: domain.GoalResolution, Standing: domain.StandingRunning, Target: time.Hour, ArmedAt: at}
	if err := records.Save(ctx, record, at); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := records.Save(ctx, domain.EntrySla{ID: "s-2", TenantID: tenant, ItemID: "gone", Goal: domain.GoalResolution, Target: time.Hour, ArmedAt: at}, at); err != nil {
		t.Fatalf("Save of an entry gone: %v", err)
	}

	ratings := announcer.Ratings(&ratingStore{rows: map[shared.ID]domain.Rating{}})
	if _, err := ratings.Save(ctx, domain.Rating{ID: "r-1", TenantID: tenant, ItemID: entry.ID, Score: 5, RatedAt: at}); err != nil {
		t.Fatalf("Save rating: %v", err)
	}

	store := &approvalStore{rows: map[shared.ID]domain.Approval{}}
	approvals := announcer.Approvals(store)
	approval := domain.Approval{ID: "a-1", TenantID: tenant, ItemID: entry.ID, Step: "approve", State: domain.ApprovalPending, RequestedAt: at, Version: 1}
	if err := approvals.Insert(ctx, approval); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := approvals.Record(ctx, domain.Decision{ID: "d-1", TenantID: tenant, ApprovalID: approval.ID, Verdict: domain.VerdictApprove, DecidedAt: at}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	approved := approval
	approved.State, approved.Version = domain.ApprovalApproved, 2
	if err := approvals.Update(ctx, approved, 1); err != nil {
		t.Fatalf("Update: %v", err)
	}

	want := []struct {
		entity string
		id     shared.ID
	}{{EntrySlaEntity, "s-1"}, {SatisfactionEntity, "r-1"}, {ApprovalEntity, "a-1"}, {ApprovalEntity, "a-1"}}
	if len(log.changes) != len(want) {
		t.Fatalf("%d changes recorded, want %d: %+v", len(log.changes), len(want), log.changes)
	}
	for i, change := range log.changes {
		if change.Entity != want[i].entity || change.EntityID != want[i].id || change.ContainerID != collection ||
			change.Op != changelog.Upsert || change.TenantID != tenant || change.HLC.IsZero() {
			t.Errorf("change %d = %+v, want %s %s under the collection", i, change, want[i].entity, want[i].id)
		}
	}
	if log.changes[0].Payload["item_id"] != entry.ID.String() || log.changes[1].Payload["score"] != 5 {
		t.Errorf("the record and the rating were told as %v and %v", log.changes[0].Payload, log.changes[1].Payload)
	}
	last := log.changes[3].Payload
	if last["state"] != string(domain.ApprovalApproved) || len(last["decisions"].([]usecase.Output)) != 1 {
		t.Errorf("the decided approval was told as %v, want it approved with its trail", last)
	}
}
