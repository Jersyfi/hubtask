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

// RecordWhiteboardMention tells the people a whiteboard's comment named that it did (UC-CNV-09
// check 2). Written in the comment's transaction, so the comment and its records commit together.
// COMMENT's switch decides. The record names the board and who wrote, nothing else (rule 10); who
// may read the board was judged by the caller before.
type RecordWhiteboardMention struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          Queue
	IDs           clock.IDGenerator
	Signals       Signals
}

// Mentioned writes one record per person and queues each delivery; the writer is never told of
// their own words.
func (r RecordWhiteboardMention) Mentioned(ctx context.Context, tenant, board, actor shared.ID, people []shared.ID, at time.Time) error {
	for _, person := range people {
		if person.IsZero() || person == actor {
			continue
		}
		written, err := domain.New(domain.NewInput{
			ID: r.IDs.NewID(), TenantID: tenant, RecipientID: person, Category: domain.CategoryComment,
			Channel: domain.ChannelEmail, WhiteboardID: board, ActorID: actor, At: at,
		})
		if err != nil {
			return err
		}
		decision, err := decideFor(ctx, r.Accounts, r.Preferences, written, person, domain.CategoryComment)
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
			Kind: queue.KindWhiteboardMentionEmail, TenantID: written.TenantID, DedupeKey: written.ID.String(),
			RunAt: at, MaxAttempts: queue.PageReviewAttempts,
			Payload: map[string]any{"notification_id": written.ID.String()},
		}); err != nil {
			return err
		}
	}
	return nil
}
