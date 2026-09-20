// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// CheckOnDeletionConsumer names the subscriber in the deduplication and the logs.
const CheckOnDeletionConsumer = "automation-check-on-deletion"

// CheckOnDeletion seeds the workspace's check when something a rule may name is deleted
// (ADR-0060).
//
// A subscriber runs inside the dispatcher's transaction and may not reach the use case registry
// (automation.md §2.0), and the check reads every rule of the workspace and resolves every
// reference - not work for the transaction that recorded a deletion. So this writes one job and
// nothing else, deduplicated per tenant: a bulk deletion of forty labels is one check, and a
// workspace that deletes nothing pays nothing.
type CheckOnDeletion struct {
	Jobs Queue
}

var _ eventbus.Subscriber = CheckOnDeletion{}

// Name identifies the subscriber.
func (c CheckOnDeletion) Name() string { return CheckOnDeletionConsumer }

// Wants reports whether an event can have taken away something a rule names: the deletion events
// of the kinds the reference table resolves, as far as those events exist. A template's and a
// subscription's join here when their deletions become events.
func (c CheckOnDeletion) Wants(eventType event.Type) bool {
	switch eventType {
	case event.LabelDeleted, event.BucketDeleted, event.ContainerDeleted:
		return true
	default:
		return false
	}
}

// Deliver seeds the check for the event's tenant.
func (c CheckOnDeletion) Deliver(ctx context.Context, envelope event.Envelope) error {
	if c.Jobs == nil || envelope.TenantID.IsZero() {
		return nil
	}
	_, err := c.Jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindAutomationCheck,
		TenantID:  envelope.TenantID,
		DedupeKey: envelope.TenantID.String(),
	})
	return err
}
