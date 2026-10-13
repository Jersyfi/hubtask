// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RecordEmergencyAccess tells every member of a private hub that the workspace's owner opened it
// (ADR-0073 §4).
//
// It runs inside the transaction that opens the hub, so the access and every member's record
// commit together or not at all: nobody reads the hub before its members are told. A record is
// written for every member whatever happens to the mail - suppressed for somebody without an
// address - because the record is also what the web app shows them (identity.md §22).
//
// The category is never switched off; the preference still decides whether the mail names the
// hub. Delivered under its own job kind (queue.KindEmergencyAccessEmail).
type RecordEmergencyAccess struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
}

// EmergencyAccessOpened writes one record per member and queues the deliveries.
func (r RecordEmergencyAccess) EmergencyAccessOpened(
	ctx context.Context, access work.EmergencyAccess, members []shared.ID,
) error {
	for _, member := range members {
		if member == access.OpenedBy {
			continue
		}
		if err := r.one(ctx, access, member); err != nil {
			return err
		}
	}
	return nil
}

func (r RecordEmergencyAccess) one(ctx context.Context, access work.EmergencyAccess, recipientID shared.ID) error {
	written, err := domain.New(domain.NewInput{
		ID:                 r.IDs.NewID(),
		TenantID:           access.TenantID,
		RecipientID:        recipientID,
		Category:           domain.CategoryEmergencyAccess,
		Channel:            domain.ChannelEmail,
		PrivateHubAccessID: access.ID,
		ActorID:            access.OpenedBy,
		At:                 r.Clock.Now(),
	})
	if err != nil {
		return err
	}

	decision, err := decideFor(ctx, r.Accounts, r.Preferences, written, recipientID, domain.CategoryEmergencyAccess)
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
		Kind:        queue.KindEmergencyAccessEmail,
		TenantID:    written.TenantID,
		DedupeKey:   written.ID.String(),
		RunAt:       r.Clock.Now(),
		MaxAttempts: queue.EmergencyAccessAttempts,
		Payload:     map[string]any{"notification_id": written.ID.String()},
	})
	return err
}
