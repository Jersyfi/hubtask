// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RecordPageReview tells a page's owner that its verification ran out (UC-DOC-12 check 4).
//
// Written in the timer pass's transaction, so the timer's firing and the record commit together.
// REMINDER's switch decides: it is a moment the page's verification set, and a person who silences
// comments still wants it. The record names the page and nothing else (rule 10).
type RecordPageReview struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
	Signals       Signals
}

// PageReviewDue writes the owner's record and queues the delivery. A page without an owner is
// told to whoever verified it; one with neither tells nobody.
func (r RecordPageReview) PageReviewDue(ctx context.Context, page knowledge.Page) error {
	recipient := page.OwnerID
	if recipient.IsZero() {
		recipient = page.VerifiedBy
	}
	if recipient.IsZero() {
		return nil
	}
	written, err := domain.New(domain.NewInput{
		ID:          r.IDs.NewID(),
		TenantID:    page.TenantID,
		RecipientID: recipient,
		Category:    domain.CategoryReminder,
		Channel:     domain.ChannelEmail,
		PageID:      page.ID,
		At:          r.Clock.Now(),
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
		r.Signals.NotificationRecorded(ctx,
			written.Category.String(), written.Channel.String(), written.State.String())
	}
	if !decision.Send {
		return nil
	}
	_, err = r.Jobs.Enqueue(ctx, queue.Request{
		Kind:        queue.KindPageReviewEmail,
		TenantID:    written.TenantID,
		DedupeKey:   written.ID.String(),
		RunAt:       r.Clock.Now(),
		MaxAttempts: queue.PageReviewAttempts,
		Payload:     map[string]any{"notification_id": written.ID.String()},
	})
	return err
}
