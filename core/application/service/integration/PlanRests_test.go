// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The workspace's plan as the fan-out and the writer read it, changeable between two calls.
type planLeftOut struct{ leftOut entitlement.LeftOut }

func (p *planLeftOut) LeftOut(context.Context) (entitlement.LeftOut, error) { return p.leftOut, nil }

// A plan without webhooks: an event owes no delivery and queues nothing; the subscription and its
// secret stay. Included again, the next event is delivered (multi-tenancy.md §4.2).
func TestNoDeliveryIsOwedWhileThePlanLeavesWebhooksOut(t *testing.T) {
	h := withSubscription(t)
	queued := &jobs{}
	plan := &planLeftOut{leftOut: entitlement.LeftOut{entitlement.Webhooks}}
	fan := fanOut(h, queued)
	fan.Entitlements = plan

	if err := fan.Deliver(t.Context(), anEvent(t, event.ItemCreated)); err != nil {
		t.Fatalf("fanning out: %v", err)
	}
	if len(h.delivered.rows) != 0 || len(queued.requests) != 0 {
		t.Fatalf("resting, %d deliveries and %d jobs", len(h.delivered.rows), len(queued.requests))
	}
	if stored, kept := h.store.rows[hookID]; !kept || stored.Subscription.State != domain.SubscriptionActive ||
		len(stored.Secret.Ciphertext) == 0 {
		t.Errorf("the subscription did not stay as it was: %+v", stored)
	}

	plan.leftOut = entitlement.LeftOut{entitlement.AutomationRules}
	if err := fan.Deliver(t.Context(), anEvent(t, event.ItemCreated)); err != nil {
		t.Fatalf("fanning out: %v", err)
	}
	if len(h.delivered.rows) != 1 || len(queued.requests) != 1 {
		t.Fatalf("included again, %d deliveries and %d jobs", len(h.delivered.rows), len(queued.requests))
	}
}

// Resting, a subscription may be paused and nothing else: no new target, no resuming.
func TestARestingSubscriptionMayOnlyBePaused(t *testing.T) {
	h := withSubscription(t)
	h.writer.Entitlements = &planLeftOut{leftOut: entitlement.LeftOut{entitlement.Webhooks}}
	update := UpdateWebhookSubscription{Writer: h.writer}

	target := "https://elsewhere.example/hooks"
	_, err := update.Execute(t.Context(), actor(), UpdateWebhookSubscriptionCommand{ID: hookID, TargetURL: &target})
	if !errors.Is(err, shared.ErrForbidden) || shared.AsError(err).DetailCode != entitlement.NotIncludedCode ||
		shared.AsError(err).Params["feature"] != string(entitlement.Webhooks) {
		t.Fatalf("a new target answered %v", err)
	}

	paused := string(domain.SubscriptionPaused)
	held, err := update.Execute(t.Context(), actor(), UpdateWebhookSubscriptionCommand{ID: hookID, State: &paused})
	if err != nil || held.State != domain.SubscriptionPaused {
		t.Fatalf("pausing answered %+v, %v", held, err)
	}

	active := string(domain.SubscriptionActive)
	if _, err := update.Execute(t.Context(), actor(), UpdateWebhookSubscriptionCommand{ID: hookID, State: &active}); shared.AsError(err).DetailCode != entitlement.NotIncludedCode {
		t.Fatalf("resuming answered %v", err)
	}
	pausedWithTarget := UpdateWebhookSubscriptionCommand{ID: hookID, State: &paused, TargetURL: &target}
	if _, err := update.Execute(t.Context(), actor(), pausedWithTarget); shared.AsError(err).DetailCode != entitlement.NotIncludedCode {
		t.Fatalf("a pause carrying a new target answered %v", err)
	}

	h.writer.Entitlements = &planLeftOut{}
	if resumed, err := (UpdateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(),
		UpdateWebhookSubscriptionCommand{ID: hookID, State: &active}); err != nil || resumed.State != domain.SubscriptionActive {
		t.Fatalf("included again, resuming answered %+v, %v", resumed, err)
	}
}
