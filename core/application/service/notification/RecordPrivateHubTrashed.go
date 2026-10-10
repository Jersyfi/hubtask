// SPDX-License-Identifier: Apache-2.0
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

// RecordPrivateHubTrashed tells the workspace's owners that a private hub lost its last member and
// went to the trash (ADR-0073 §5).
//
// A membership message about nothing: no entry, no hub and no person. The hub's name is for its
// members only, and there are none left; the person may have just been erased by the very act that
// left the hub empty. What the owners can act on is that it is there, and the administration's list
// of private hubs shows it with its size and the day it goes.
//
// The ordinary path otherwise: the preference is honoured, the send is a job. It runs inside the
// caller's transaction, so the message and the deletion it reports commit together.
type RecordPrivateHubTrashed struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
}

// PrivateHubTrashed writes one record per recipient and queues the deliveries.
func (r RecordPrivateHubTrashed) PrivateHubTrashed(
	ctx context.Context, tenantID shared.ID, recipients []shared.ID,
) error {
	for _, recipientID := range recipients {
		if err := r.one(ctx, tenantID, recipientID); err != nil {
			return err
		}
	}
	return nil
}

func (r RecordPrivateHubTrashed) one(ctx context.Context, tenantID, recipientID shared.ID) error {
	written, err := domain.New(domain.NewInput{
		ID:          r.IDs.NewID(),
		TenantID:    tenantID,
		RecipientID: recipientID,
		Category:    domain.CategoryMembership,
		Channel:     domain.ChannelEmail,
		At:          r.Clock.Now(),
	})
	if err != nil {
		return err
	}

	decision, err := decideFor(
		ctx, r.Accounts, r.Preferences, written, recipientID, domain.CategoryMembership)
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
		Kind:      queue.KindNotificationDeliver,
		TenantID:  written.TenantID,
		DedupeKey: written.ID.String(),
		RunAt:     r.Clock.Now(),
		Payload:   map[string]any{"notification_id": written.ID.String()},
	})
	return err
}
