// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
)

// The WEBHOOK_DELIVERY class: thirty days from the attempt, swept by the job that empties the
// trash. Which attempts are due - not a pending one a living job still names - is the statement's,
// so the store stands in for it here; what the engine owns is the period, the hold and the batch.
func TestAPassSweepsWebhookDeliveriesAtThirtyDays(t *testing.T) {
	h := newRunHarness()
	deliveries := &inboxStore{rows: []inboxRow{
		{receivedAt: now.Add(-200 * 24 * time.Hour)},
		{receivedAt: now.Add(-31 * 24 * time.Hour)},
		{receivedAt: now.Add(-29 * 24 * time.Hour)},
	}}
	h.run.Deliveries = deliveries

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if want := now.AddDate(0, 0, -30); !deliveries.askedAt.Equal(want) {
		t.Errorf("the cutoff was %v, want %v", deliveries.askedAt, want)
	}
	if len(deliveries.rows) != 1 || outcome.Removed < 2 {
		t.Errorf("%d attempts left, %d removed; want the one inside the period kept", len(deliveries.rows), outcome.Removed)
	}
	if _, published := h.signals.deleted[string(domain.KindWebhookDelivery)]; !published {
		t.Error("the sweep published no deletion count for the deliveries")
	}
}

// A workspace's own period is the one the pass reads, not the default.
func TestADeliveryPeriodTheWorkspaceSetIsTheOneRead(t *testing.T) {
	h := newRunHarness()
	h.policies.policies = map[domain.DataKind]domain.Policy{
		domain.KindWebhookDelivery: {DataKind: domain.KindWebhookDelivery, RetainDays: 60},
	}
	deliveries := &inboxStore{rows: []inboxRow{{receivedAt: now.Add(-45 * 24 * time.Hour)}}}
	h.run.Deliveries = deliveries

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if len(deliveries.rows) != 1 {
		t.Error("an attempt inside the workspace's sixty days was removed")
	}
}

// A delivery sits in no container, so a hold on the workspace is the one that keeps it: counted
// as held, and no reason to come straight back. A hold on a hub reaches no delivery.
func TestOnlyAHoldOnTheWorkspaceKeepsDeliveries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		holds domain.Holds
		kept  bool
	}{
		{"on the workspace", domain.Holds{{ID: holdID, Scope: domain.HoldTenant, Reason: "Litigation"}}, true},
		{"on a collection", domain.Holds{{ID: holdID, Scope: domain.HoldContainer, ScopeID: collectionID, Reason: "Litigation"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRunHarness()
			h.holds.holds = tc.holds
			deliveries := &inboxStore{rows: []inboxRow{{receivedAt: now.Add(-200 * 24 * time.Hour)}}}
			h.run.Deliveries = deliveries

			outcome, err := h.run.Execute(t.Context(), actor())
			if err != nil {
				t.Fatalf("the run failed: %v", err)
			}
			if kept := len(deliveries.rows) == 1; kept != tc.kept {
				t.Fatalf("kept %v, want %v", kept, tc.kept)
			}
			if tc.kept && (outcome.Blocked[BlockedByLegalHold] == 0 || !h.run.Exhausted(outcome)) {
				t.Errorf("a held delivery log reported %+v", outcome)
			}
		})
	}
}

// One batch per run, and the job comes back for the rest.
func TestTheDeliverySweepTakesOneBatch(t *testing.T) {
	h := newRunHarness()
	deliveries := &inboxStore{}
	for range h.purger.BatchSize + 5 {
		deliveries.rows = append(deliveries.rows, inboxRow{receivedAt: now.Add(-200 * 24 * time.Hour)})
	}
	h.run.Deliveries = deliveries

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if len(deliveries.rows) != 5 {
		t.Errorf("%d left, want one batch taken", len(deliveries.rows))
	}
	if h.run.Exhausted(outcome) {
		t.Error("a full batch of deliveries read as the end")
	}
}
