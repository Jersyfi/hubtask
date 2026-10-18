// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webhook

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
)

type planLeftOut struct{ leftOut entitlement.LeftOut }

func (p planLeftOut) LeftOut(context.Context) (entitlement.LeftOut, error) { return p.leftOut, nil }

// The worker: a delivery queued before the plan left webhooks out sends nothing, settles nothing -
// so nothing counts towards switching the subscription off - and asks for no retry; the row stays
// as it was (multi-tenancy.md §4.2).
func TestNoDeliveryLeavesWhileThePlanLeavesWebhooksOut(t *testing.T) {
	h := newHarness(t, 1)
	h.deliverer.Entitlements = planLeftOut{leftOut: entitlement.LeftOut{entitlement.Webhooks}}

	if _, err := h.deliverer.Run(context.Background(), job()); err != nil {
		t.Fatalf("running: %v", err)
	}
	if len(h.calls.requests) != 0 {
		t.Fatalf("%d calls left for a plan without webhooks", len(h.calls.requests))
	}
	if len(h.results.delivered) != 0 || len(h.results.failures) != 0 || len(h.queued.requests) != 0 {
		t.Errorf("settled %v / %v, retried %d", h.results.delivered, h.results.failures, len(h.queued.requests))
	}
	if stored := h.store.deliveries[deliveryID]; stored.Status != domain.DeliveryPending {
		t.Errorf("the delivery is %q", stored.Status)
	}

	// Another feature left out sends as ever.
	h.deliverer.Entitlements = planLeftOut{leftOut: entitlement.LeftOut{entitlement.AutomationRules}}
	if _, err := h.deliverer.Run(context.Background(), job()); err != nil {
		t.Fatalf("running: %v", err)
	}
	if len(h.calls.requests) != 1 {
		t.Fatalf("%d calls with webhooks included", len(h.calls.requests))
	}
}
