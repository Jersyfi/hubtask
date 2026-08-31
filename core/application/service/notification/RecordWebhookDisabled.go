// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RecordWebhookDisabled tells the owner of a webhook subscription that this system has stopped
// calling their server (G-03, automation.md §3.1).
//
// The notification path C-09 built rather than a new channel: the preference is honoured, the
// record is deduplicated, and the send is a job like every other. What is different is only what
// caused it - nobody's action and no domain event, but a run of failures the system concluded
// something from.
//
// # Why it carries no event
//
// `event_id` is null, which the column has allowed since C-09 "for the invitation, which is not an
// event". A disabled subscription is the second of those: the trigger is the eighth failed attempt
// of the third dead-lettered delivery, and there is no envelope that says so. The deduplication
// index therefore does not apply, which is correct here - the aggregate disables itself once, so
// the caller is already the thing that happens once.
type RecordWebhookDisabled struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          queue.Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
	Signals       Signals
}

// WebhookDisabled records the message and queues its delivery.
//
// It runs inside the caller's transaction - the delivery adapter's short write - so the record and
// the subscription's new state commit together. A notification that committed without the
// disabling would tell somebody their integration had stopped while it carried on.
func (r RecordWebhookDisabled) WebhookDisabled(
	ctx context.Context, tenantID, recipientID, subscriptionID shared.ID,
) error {
	if recipientID.IsZero() {
		// A subscription created by something with no account behind it - a service account is an
		// account, so this is the system itself. There is nobody to tell, and inventing a
		// recipient would be worse than the silence.
		return nil
	}

	written, err := domain.New(domain.NewInput{
		ID:          r.IDs.NewID(),
		TenantID:    tenantID,
		RecipientID: recipientID,
		Category:    domain.CategoryIntegration,
		Channel:     domain.ChannelEmail,
		// The subscription stands in for the item: it is what the message is about, and it is what
		// a renderer needs to name. No event - see the note on the type.
		ItemID: subscriptionID,
		At:     r.Clock.Now(),
	})
	if err != nil {
		return err
	}

	decision, err := decideFor(ctx, r.Accounts, r.Preferences, written, recipientID, domain.CategoryIntegration)
	if err != nil {
		return err
	}
	if !decision.Send {
		written = written.Suppress(decision.Reason)
	}

	first, err := r.Notifications.Insert(ctx, written)
	if err != nil {
		return err
	}
	if !first || !decision.Send {
		return nil
	}

	_, err = r.Jobs.Enqueue(ctx, queue.Request{
		Kind:     queue.KindNotificationDeliver,
		TenantID: written.TenantID,
		// The record's own identifier, so a retried write cannot queue a second send of one
		// message - the same key RecordNotifications uses.
		DedupeKey: written.ID.String(),
		RunAt:     r.Clock.Now(),
		Payload:   map[string]any{"notification_id": written.ID.String()},
	})
	return err
}
