// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	integrationrepo "github.com/Jersyfi/hubtask/core/application/repository/integration"
	integrationservice "github.com/Jersyfi/hubtask/core/application/service/integration"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	"github.com/Jersyfi/hubtask/core/domain/event"
	integrationdomain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// SY-9 (offline-sync.md §8): four hundred offline changes to forty entries under one subscription
// produce forty deliveries, the outbox holds four hundred events, and every event carries the
// device's moment as occurred_at and the server's as received_at.
func TestFourHundredOfflineChangesToFortyEntriesOweFortyDeliveries(t *testing.T) {
	ctx := context.Background()
	// A workspace of its own: a subscription is workspace-wide, and one in the shared tenant
	// would collect every other test's events.
	tenant, author := seedMaterialisingTenant(ctx, t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
		freshID(t).String(), tenant.String(), author.String()); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}
	_, collection := hubWithCollection(ctx, t, tenant, author)
	const entries, changesEach = 40, 10
	items := make([]shared.ID, entries)
	for i := range items {
		items[i] = seedTask(ctx, t, tenant, author, collection)
	}
	subscription := seedSubscriptionFor(ctx, t, tenant, author, string(event.ItemUpdated))

	// The device: ten changes to each of forty entries, made over the last four minutes - within
	// the skew, so that the readings stand as the moments the person acted.
	push := mergingPushFor(ctx, t)
	device := freshID(t)
	wall := time.Now().Add(-4 * time.Minute)
	mutations := make([]syncservice.Mutation, 0, entries*changesEach)
	for round := range changesEach {
		for i, item := range items {
			reading, err := shared.NewHLC(wall.Add(time.Duration(round*entries+i)*100*time.Millisecond), 1, device.String())
			if err != nil {
				t.Fatalf("building a reading: %v", err)
			}
			mutations = append(mutations, patchMutation(t, item, "title", fmt.Sprintf("Draft %d", round+1), reading))
		}
	}
	response, err := push.Push(ctx, pushActor(tenant, author), syncservice.PushRequest{DeviceID: device, Mutations: mutations})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	for _, result := range response.Results {
		if result.Result != syncdomain.Applied {
			t.Fatalf("a change was answered %+v", result)
		}
	}

	// The fan-out over every event of the push, the dispatcher's way.
	fanOut := integrationservice.FanOut{
		Subscriptions: postgres.NewWebhookSubscriptionRepository(),
		Deliveries:    postgres.NewWebhookDeliveryRepository(),
		Jobs:          jobQueue(t), Clock: clockadapter.System{},
		IDs: clockadapter.NewUUIDv7(clockadapter.System{}),
	}
	for round := 0; ; round++ {
		dispatchOnce(ctx, t, tenant, fanOut)
		if countIn(ctx, t, `SELECT count(*) FROM outbox_event WHERE tenant_id = $1 AND dispatched_at IS NULL`, tenant.String()) == 0 {
			break
		}
		if round > 20 {
			t.Fatalf("the outbox does not drain")
		}
	}

	// Four hundred events, one push, both clocks; forty deliveries, each standing for its
	// entry's last change.
	if got := countIn(ctx, t, `SELECT count(*) FROM outbox_event WHERE tenant_id = $1 AND event_type = $2 AND push_id IS NOT NULL`,
		tenant.String(), string(event.ItemUpdated)); got != entries*changesEach {
		t.Errorf("the outbox holds %d pushed updates, want %d", got, entries*changesEach)
	}
	if got := countIn(ctx, t, `SELECT count(DISTINCT push_id) FROM outbox_event WHERE tenant_id = $1 AND push_id IS NOT NULL`, tenant.String()); got != 1 {
		t.Errorf("the events name %d pushes, want one", got)
	}
	// occurred_at is the device's moment, inside the four minutes the readings span; received_at
	// is the writer's clock - the fixture's fixed one here, the system's in production - and is
	// never the device's.
	if got := countIn(ctx, t, `SELECT count(*) FROM outbox_event WHERE tenant_id = $1 AND push_id IS NOT NULL
		AND occurred_at >= $2 AND occurred_at <= $3 AND received_at = $4`,
		tenant.String(), wall.Add(-time.Second), wall.Add(time.Minute), created); got != entries*changesEach {
		t.Errorf("%d events carry the device's moment and the writer's clock, want all %d", got, entries*changesEach)
	}
	if got := countIn(ctx, t, `SELECT count(*) FROM webhook_delivery WHERE subscription_id = $1`, subscription.String()); got != entries {
		t.Errorf("%d deliveries were recorded, want one per entry", got)
	}
	if got := countIn(ctx, t, `SELECT count(*) FROM webhook_delivery d JOIN outbox_event e ON e.id = d.event_id
		WHERE d.subscription_id = $1 AND e.payload->'change_set'->'title'->>'to' = $2`, subscription.String(), fmt.Sprintf("Draft %d", changesEach)); got != entries {
		t.Errorf("%d deliveries stand for the entry's last change, want all %d", got, entries)
	}
}

// seedSubscriptionFor is seedSubscription for a tenant of one's own and an event type of one's
// choosing.
func seedSubscriptionFor(ctx context.Context, t *testing.T, tenant, author shared.ID, eventType string) shared.ID {
	t.Helper()
	subscription, err := integrationdomain.NewWebhookSubscription(integrationdomain.NewWebhookSubscriptionInput{
		ID: freshID(t), TenantID: tenant, CreatedBy: author,
		TargetURL:  "https://example.org/hooks/" + freshName(t),
		EventTypes: []string{eventType},
		Now:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("building the subscription: %v", err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewWebhookSubscriptionRepository().Insert(ctx, integrationrepo.StoredSubscription{
			Subscription: subscription,
			Secret:       integrationrepo.SealedSecret{KeyID: "k1", Ciphertext: []byte("sealed-secret")},
		})
	}); err != nil {
		t.Fatalf("seeding the subscription: %v", err)
	}
	return subscription.ID
}

// The cross-tenant negative for the two methods the collapse added (gate SG-3): a push's pending
// delivery is not found from the other tenant, and cannot be repointed from it.
func TestAPushsDeliveryIsNeitherFoundNorRepointedAcrossTheTenantBoundary(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	subscription := seedSubscription(ctx, t, tenantA)
	deliveries := postgres.NewWebhookDeliveryRepository()
	key := integrationdomain.CollapseKey{PushID: freshID(t), Subject: "item/" + freshID(t).String(), EventType: string(event.ItemCreated)}
	delivery, err := integrationdomain.NewWebhookDelivery(freshID(t), tenantA, subscription.ID, freshID(t), 1, time.Now())
	if err != nil {
		t.Fatalf("building the delivery: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return deliveries.Insert(ctx, delivery.Collapsed(key))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	newer := freshID(t)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, found, err := deliveries.FindPendingOfPush(ctx, subscription.ID, key); err != nil || found {
			t.Errorf("the other tenant found the push's delivery (%v, %v)", found, err)
		}
		repointed, err := deliveries.Repoint(ctx, delivery.ID, newer)
		if err != nil || repointed {
			t.Errorf("the other tenant repointed the delivery (%v, %v)", repointed, err)
		}
		return nil
	}); err != nil {
		t.Fatalf("asking as the other tenant: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		found, ok, err := deliveries.FindPendingOfPush(ctx, subscription.ID, key)
		if err != nil || !ok || found.ID != delivery.ID || found.Collapse != key {
			t.Errorf("the own tenant found %+v (%v, %v)", found, ok, err)
		}
		return nil
	}); err != nil {
		t.Fatalf("asking: %v", err)
	}
}
