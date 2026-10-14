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

// RecordRoleGranted tells a person that they were given a role (UC-NOT-01 check 9).
//
// A grant is not an event - membership changes are in the audit trail, not the outbox
// (domain-model.md §4) - so the record is written in the grant's own transaction, as the emergency
// access writes its notices: the role and the record commit together or neither does, and once
// the grant is there nothing can lose the message. The record names the grant and nothing else;
// the mail reads the role, the scope and the place's name from it when it is sent, and a grant
// revoked before then takes its record along.
//
// MEMBERSHIP's switch and its title preference decide, as for being added to an entry: both are
// "you are now part of this" to the person reading it.
type RecordRoleGranted struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
	Signals       Signals
}

// RoleGranted writes the person's record and queues the delivery.
//
// A grant to a group tells nobody. Check 9 is about the person given a role; a group's members did
// not change, the group's reach did, and a grant to a large group would mail every member at once
// about a change none of them asked about - joining a group mails nobody either.
func (r RecordRoleGranted) RoleGranted(ctx context.Context, grant identity.Grant, actorID shared.ID) error {
	if grant.AccountID.IsZero() {
		return nil
	}
	written, err := domain.New(domain.NewInput{
		ID:           r.IDs.NewID(),
		TenantID:     grant.TenantID,
		RecipientID:  grant.AccountID,
		Category:     domain.CategoryMembership,
		Channel:      domain.ChannelEmail,
		MembershipID: grant.ID,
		ActorID:      actorID,
		At:           r.Clock.Now(),
	})
	if err != nil {
		return err
	}

	decision, err := decideFor(ctx, r.Accounts, r.Preferences, written, grant.AccountID, domain.CategoryMembership)
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
	if r.Signals != nil {
		r.Signals.NotificationRecorded(ctx,
			written.Category.String(), written.Channel.String(), written.State.String())
	}
	if !decision.Send {
		return nil
	}
	_, err = r.Jobs.Enqueue(ctx, queue.Request{
		Kind:        queue.KindRoleGrantedEmail,
		TenantID:    written.TenantID,
		DedupeKey:   written.ID.String(),
		RunAt:       r.Clock.Now(),
		MaxAttempts: queue.RoleGrantedAttempts,
		Payload:     map[string]any{"notification_id": written.ID.String()},
	})
	return err
}
