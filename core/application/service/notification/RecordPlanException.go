// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RecordPlanException tells whoever decided a plan's time tolerance - the level above, as
// PRINCE2's exception goes up - that the plan's forecast end slipped past its latest baseline's by
// more than the tolerance (configuration.md §10). REMINDER's switch decides: it is a moment the person
// asked to be told of when they set the bound. The record names the baseline and nothing else
// (rule 10), and the table keeps one per baseline and recipient, so every later read of the same
// exception writes nothing.
type RecordPlanException struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          Queue
	IDs           clock.IDGenerator
	Signals       Signals
}

// PlanExceeded writes the record and queues the delivery; the reader who found the exception is
// not told of it by mail, since they are looking at it.
func (r RecordPlanException) PlanExceeded(ctx context.Context, tenant, baseline, recipient, reader shared.ID, at time.Time) error {
	if recipient.IsZero() || recipient == reader {
		return nil
	}
	written, err := domain.New(domain.NewInput{
		ID: r.IDs.NewID(), TenantID: tenant, RecipientID: recipient, Category: domain.CategoryReminder,
		Channel: domain.ChannelEmail, BaselineID: baseline, At: at,
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
	if r.Signals != nil {
		r.Signals.NotificationRecorded(ctx, written.Category.String(), written.Channel.String(), written.State.String())
	}
	if !decision.Send {
		return nil
	}
	_, err = r.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindPlanExceptionEmail, TenantID: written.TenantID, DedupeKey: written.ID.String(),
		RunAt: at, MaxAttempts: queue.PageReviewAttempts,
		Payload: map[string]any{"notification_id": written.ID.String()},
	})
	return err
}
