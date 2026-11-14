// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RecordPost tells a post's audience it was published (UC-DOC-17 check 5) and reminds those who
// have not confirmed it (UC-DOC-18 check 2). POST's switch decides. The record names the post's
// page and, for a reminder, the writer who reminds - nothing else (rule 10). Who is told was judged
// by the application before (rule 2).
type RecordPost struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
	Signals       Signals
}

// PostPublished writes one record per person of the audience.
func (r RecordPost) PostPublished(ctx context.Context, tenant shared.ID, post knowledge.Post, recipients []shared.ID) error {
	return r.record(ctx, tenant, post, "", recipients)
}

// PostReminder writes one record per person who has not confirmed, naming the writer who reminds.
func (r RecordPost) PostReminder(ctx context.Context, tenant, writer shared.ID, post knowledge.Post, recipients []shared.ID) error {
	return r.record(ctx, tenant, post, writer, recipients)
}

func (r RecordPost) record(ctx context.Context, tenant shared.ID, post knowledge.Post, actor shared.ID, people []shared.ID) error {
	at := r.Clock.Now()
	for _, person := range people {
		if person.IsZero() || person == actor {
			continue
		}
		written, err := domain.New(domain.NewInput{
			ID: r.IDs.NewID(), TenantID: tenant, RecipientID: person, Category: domain.CategoryPost,
			Channel: domain.ChannelEmail, PageID: post.PageID, ActorID: actor, At: at,
		})
		if err != nil {
			return err
		}
		decision, err := decideFor(ctx, r.Accounts, r.Preferences, written, person, domain.CategoryPost)
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
			Kind: queue.KindPostEmail, TenantID: written.TenantID, DedupeKey: written.ID.String(),
			RunAt: at, MaxAttempts: queue.PageReviewAttempts,
			Payload: map[string]any{"notification_id": written.ID.String()},
		}); err != nil {
			return err
		}
	}
	return nil
}
