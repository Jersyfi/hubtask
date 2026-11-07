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

// ConnectionNotices records what a connection that stopped, or is about to, has to tell whoever can
// fix it before people notice: the workspace's administrators of a mailbox that has refused for over
// an hour (ADR-0094), a SAML provider whose certificates end soon (identity.md §26.1) and a CMDB
// connector or a forge hook that switched itself off; a calendar push's owner of the push its
// provider refused.
//
// The integration category: what an integration has to tell its owner, on the switch somebody set
// for exactly that. No event, like a disabled webhook: a state that was found is what caused it.
type ConnectionNotices struct {
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
func (r ConnectionNotices) MailboxFailing(ctx context.Context, tenantID, mailboxID shared.ID) error {
	return r.tell(ctx, domain.NewInput{TenantID: tenantID, MailboxID: mailboxID})
}

// SAMLCertificateEnding tells that a SAML provider's last certificate ends soon: from then on
// nobody signs in through it. The watch calls it once per warning.
func (r ConnectionNotices) SAMLCertificateEnding(ctx context.Context, tenantID, providerID shared.ID) error {
	return r.tell(ctx, domain.NewInput{TenantID: tenantID, IdentityProviderID: providerID})
}

// ConnectorSwitchedOff tells the workspace's and the hub's administrators that a CMDB connector
// stopped after its failures in a row: nothing is pulled until somebody switches it on again.
func (r ConnectionNotices) ConnectorSwitchedOff(ctx context.Context, tenantID, hubID, connectorID shared.ID) error {
	return r.tellAt(ctx, domain.NewInput{TenantID: tenantID, ConnectorID: connectorID},
		[]identity.Scope{identity.TenantScope(), identity.HubScope(hubID)})
}

// HookSwitchedOff tells the workspace's and the hub's administrators that a forge hook stopped
// because the person it acts as may no longer write in the hub: no commit is linked until somebody
// who may switches it on again, and so defines it as themselves.
func (r ConnectionNotices) HookSwitchedOff(ctx context.Context, tenantID, hubID, hookID shared.ID) error {
	return r.tellAt(ctx, domain.NewInput{TenantID: tenantID, CodeHookID: hookID},
		[]identity.Scope{identity.TenantScope(), identity.HubScope(hubID)})
}

// CalendarPushStopped tells a calendar push's owner that its provider refused it for good: the
// calendar is not updated until they connect it again.
func (r ConnectionNotices) CalendarPushStopped(ctx context.Context, tenantID, ownerID, pushID shared.ID) error {
	return r.one(ctx, domain.NewInput{TenantID: tenantID, CalendarPushID: pushID}, ownerID)
}

// tell records one message per administrator and queues its delivery, inside the caller's
// transaction, so the message commits with the check that found the state.
func (r ConnectionNotices) tell(ctx context.Context, subject domain.NewInput) error {
	return r.tellAt(ctx, subject, []identity.Scope{identity.TenantScope()})
}

func (r ConnectionNotices) tellAt(ctx context.Context, subject domain.NewInput, path []identity.Scope) error {
	administrators, err := r.Memberships.Administrators(ctx, path)
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

func (r ConnectionNotices) one(ctx context.Context, subject domain.NewInput, recipientID shared.ID) error {
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
