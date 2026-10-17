// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/application/service/platform"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// newPlatformSubscription writes one under the system scope and removes it after the test.
func newPlatformSubscription(ctx context.Context, t *testing.T, uow persistence.UnitOfWork) integration.PlatformSubscription {
	t.Helper()
	subscriptions := postgres.NewPlatformSubscriptionRepository()
	sub, err := integration.NewPlatformSubscription(freshID(t), "https://shop.example/hubtask", false, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		return subscriptions.Insert(ctx, sub, adminrepo.PlatformSecret{Ciphertext: []byte("sealed"), KeyID: "k1"})
	}); err != nil {
		t.Fatalf("subscribing: %v", err)
	}
	t.Cleanup(func() {
		_ = uow.Within(context.WithoutCancel(ctx), persistence.SystemScope(), func(ctx context.Context) error {
			_, err := subscriptions.Delete(ctx, sub.ID)
			return err
		})
	})
	return sub
}

// A subscription is written under the system scope, read under the installation's with its
// sealed secret, changed only on the version read, rotated and removed.
func TestAnInstallationSubscriptionIsKept(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	subscriptions := postgres.NewPlatformSubscriptionRepository()
	sub := newPlatformSubscription(ctx, t, uow)

	read := func() adminrepo.StoredPlatformSubscription {
		t.Helper()
		var stored adminrepo.StoredPlatformSubscription
		if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			found, err := subscriptions.Find(ctx, sub.ID)
			stored = found
			return err
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		return stored
	}
	stored := read()
	if stored.Subscription.TargetURL != sub.TargetURL || string(stored.Secret.Ciphertext) != "sealed" ||
		stored.Subscription.State != integration.PlatformActive || stored.Subscription.Version != 1 {
		t.Fatalf("read %+v", stored)
	}

	paused, _ := stored.Subscription.InState(integration.PlatformPaused)
	write := func(fn func(context.Context) error) {
		t.Helper()
		if err := uow.Within(ctx, persistence.SystemScope(), fn); err != nil {
			t.Fatal(err)
		}
	}
	write(func(ctx context.Context) error {
		if ok, err := subscriptions.Update(ctx, paused, 1, time.Now()); err != nil || !ok {
			t.Errorf("the change on the version read: %v, %v", ok, err)
		}
		if ok, err := subscriptions.Update(ctx, paused, 1, time.Now()); err != nil || ok {
			t.Errorf("a change on a stale version wrote: %v, %v", ok, err)
		}
		if ok, err := subscriptions.Rotate(ctx, sub.ID, adminrepo.PlatformSecret{Ciphertext: []byte("new"), KeyID: "k2"}, time.Now()); err != nil || !ok {
			t.Errorf("rotating: %v, %v", ok, err)
		}
		return subscriptions.Outcome(ctx, sub.ID, time.Time{}, "webhooks.target_unavailable")
	})
	stored = read()
	if stored.Subscription.State != integration.PlatformPaused || string(stored.Secret.Ciphertext) != "new" ||
		stored.Subscription.LastError != "webhooks.target_unavailable" || stored.Subscription.Version != 3 {
		t.Errorf("after the changes %+v", stored)
	}
	write(func(ctx context.Context) error {
		active, err := subscriptions.Active(ctx)
		for _, id := range active {
			if id == sub.ID {
				t.Error("a paused subscription counts as active")
			}
		}
		return err
	})

	write(func(ctx context.Context) error {
		if ok, err := subscriptions.Delete(ctx, sub.ID); err != nil || !ok {
			t.Errorf("removing: %v, %v", ok, err)
		}
		return nil
	})
	err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		_, err := subscriptions.Find(ctx, sub.ID)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a removed subscription answered %v", err)
	}
}

// An event's deliveries and their jobs commit with the event, for the active subscriptions only; a
// change that rolls back leaves none, and the jobs carry no tenant, so a workspace's purge leaves
// them.
func TestAnEventIsQueuedForTheActiveSubscriptionsInItsTransaction(t *testing.T) {
	ctx := context.Background()
	pool := appPool(ctx, t)
	uow := postgres.NewUnitOfWork(pool)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	active, paused := newPlatformSubscription(ctx, t, uow), newPlatformSubscription(ctx, t, uow)
	resting, _ := paused.InState(integration.PlatformPaused)
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		_, err := postgres.NewPlatformSubscriptionRepository().Update(ctx, resting, 1, time.Now())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	recorder := platformRecorder(ids)
	recorder.Fanout = platform.Fanout{
		Subscriptions: postgres.NewPlatformSubscriptionRepository(), Deliveries: postgres.NewPlatformDeliveryRepository(),
		Jobs: postgres.NewQueue(ids, clockadapter.System{}), IDs: ids, Clock: clockadapter.System{},
	}
	workspace := seedWorkspace(ctx, t, "fanned")

	deliveries := func(sub shared.ID) []integration.PlatformDelivery {
		t.Helper()
		var page []integration.PlatformDelivery
		if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			read, err := postgres.NewPlatformDeliveryRepository().Page(ctx, sub, "", "", 50)
			page = read
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return page
	}

	refused := errors.New("refused")
	_ = uow.Within(ctx, persistence.Scope{TenantID: workspace}, func(ctx context.Context) error {
		if err := recorder.Record(ctx, event.WorkspaceSuspended, event.Actor{Kind: shared.ActorSystem}, time.Now(), nil); err != nil {
			t.Fatal(err)
		}
		return refused
	})
	if len(deliveries(active.ID)) != 0 {
		t.Fatal("a rolled-back change left a delivery")
	}
	if err := uow.Within(ctx, persistence.Scope{TenantID: workspace}, func(ctx context.Context) error {
		return recorder.Record(ctx, event.WorkspaceSuspended, event.Actor{Kind: shared.ActorSystem}, time.Now(), nil)
	}); err != nil {
		t.Fatalf("recording: %v", err)
	}
	queued := deliveries(active.ID)
	if len(queued) != 1 || queued[0].Status != integration.DeliveryPending || queued[0].EventType != string(event.WorkspaceSuspended) {
		t.Fatalf("the active subscription holds %+v", queued)
	}
	if len(deliveries(paused.ID)) != 0 {
		t.Error("a paused subscription was queued a delivery")
	}

	var tenant *string
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT tenant_id::text FROM job WHERE kind = 'platform.deliver' AND payload->>'delivery_id' = $1`,
		queued[0].ID.String()).Scan(&tenant); err != nil {
		t.Fatalf("the delivery's job: %v", err)
	}
	if tenant != nil {
		t.Errorf("the delivery's job belongs to workspace %s", *tenant)
	}
	_, _ = adminPool(ctx, t).Exec(ctx, `DELETE FROM job WHERE kind = 'platform.deliver' AND payload->>'delivery_id' = $1`, queued[0].ID.String())
}
