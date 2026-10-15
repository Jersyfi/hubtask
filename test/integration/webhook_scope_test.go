// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-INT-01 check 9: a subscription's scope is stored and read back by every statement that reads
// a subscription, and widening it to the workspace stores the workspace again.
func TestASubscriptionsScopeRoundTrips(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub := freshID(t)
	subscription, err := domain.NewWebhookSubscription(domain.NewWebhookSubscriptionInput{
		ID: freshID(t), TenantID: tenantA, CreatedBy: authorA,
		TargetURL:  "https://example.org/hooks/" + freshName(t),
		EventTypes: []string{string(event.ItemCreated)},
		Scope:      domain.WebhookScope{Level: domain.ScopeHub, ID: hub},
		Now:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("building the subscription: %v", err)
	}
	work := postgres.NewUnitOfWork(appPool(ctx, t))
	store := postgres.NewWebhookSubscriptionRepository()
	inA := func(fn func(context.Context) error) {
		t.Helper()
		if err := work.Within(ctx, persistence.Scope{TenantID: tenantA}, fn); err != nil {
			t.Fatal(err)
		}
	}

	inA(func(ctx context.Context) error {
		return store.Insert(ctx, repository.StoredSubscription{
			Subscription: subscription,
			Secret:       repository.SealedSecret{KeyID: "k1", Ciphertext: []byte("sealed-secret")},
		})
	})
	inA(func(ctx context.Context) error {
		found, err := store.Find(ctx, subscription.ID)
		if err != nil {
			return err
		}
		if found.Subscription.Scope != (domain.WebhookScope{Level: domain.ScopeHub, ID: hub}) {
			t.Errorf("Find read the scope as %+v", found.Subscription.Scope)
		}
		wanting, err := store.WantingEvent(ctx, event.ItemCreated)
		if err != nil {
			return err
		}
		for _, each := range wanting {
			if each.Subscription.ID == subscription.ID && each.Subscription.Scope.ID != hub {
				t.Errorf("WantingEvent read the scope as %+v", each.Subscription.Scope)
			}
		}

		widened := found.Subscription
		widened.Scope = domain.WebhookScope{Level: domain.ScopeTenant}
		changed, err := store.Update(ctx, widened, found.Subscription.Version)
		if err != nil || !changed {
			t.Fatalf("widening: (%v, %v)", changed, err)
		}
		again, err := store.Find(ctx, subscription.ID)
		if err != nil {
			return err
		}
		if !again.Subscription.Scope.IsWorkspace() {
			t.Errorf("the widened scope reads %+v", again.Subscription.Scope)
		}
		return nil
	})
}

// An event the subscription's condition could not be evaluated for is a row of its delivery log
// with the expression's code, no attempt, and the status a reader narrows to.
func TestAHeldBackEventIsARowOfTheDeliveryLog(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	subscription := seedSubscription(ctx, t, tenantA)
	held, err := domain.NewFilterFailedDelivery(freshID(t), tenantA, subscription.ID, freshID(t),
		"expression.timed_out", time.Now().UTC())
	if err != nil {
		t.Fatalf("building the row: %v", err)
	}
	work := postgres.NewUnitOfWork(appPool(ctx, t))
	deliveries := postgres.NewWebhookDeliveryRepository()
	if err := work.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		if err := deliveries.Insert(ctx, held); err != nil {
			return err
		}
		listed, err := deliveries.List(ctx, repository.DeliveryQuery{
			SubscriptionID: subscription.ID, Status: domain.DeliveryFilterFailed, PageSize: 10,
		})
		if err != nil {
			return err
		}
		if len(listed) != 1 || listed[0].ErrorCode != "expression.timed_out" || listed[0].Attempt != 0 ||
			listed[0].IsDeadLettered() {
			t.Errorf("the log answered %+v", listed)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
