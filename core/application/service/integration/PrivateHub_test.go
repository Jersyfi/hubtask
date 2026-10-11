// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var privateCollection = shared.ID("01936f2a-7c1e-7000-8000-000000000f31")

// hiddenTo answers which accounts a private hub is hidden from.
type hiddenTo map[shared.ID]bool

func (h hiddenTo) Hidden(_ context.Context, actor appshared.ActorContext, path []identity.Scope) (bool, error) {
	return h[actor.AccountID] && len(path) > 1, nil
}

// D5, ADR-0073 §1: a subscription delivers an event of a private hub only when its creator
// reaches the hub - an administrator's subscription does not read a family member's hub.
func TestASubscriptionDeliversAPrivateHubsEventOnlyToItsReach(t *testing.T) {
	inHub := anEvent(t, event.ItemCreated)
	inHub.Payload = map[string]any{"collection_id": privateCollection.String()}

	for name, tc := range map[string]struct {
		hidden bool
		want   int
	}{"its creator outside the hub": {true, 0}, "its creator in the hub": {false, 1}} {
		t.Run(name, func(t *testing.T) {
			h := withSubscription(t)
			queued := &jobs{}
			out := fanOut(h, queued)
			out.Reach = hiddenTo{author: tc.hidden}

			if err := out.Deliver(t.Context(), inHub); err != nil {
				t.Fatalf("fanning out: %v", err)
			}
			if len(h.delivered.rows) != tc.want || len(queued.requests) != tc.want {
				t.Errorf("%d deliveries, %d jobs; want %d", len(h.delivered.rows), len(queued.requests), tc.want)
			}
		})
	}
}

// A poll leaves out what the caller does not reach and still moves past it.
func TestAPollLeavesOutAPrivateHubTheCallerDoesNotReach(t *testing.T) {
	hidden := emitted(now.Add(-2*time.Hour), "01936f2a-0000-7000-8000-000000000001", event.ItemCreated)
	hidden.Payload = map[string]any{"collection_id": privateCollection.String()}
	open := emitted(now.Add(-2*time.Hour), "01936f2a-0000-7000-8000-000000000002", event.ItemCreated)
	h := newPollHarness(hidden, open)
	actor := pollingActor(bothScopes()...)
	h.handler.Reach = hiddenTo{actor.AccountID: true}

	page, err := h.handler.Execute(context.Background(), actor,
		PollTriggerEventsCommand{EventType: string(event.ItemCreated)})
	if err != nil {
		t.Fatalf("polling: %v", err)
	}
	if len(page.Events) != 1 || page.Events[0]["id"] != open.ID.String() {
		t.Fatalf("answered %v, want only the event outside the private hub", page.Events)
	}
}
