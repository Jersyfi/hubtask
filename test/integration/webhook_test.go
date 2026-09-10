// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// seedSubscription writes one subscription in the tenant given, through the repository, so that
// the test exercises the statement the application uses rather than an INSERT of its own.
func seedSubscription(ctx context.Context, t *testing.T, tenant shared.ID) domain.WebhookSubscription {
	t.Helper()

	subscription, err := domain.NewWebhookSubscription(domain.NewWebhookSubscriptionInput{
		ID: freshID(t), TenantID: tenant, CreatedBy: authorA,
		TargetURL:  "https://example.org/hooks/" + freshName(t),
		EventTypes: []string{string(event.ItemCreated)},
		Now:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("building the subscription: %v", err)
	}

	work := postgres.NewUnitOfWork(appPool(ctx, t))
	if err := work.Within(ctx, persistence.Scope{TenantID: tenant}, func(txCtx context.Context) error {
		return postgres.NewWebhookSubscriptionRepository().Insert(txCtx, repository.StoredSubscription{
			Subscription: subscription,
			Secret:       repository.SealedSecret{KeyID: "k1", Ciphertext: []byte("sealed-secret")},
		})
	}); err != nil {
		t.Fatalf("seeding the subscription: %v", err)
	}
	return subscription
}

// The cross-tenant negative gate SG-3 requires, for every read and write of the subscriptions.
// Row level security is what makes it pass: none of the statements carries a tenant condition.
func TestASubscriptionOfOneTenantIsInvisibleInAnother(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	subscription := seedSubscription(ctx, t, tenantA)
	work := postgres.NewUnitOfWork(appPool(ctx, t))
	store := postgres.NewWebhookSubscriptionRepository()

	inB := func(fn func(context.Context) error) error {
		return work.Within(ctx, persistence.Scope{TenantID: tenantB}, fn)
	}

	if err := inB(func(txCtx context.Context) error {
		_, err := store.Find(txCtx, subscription.ID)
		if !isNotFound(err) {
			t.Errorf("Find: error = %v, want not found", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("reading from the wrong tenant: %v", err)
	}

	if err := inB(func(txCtx context.Context) error {
		listed, err := store.List(txCtx)
		if err != nil {
			return err
		}
		for _, found := range listed {
			if found.ID == subscription.ID {
				t.Error("List answered another tenant's subscription")
			}
		}

		wanting, err := store.WantingEvent(txCtx, event.ItemCreated)
		if err != nil {
			return err
		}
		for _, found := range wanting {
			if found.Subscription.ID == subscription.ID {
				t.Error("WantingEvent answered another tenant's subscription")
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("listing from the wrong tenant: %v", err)
	}

	// The three writes reach nothing, and report that they changed nothing rather than failing:
	// "no such row" is what a policy looks like from outside.
	if err := inB(func(txCtx context.Context) error {
		changed, err := store.Update(txCtx, subscription.Paused(), subscription.Version)
		if err != nil {
			return err
		}
		if changed {
			t.Error("another tenant updated a subscription")
		}

		rotated, err := store.Rotate(txCtx, subscription.ID,
			repository.SealedSecret{KeyID: "k1", Ciphertext: []byte("theirs")},
			time.Now().UTC().Add(time.Hour), subscription.Version)
		if err != nil {
			return err
		}
		if rotated {
			t.Error("another tenant rotated a subscription's secret")
		}

		deleted, err := store.Delete(txCtx, subscription.ID)
		if err != nil {
			return err
		}
		if deleted {
			t.Error("another tenant deleted a subscription")
		}
		return nil
	}); err != nil {
		t.Fatalf("writing from the wrong tenant: %v", err)
	}

	// And it is still there, which is the assertion the three booleans above only imply.
	if err := work.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(txCtx context.Context) error {
		stored, err := store.Find(txCtx, subscription.ID)
		if err != nil {
			return err
		}
		if stored.Subscription.State != domain.SubscriptionActive {
			t.Errorf("the subscription is %s after another tenant's writes", stored.Subscription.State)
		}
		if string(stored.Secret.Ciphertext) != "sealed-secret" {
			t.Error("another tenant's rotation reached the secret")
		}
		return nil
	}); err != nil {
		t.Fatalf("reading it back: %v", err)
	}
}

// The deliveries are a second table with its own statements, and "the subscriptions are bounded"
// proves nothing about them.
func TestADeliveryOfOneTenantIsInvisibleInAnother(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	subscription := seedSubscription(ctx, t, tenantA)
	work := postgres.NewUnitOfWork(appPool(ctx, t))
	deliveries := postgres.NewWebhookDeliveryRepository()

	delivery, err := domain.NewWebhookDelivery(
		freshID(t), tenantA, subscription.ID, freshID(t), 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("building the delivery: %v", err)
	}
	if err := work.Within(ctx, persistence.Scope{TenantID: tenantA}, func(txCtx context.Context) error {
		return deliveries.Insert(txCtx, delivery)
	}); err != nil {
		t.Fatalf("seeding the delivery: %v", err)
	}

	if err := work.Within(ctx, persistence.Scope{TenantID: tenantB}, func(txCtx context.Context) error {
		if _, err := deliveries.Find(txCtx, delivery.ID); !isNotFound(err) {
			t.Errorf("Find: error = %v, want not found", err)
		}

		listed, err := deliveries.List(txCtx, repository.DeliveryQuery{
			SubscriptionID: subscription.ID, PageSize: 50,
		})
		if err != nil {
			return err
		}
		if len(listed) != 0 {
			t.Errorf("another tenant listed %d deliveries", len(listed))
		}

		// The outcome write reaches no row, and says nothing about it - which is the same shape
		// the touch of an access token has.
		return deliveries.RecordOutcome(txCtx, repository.DeliveryOutcome{
			ID: delivery.ID, Status: domain.DeliverySucceeded, ResponseStatus: 200,
		})
	}); err != nil {
		t.Fatalf("reaching from the wrong tenant failed for the wrong reason: %v", err)
	}

	if err := work.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(txCtx context.Context) error {
		found, err := deliveries.Find(txCtx, delivery.ID)
		if err != nil {
			return err
		}
		if found.Status != domain.DeliveryPending {
			t.Errorf("another tenant settled a delivery: it is %s", found.Status)
		}
		return nil
	}); err != nil {
		t.Fatalf("reading the delivery back: %v", err)
	}
}

// A webhook aggregate read back has to be one the domain can act on, and the field that decides it
// is one a query can leave out without anything looking wrong.
//
// `Retried` and `Replayed` both build the next attempt out of the row that was read, and both
// refuse a delivery whose tenant is zero. The listing and the find left `tenant_id` unselected -
// row level security already scopes the row, so nothing in a unit test or a service test missed
// it - and the consequence was that **every** retry and **every** replay answered
// `webhooks.delivery_incomplete`: the first failed attempt rolled its own outcome back with it, so
// a delivery stayed `PENDING` for ever, no subscription ever reached its failure run, and nothing
// could be dead-lettered for a replay to act on. Only a real read shows it.
func TestADeliveryReadBackCanProduceItsOwnNextAttempt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	subscription := seedSubscription(ctx, t, tenantA)
	work := postgres.NewUnitOfWork(appPool(ctx, t))
	deliveries := postgres.NewWebhookDeliveryRepository()
	subscriptions := postgres.NewWebhookSubscriptionRepository()

	delivery, err := domain.NewWebhookDelivery(
		freshID(t), tenantA, subscription.ID, freshID(t), 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("building the delivery: %v", err)
	}
	if err := work.Within(ctx, persistence.Scope{TenantID: tenantA}, func(txCtx context.Context) error {
		if err := deliveries.Insert(txCtx, delivery); err != nil {
			return err
		}
		// Failed, because that is the state a retry follows from.
		return deliveries.RecordOutcome(txCtx, repository.DeliveryOutcome{
			ID: delivery.ID, Status: domain.DeliveryFailed, ErrorCode: "webhooks.target_unreachable",
			NextAttemptAt: time.Now().UTC().Add(time.Minute),
		})
	}); err != nil {
		t.Fatalf("seeding the failed delivery: %v", err)
	}

	if err := work.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(txCtx context.Context) error {
		found, err := deliveries.Find(txCtx, delivery.ID)
		if err != nil {
			return err
		}
		if found.TenantID != tenantA {
			t.Fatalf("the delivery came back with tenant %v, want %v", found.TenantID, tenantA)
		}
		// The two the whole ladder rests on. A failure here is what an operator sees as a
		// subscription that never fails and a dead letter that never arrives.
		next, err := found.Retried(freshID(t), time.Now().UTC())
		if err != nil {
			t.Fatalf("a delivery read back could not be retried: %v", err)
		}
		if next.Attempt != found.Attempt+1 {
			t.Errorf("the retry is attempt %d, want %d", next.Attempt, found.Attempt+1)
		}
		if next.EventID != found.EventID {
			t.Errorf("the retry carries event %v, want %v", next.EventID, found.EventID)
		}

		listed, err := deliveries.List(txCtx, repository.DeliveryQuery{
			SubscriptionID: subscription.ID, PageSize: 50,
		})
		if err != nil {
			return err
		}
		if len(listed) != 1 {
			t.Fatalf("listed %d deliveries, want 1", len(listed))
		}
		if listed[0].TenantID != tenantA {
			t.Errorf("the listed delivery carries tenant %v, want %v", listed[0].TenantID, tenantA)
		}

		// The subscription had the same hole, with the same cause and a different symptom: every
		// auditable operation on one writes its entry under `subscription.TenantID`, and the audit
		// port refuses an entry whose tenant is zero. That is what answered `audit.entry_incomplete`
		// on every replay.
		stored, err := subscriptions.Find(txCtx, subscription.ID)
		if err != nil {
			return err
		}
		if stored.Subscription.TenantID != tenantA {
			t.Errorf(
				"the subscription came back with tenant %v, want %v",
				stored.Subscription.TenantID, tenantA,
			)
		}
		return nil
	}); err != nil {
		t.Fatalf("reading the delivery back: %v", err)
	}
}

// The round trip the use cases depend on, and the one property only a database can show: what the
// rotation writes is one statement, so a subscription never has a new secret and no grace.
func TestARotationMovesTheSecretAndItsGraceTogether(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	subscription := seedSubscription(ctx, t, tenantA)
	work := postgres.NewUnitOfWork(appPool(ctx, t))
	store := postgres.NewWebhookSubscriptionRepository()
	until := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	if err := work.Within(ctx, persistence.Scope{TenantID: tenantA}, func(txCtx context.Context) error {
		changed, err := store.Rotate(txCtx, subscription.ID,
			repository.SealedSecret{KeyID: "k2", Ciphertext: []byte("the-new-sealed-secret")},
			until, subscription.Version)
		if err != nil {
			return err
		}
		if !changed {
			t.Fatal("the rotation matched no row")
		}
		return nil
	}); err != nil {
		t.Fatalf("rotating: %v", err)
	}

	if err := work.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(txCtx context.Context) error {
		stored, err := store.Find(txCtx, subscription.ID)
		if err != nil {
			return err
		}
		if string(stored.Secret.Ciphertext) != "the-new-sealed-secret" || stored.Secret.KeyID != "k2" {
			t.Errorf("the current secret is %+v", stored.Secret)
		}
		if string(stored.Previous.Ciphertext) != "sealed-secret" {
			t.Errorf("the previous secret is %+v", stored.Previous)
		}
		if !stored.Subscription.PreviousSecretUntil.Equal(until) {
			t.Errorf("the grace ends at %v, want %v", stored.Subscription.PreviousSecretUntil, until)
		}
		if stored.Subscription.Version != subscription.Version+1 {
			t.Errorf("version = %d", stored.Subscription.Version)
		}
		return nil
	}); err != nil {
		t.Fatalf("reading it back: %v", err)
	}
}

// isNotFound is the answer a tenant boundary produces: the row is invisible rather than forbidden,
// because anything else confirms that it exists (T-04, multi-tenancy.md 2).
func isNotFound(err error) bool { return errors.Is(err, shared.ErrNotFound) }
