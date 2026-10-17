// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package platform

import (
	"context"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

type activeSubs struct {
	adminrepo.PlatformSubscriptions
	ids []shared.ID
}

func (a activeSubs) Active(context.Context) ([]shared.ID, error) { return a.ids, nil }

type queuedDeliveries struct {
	adminrepo.PlatformDeliveries
	queued map[string]integration.PlatformDelivery
}

func (q *queuedDeliveries) Insert(_ context.Context, d integration.PlatformDelivery) (bool, error) {
	key := d.SubscriptionID.String() + "/" + d.EventID.String()
	if _, held := q.queued[key]; held {
		return false, nil
	}
	q.queued[key] = d
	return true, nil
}

type asked struct{ requests []queue.Request }

func (a *asked) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	a.requests = append(a.requests, request)
	return "job", nil
}

// Each event is queued once per active subscription, and its first attempt asked for at once as an
// installation job; a repeated fan-out of the same event queues nothing more.
func TestAnEventIsQueuedForEveryActiveSubscriptionOnce(t *testing.T) {
	deliveries, jobs := &queuedDeliveries{queued: map[string]integration.PlatformDelivery{}}, &asked{}
	rec, l := recorder(row{record: maria})
	rec.Fanout = Fanout{
		Subscriptions: activeSubs{ids: []shared.ID{"0192f000-0000-7000-8000-0000000000a1", "0192f000-0000-7000-8000-0000000000a2"}},
		Deliveries:    deliveries, Jobs: jobs, IDs: &ids{}, Clock: clock.Fixed(at),
	}
	if err := rec.Record(t.Context(), event.WorkspaceSuspended, event.Actor{}, at, nil); err != nil {
		t.Fatal(err)
	}
	if len(deliveries.queued) != 2 || len(jobs.requests) != 2 {
		t.Fatalf("queued %d deliveries and %d jobs, want 2 and 2", len(deliveries.queued), len(jobs.requests))
	}
	for _, request := range jobs.requests {
		if request.Kind != queue.KindPlatformDeliver || !request.TenantID.IsZero() || !request.RunAt.Equal(at) ||
			request.Payload["delivery_id"] == "" {
			t.Errorf("asked %+v", request)
		}
	}
	if err := rec.Fanout.Queue(t.Context(), l.events[0]); err != nil {
		t.Fatal(err)
	}
	if len(jobs.requests) != 2 {
		t.Errorf("a repeated fan-out asked %d more jobs", len(jobs.requests)-2)
	}
}

func TestAnEventWithoutSubscriptionsIsOnlyKept(t *testing.T) {
	rec, l := recorder(row{record: maria})
	if err := rec.Record(t.Context(), event.WorkspaceResumed, event.Actor{}, time.Now(), nil); err != nil {
		t.Fatal(err)
	}
	if len(l.events) != 1 {
		t.Error("the event was not kept")
	}
}
