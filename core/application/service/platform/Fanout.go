// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package platform

import (
	"context"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// Jobs is where a delivery's attempt is asked for.
type Jobs interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// Fanout queues an installation event for every active subscription, in the event's own
// transaction: a delivery exists exactly when its event does, and leaves as soon as the worker is
// woken by the commit (UC-INS-14 check 2). A paused subscription is sent nothing; the platform reads
// what it missed from the feed.
type Fanout struct {
	Subscriptions adminrepo.PlatformSubscriptions
	Deliveries    adminrepo.PlatformDeliveries
	Jobs          Jobs
	IDs           clock.IDGenerator
	Clock         clock.Clock
}

func (f Fanout) on() bool { return f.Subscriptions != nil && f.Deliveries != nil && f.Jobs != nil }

// Queue writes a delivery per active subscription and asks for its first attempt now.
func (f Fanout) Queue(ctx context.Context, envelope event.Envelope) error {
	if !f.on() {
		return nil
	}
	subscriptions, err := f.Subscriptions.Active(ctx)
	if err != nil {
		return err
	}
	now := f.Clock.Now()
	for _, subscription := range subscriptions {
		delivery, err := integration.NewPlatformDelivery(f.IDs.NewID(), subscription, envelope.ID, string(envelope.Type), now)
		if err != nil {
			return err
		}
		queued, err := f.Deliveries.Insert(ctx, delivery)
		if err != nil {
			return err
		}
		if !queued {
			continue
		}
		if _, err := f.Jobs.Enqueue(ctx, DeliveryJob(delivery.ID, now)); err != nil {
			return err
		}
	}
	return nil
}

// DeliveryJob is the installation job that makes a delivery's next attempt at `at`: no tenant, so
// a workspace's purge, which removes its own jobs, leaves it.
func DeliveryJob(deliveryID shared.ID, at time.Time) queue.Request {
	return queue.Request{
		Kind: queue.KindPlatformDeliver, RunAt: at.UTC(),
		Payload: map[string]any{"delivery_id": deliveryID.String()},
	}
}
