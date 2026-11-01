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

// TellAdministrators records what the workspace's administrators have to fix before people notice:
// a connected mailbox that has refused for over an hour (ADR-0094), and a SAML provider whose
// certificates end soon (identity.md §26.1).
//
// The integration category: what an integration has to tell its owner, on the switch somebody set
// for exactly that. No event, like a disabled webhook: a state that was found is what caused it.
type TellAdministrators struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Memberships   identityrepo.Memberships
	Preferences   repository.Preferences
	Jobs          queue.Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
}

// MailboxFailing tells that a mailbox refused for over an hour: mail sent to the desk through it
// is not arriving. Told once, when the mailbox turns failing - the poll keeps trying every minute
// and a message each time would be noise.
func (r TellAdministrators) MailboxFailing(ctx context.Context, tenantID, mailboxID shared.ID) error {
	return r.tell(ctx, domain.NewInput{TenantID: tenantID, MailboxID: mailboxID})
}

// SAMLCertificateEnding tells that a SAML provider's last certificate ends soon: from then on
// nobody signs in through it. The watch calls it once per warning.
func (r TellAdministrators) SAMLCertificateEnding(ctx context.Context, tenantID, providerID shared.ID) error {
	return r.tell(ctx, domain.NewInput{TenantID: tenantID, IdentityProviderID: providerID})
}

// tell records one message per administrator and queues its delivery, inside the caller's
// transaction, so the message commits with the check that found the state.
func (r TellAdministrators) tell(ctx context.Context, subject domain.NewInput) error {
	administrators, err := r.Memberships.Administrators(ctx, []identity.Scope{identity.TenantScope()})
	if err != nil {
		return err
	}
	for _, recipientID := range administrators {
		if err := r.one(ctx, subject, recipientID); err != nil {
			return err
		}
	}
	return nil
}

func (r TellAdministrators) one(ctx context.Context, subject domain.NewInput, recipientID shared.ID) error {
	subject.ID, subject.RecipientID, subject.At = r.IDs.NewID(), recipientID, r.Clock.Now()
	subject.Category, subject.Channel = domain.CategoryIntegration, domain.ChannelEmail
	written, err := domain.New(subject)
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
