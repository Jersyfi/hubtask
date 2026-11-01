// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RecordMailboxFailing tells the workspace's administrators that a connected mailbox has refused
// for over an hour (ADR-0094): mail sent to the desk through it is not arriving, and no ticket or
// service clock starts until somebody fixes its sign-in. Told once, when the mailbox turns
// failing - the poll keeps trying every minute and a message each time would be noise.
//
// The integration category: what an integration has to tell its owner, on the switch somebody set
// for exactly that. No event, like a disabled webhook: a run of refusals is what caused it.
type RecordMailboxFailing struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Memberships   identityrepo.Memberships
	Preferences   repository.Preferences
	Jobs          queue.Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
}

// MailboxFailing records one message per administrator and queues its delivery, inside the
// caller's transaction, so the message commits with the check that found the mailbox failing.
func (r RecordMailboxFailing) MailboxFailing(ctx context.Context, tenantID, mailboxID shared.ID) error {
	administrators, err := r.Memberships.Administrators(ctx, []identity.Scope{identity.TenantScope()})
	if err != nil {
		return err
	}
	for _, recipientID := range administrators {
		if err := r.one(ctx, tenantID, mailboxID, recipientID); err != nil {
			return err
		}
	}
	return nil
}

func (r RecordMailboxFailing) one(ctx context.Context, tenantID, mailboxID, recipientID shared.ID) error {
	written, err := domain.New(domain.NewInput{
		ID: r.IDs.NewID(), TenantID: tenantID, RecipientID: recipientID,
		Category: domain.CategoryIntegration, Channel: domain.ChannelEmail,
		MailboxID: mailboxID, At: r.Clock.Now(),
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
	if err != nil || !first || !decision.Send {
		return err
	}
	_, err = r.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindNotificationDeliver, TenantID: written.TenantID,
		DedupeKey: written.ID.String(), RunAt: r.Clock.Now(),
		Payload: map[string]any{"notification_id": written.ID.String()},
	})
	return err
}
