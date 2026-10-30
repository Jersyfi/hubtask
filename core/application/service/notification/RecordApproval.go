// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RecordApproval tells people of an approval (UC-SVC-09 check 4): the people a level begins to
// count that it waits for their yes - ASSIGNMENT's switch, work landing on them -, and while it
// waits a reminder - REMINDER's, a moment the step's hours set. The record names the approval and
// nothing else (rule 10); the mail links to the app, where the reader signs in as ever (P-02).
//
// Written in the transaction that opened or moved the approval, or in the timer pass's, so the
// approval and its records commit together.
type RecordApproval struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
	Signals       Signals
}

// ApprovalAsked tells the people a level counts; the person who caused it is not told of their
// own request.
func (r RecordApproval) ApprovalAsked(ctx context.Context, approval servicedesk.Approval, people []shared.ID, actor shared.ID) error {
	return r.record(ctx, approval, people, actor, domain.CategoryAssignment)
}

// ApprovalReminded reminds the people named.
func (r RecordApproval) ApprovalReminded(ctx context.Context, approval servicedesk.Approval, people []shared.ID) error {
	return r.record(ctx, approval, people, shared.ID(""), domain.CategoryReminder)
}

func (r RecordApproval) record(
	ctx context.Context, approval servicedesk.Approval, people []shared.ID, actor shared.ID, category domain.Category,
) error {
	at := r.Clock.Now()
	for _, person := range people {
		if person.IsZero() || person == actor {
			continue
		}
		written, err := domain.New(domain.NewInput{
			ID: r.IDs.NewID(), TenantID: approval.TenantID, RecipientID: person, Category: category,
			Channel: domain.ChannelEmail, ApprovalID: approval.ID, ActorID: actor, At: at,
		})
		if err != nil {
			return err
		}
		decision, err := decideFor(ctx, r.Accounts, r.Preferences, written, person, category)
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
		if !first {
			continue
		}
		if r.Signals != nil {
			r.Signals.NotificationRecorded(ctx, written.Category.String(), written.Channel.String(), written.State.String())
		}
		if !decision.Send {
			continue
		}
		if _, err := r.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindApprovalEmail, TenantID: written.TenantID, DedupeKey: written.ID.String(),
			RunAt: at, MaxAttempts: queue.PageReviewAttempts,
			Payload: map[string]any{"notification_id": written.ID.String()},
		}); err != nil {
			return err
		}
	}
	return nil
}
