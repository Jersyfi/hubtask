// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"

	"context"
)

// RecordReminder turns a reminder that has come due into records and the jobs that send them
// (D-03).
//
// Deliberately not a second delivery path, for the reason the invitation is not one: it writes the
// same record every other notification writes and queues the same `notification.deliver` job, so
// there is one place that renders an email and one place that sends it. What is different is only
// where the recipients come from - a reminder names them, or means the assignee and the entry's
// members - and that is the firing service's question, not this one's.
//
// It runs inside the firing pass's transaction, which is what makes it exactly-once: the records,
// the delivery jobs, the reminder's own PENDING → SENT transition and the job's completion commit
// together. A process that dies halfway leaves none of them, and the reminder is still pending
// when the job is claimed again.
type RecordReminder struct {
	Notifications repository.Notifications
	Preferences   repository.Preferences
	Accounts      identityrepo.Accounts
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
	Signals       Signals
}

// Execute writes one record per recipient and queues what is to be sent.
//
// The preference is consulted, unlike the invitation's: somebody may switch reminders off, and the
// suppressed record is what tells them afterwards why they heard nothing. Nobody is named as the
// actor - a reminder is caused by the clock rather than by a person - which is also what keeps the
// domain's self-caused rule from suppressing a reminder somebody set for themselves.
func (r RecordReminder) Execute(
	ctx context.Context, tenantID, itemID shared.ID, recipients []shared.ID,
) error {
	for _, recipient := range recipients {
		if recipient.IsZero() {
			continue
		}
		if err := r.record(ctx, tenantID, itemID, recipient); err != nil {
			return err
		}
	}
	return nil
}

func (r RecordReminder) record(ctx context.Context, tenantID, itemID, recipient shared.ID) error {
	written, err := domain.New(domain.NewInput{
		ID:          r.IDs.NewID(),
		TenantID:    tenantID,
		RecipientID: recipient,
		Category:    domain.CategoryReminder,
		Channel:     domain.ChannelEmail,
		// No event and no actor. What keeps this record single is not the deduplication index -
		// which does not apply without an event - but the guarded transition that produced it: a
		// reminder leaves PENDING once, in the transaction that writes this row.
		ItemID: itemID,
		At:     r.Clock.Now(),
	})
	if err != nil {
		return err
	}

	decision, err := decideFor(ctx, r.Accounts, r.Preferences, written, recipient, domain.CategoryReminder)
	if err != nil {
		return err
	}
	if !decision.Send {
		written = written.Suppress(decision.Reason)
	}

	first, err := r.Notifications.Insert(ctx, written)
	if err != nil || !first {
		return err
	}
	r.report(ctx, written)

	if written.State != domain.StatePending {
		return nil
	}
	_, enqueued := r.Jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindNotificationDeliver,
		TenantID:  tenantID,
		DedupeKey: written.ID.String(),
		Payload: map[string]any{
			// The identifier and nothing else. Everything the delivery needs is read when it runs,
			// which is what makes the email right about an entry that was renamed while it waited -
			// and what keeps personal data out of a table nothing cleans (rule 10).
			"notification_id": written.ID.String(),
		},
	})
	return enqueued
}

func (r RecordReminder) report(ctx context.Context, written domain.Notification) {
	if r.Signals == nil {
		return
	}
	r.Signals.NotificationRecorded(ctx,
		written.Category.String(), written.Channel.String(), written.State.String())
}
