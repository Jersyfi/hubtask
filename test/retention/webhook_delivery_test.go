// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package retention

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// subscription seeds one webhook subscription in the suite's workspace.
func (s *suite) subscription(t *testing.T) shared.ID {
	t.Helper()
	id := freshID(t)
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO webhook_subscription (id, tenant_id, target_url, event_types, secret_enc, created_by)
		VALUES ($1, $2, 'https://receiver.example.invalid/hook', ARRAY['de.hubtask.work.item.created.v1'], '\x00', $3)`,
		id.String(), s.tenant.String(), s.author.String()); err != nil {
		t.Fatalf("seeding the subscription: %v", err)
	}
	return id
}

// delivery seeds one attempt `daysAgo` days old.
func (s *suite) delivery(t *testing.T, subscription shared.ID, status string, daysAgo int) shared.ID {
	t.Helper()
	id := freshID(t)
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO webhook_delivery (id, tenant_id, subscription_id, event_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		id.String(), s.tenant.String(), subscription.String(), freshID(t).String(), status,
		now.AddDate(0, 0, -daysAgo)); err != nil {
		t.Fatalf("seeding the delivery: %v", err)
	}
	return id
}

// livingJob seeds the delivery job a pending attempt waits for.
func (s *suite) livingJob(t *testing.T, delivery shared.ID, state string) {
	t.Helper()
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO job (id, tenant_id, kind, payload, state)
		VALUES ($1, $2, 'webhook.deliver', jsonb_build_object('delivery_id', $3::text), $4)`,
		freshID(t).String(), s.tenant.String(), delivery.String(), state); err != nil {
		t.Fatalf("seeding the delivery job: %v", err)
	}
}

func (s *suite) deliveryExists(t *testing.T, id shared.ID) bool {
	t.Helper()
	return s.count(t, `SELECT count(*) FROM webhook_delivery WHERE id = $1`, id.String()) == 1
}

// UC-PRV-08 check 2, the delivery log's part: an attempt goes thirty days after it was made, in
// its own workspace only - except a pending one a living job still names.
func TestWebhookDeliveriesGoAfterThirtyDays(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	other := newSuite(t, 24*time.Hour)
	subscription := s.subscription(t)

	gone := []shared.ID{
		s.delivery(t, subscription, "SUCCEEDED", 31),
		s.delivery(t, subscription, "FAILED", 31),
		s.delivery(t, subscription, "DEAD_LETTER", 31),
		// A pending attempt whose job dead-lettered and was purged: nothing will read it again.
		s.delivery(t, subscription, "PENDING", 31),
	}
	recent := s.delivery(t, subscription, "SUCCEEDED", 29)
	waiting := s.delivery(t, subscription, "PENDING", 31)
	s.livingJob(t, waiting, "PENDING")
	running := s.delivery(t, subscription, "PENDING", 31)
	s.livingJob(t, running, "RUNNING")
	theirs := other.delivery(t, other.subscription(t), "SUCCEEDED", 31)

	outcome := s.sweep(t)

	for _, id := range gone {
		if s.deliveryExists(t, id) {
			t.Errorf("the attempt %s outlived the period", id)
		}
	}
	for name, id := range map[string]shared.ID{
		"recent": recent, "waiting for its job": waiting, "with its job running": running,
	} {
		if !s.deliveryExists(t, id) {
			t.Errorf("the %s attempt was removed", name)
		}
	}
	if !other.deliveryExists(t, theirs) {
		t.Fatal("one workspace's run removed another workspace's delivery")
	}
	if outcome.Removed < len(gone) {
		t.Errorf("the pass reports %+v, want at least %d removed", outcome, len(gone))
	}
}

// A hold on the workspace keeps the log: counted as held, and the pass is the end.
func TestAHoldOnTheWorkspaceKeepsTheDeliveryLog(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	held := s.delivery(t, s.subscription(t), "SUCCEEDED", 31)
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO legal_hold (id, tenant_id, scope_kind, scope_id, reason, placed_by)
		VALUES ($1, $2, 'TENANT', NULL, 'Pending litigation', $3)`,
		freshID(t).String(), s.tenant.String(), s.author.String()); err != nil {
		t.Fatalf("placing the hold: %v", err)
	}

	outcome := s.sweep(t)

	if !s.deliveryExists(t, held) {
		t.Fatal("a delivery was removed under a hold on the workspace")
	}
	if outcome.Blocked[lifecycle.BlockedByLegalHold] == 0 {
		t.Errorf("the pass reports %+v, want the held delivery counted", outcome)
	}
}
