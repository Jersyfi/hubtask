// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-INT-12 check 1: a chat room's format, room and language are stored and read back, and the
// workspace next door does not see the room.
func TestAChatRoomRoundTripsAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	subscription, err := domain.NewWebhookSubscription(domain.NewWebhookSubscriptionInput{
		ID: freshID(t), TenantID: tenantA, CreatedBy: authorA,
		TargetURL:  "https://matrix.example.org",
		EventTypes: []string{string(event.ItemCompleted)},
		Chat:       domain.ChatTarget{Format: domain.FormatMatrix, Room: fmt.Sprintf("!room%d:example.org", fixtureCounter.Add(1)), Locale: "de"},
		Now:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("building the subscription: %v", err)
	}
	work := postgres.NewUnitOfWork(appPool(ctx, t))
	store := postgres.NewWebhookSubscriptionRepository()
	in := func(tenant shared.ID, fn func(context.Context) error) {
		t.Helper()
		if err := work.Within(ctx, persistence.Scope{TenantID: tenant}, fn); err != nil {
			t.Fatal(err)
		}
	}

	in(tenantA, func(ctx context.Context) error {
		return store.Insert(ctx, repository.StoredSubscription{
			Subscription: subscription,
			Secret:       repository.SealedSecret{KeyID: "k1", Ciphertext: []byte("sealed-token")},
		})
	})
	in(tenantA, func(ctx context.Context) error {
		found, err := store.Find(ctx, subscription.ID)
		if err != nil {
			return err
		}
		got := found.Subscription
		if got.Format != domain.FormatMatrix || got.Room != subscription.Room || got.Locale != "de" {
			t.Errorf("Find read the room as %+v", got)
		}
		return nil
	})
	in(tenantB, func(ctx context.Context) error {
		if _, err := store.Find(ctx, subscription.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door read the room: %v", err)
		}
		return nil
	})
}
