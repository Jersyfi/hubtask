// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"errors"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RecordInvitation turns the invitation job B-02 has been queueing into a message.
//
// B-02 wrote the queue call and left the other end unbuilt on purpose: an invitation has to be
// queued in the transaction that creates the account, so that the seat and the message exist
// together or neither does, and the delivery had nowhere to go until there was a mail port. This
// is that other end - and it is deliberately not a second delivery path. The job writes a record
// and queues the same `notification.deliver` job every other notification uses, so there is one
// place that renders an email and one place that sends it.
//
// It runs inside the runner's transaction, which is what makes it exactly-once: the record, the
// delivery job and the invitation job's own completion commit together (core/port/queue). That is
// why this handler is not detached and the delivery is.
type RecordInvitation struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
	Signals       Signals
}

// Execute writes the invitation's record and queues its delivery.
//
// The preference is not consulted, and there is no branch here that could consult it: the
// invitation is the one category no preference may switch off (domain.Category.Suppressible), and
// the setting that would switch it off sits behind the door this message unlocks.
func (r RecordInvitation) Execute(
	ctx context.Context, tenantID, accountID, invitedBy shared.ID,
) error {
	invited, err := r.Accounts.Find(ctx, accountID)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		// The account was removed between the invitation and this job. Nothing to send, and not a
		// failure: an invitation to somebody who is no longer invited is finished business.
		return nil
	case err != nil:
		return err
	}

	written, err := domain.New(domain.NewInput{
		ID:          r.IDs.NewID(),
		TenantID:    tenantID,
		RecipientID: accountID,
		Category:    domain.CategoryInvitation,
		Channel:     domain.ChannelEmail,
		// No event: this one is queued by the use case that created the account rather than
		// announced by the outbox, which is also why the deduplication index does not apply to it.
		// What keeps it single is the job's own dedupe key - one pending invitation per account.
		ActorID: invitedBy,
		At:      r.Clock.Now(),
	})
	if err != nil {
		return err
	}

	if invited.Email == "" {
		// An invited account with no address cannot exist through InviteAccount, which normalises
		// and checks one. Recorded rather than assumed away, because a record that says why is the
		// whole point of this table.
		written = written.Suppress(domain.ReasonNoAddress)
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
		Payload:   map[string]any{"notification_id": written.ID.String()},
	})
	return enqueued
}

func (r RecordInvitation) report(ctx context.Context, written domain.Notification) {
	if r.Signals == nil {
		return
	}
	r.Signals.NotificationRecorded(ctx,
		written.Category.String(), written.Channel.String(), written.State.String())
}
