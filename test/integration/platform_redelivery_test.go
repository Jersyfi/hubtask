// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A redelivery starts a fresh round of attempts however the last one ended, and a dead letter whose
// event was swept is neither requeued nor counted - so the count falls to zero and A-21 resolves.
func TestARedeliveryStartsAFreshRoundAndLeavesExpiredOnes(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	deliveries := postgres.NewPlatformDeliveryRepository()
	subscriptions := postgres.NewPlatformSubscriptionRepository()
	sub := newPlatformSubscription(ctx, t, uow)
	cut, _ := integration.NewPlatformDelivery(freshID(t), sub.ID, freshID(t), string(event.WorkspaceCreated), time.Now())
	expired, _ := integration.NewPlatformDelivery(freshID(t), sub.ID, freshID(t), string(event.WorkspaceCreated), time.Now())
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		for _, d := range []integration.PlatformDelivery{cut, expired} {
			if _, err := deliveries.Insert(ctx, d); err != nil {
				return err
			}
		}
		// Three attempts missed, then held back by a pause: a round cut short at three.
		d := cut
		for range 3 {
			d = d.Missed(503, "webhooks.target_unavailable", time.Now(), time.Now())
			if _, err := deliveries.Record(ctx, d); err != nil {
				return err
			}
		}
		if _, err := deliveries.Record(ctx, d.Withheld("admin.subscription_paused", time.Now())); err != nil {
			return err
		}
		_, err := deliveries.Record(ctx, expired.Expired(time.Now()))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	read := func() integration.PlatformSubscription {
		t.Helper()
		var stored adminrepo.StoredPlatformSubscription
		if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			found, err := subscriptions.Find(ctx, sub.ID)
			stored = found
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return stored.Subscription
	}
	if got := read().DeadLetters; got != 1 {
		t.Fatalf("%d dead letters counted, want the one that can be redelivered", got)
	}
	var requeued []shared.ID
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		ids, err := deliveries.Requeue(ctx, sub.ID, time.Now(), 100)
		requeued = ids
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(requeued) != 1 || requeued[0] != cut.ID {
		t.Fatalf("requeued %v, want only the delivery that can be sent", requeued)
	}
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		again, err := deliveries.Find(ctx, cut.ID)
		if err == nil && (again.Attempt != integration.MaxDeliveryAttempts || again.Status != integration.DeliveryPending) {
			t.Errorf("requeued as %+v, want a fresh round from attempt %d", again, integration.MaxDeliveryAttempts)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := read().DeadLetters; got != 0 {
		t.Errorf("%d dead letters counted after the redelivery", got)
	}
}
