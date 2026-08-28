// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	automation "github.com/Jersyfi/hubtask/core/domain/model/automation"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RecordRuleDisabled tells the writer of an automation rule that it has switched itself off after a
// run of failures (G-07, automation.md §2).
//
// The same path RecordWebhookDisabled takes, and for the same reasons: the preference is honoured,
// the record is deduplicated, and the send is a job like every other. A rule that stopped acting and
// told nobody is a rule whose owner discovers the silence weeks later, by which time the work it was
// supposed to do has not been done and nobody knows since when.
//
// The recipient is the rule's **author**, not the account it runs as. A service account has nobody
// behind it to read a message; the person who wrote the rule is the one who can fix it.
//
// It carries no event, exactly as the webhook's does. The trigger is the fifth consecutive failed
// run, and there is no envelope that says so - the column has allowed a null since C-09. The
// deduplication index therefore does not apply, which is right: a rule disables itself once, so the
// caller is already the thing that happens once.
type RecordRuleDisabled struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
	Signals       Signals
}

// RuleDisabled records the message and queues its delivery.
//
// It runs inside the engine's transaction, so the record and the rule's new state commit together. A
// notification that committed without the disabling would tell somebody their rule had stopped while
// it carried on.
func (r RecordRuleDisabled) RuleDisabled(
	ctx context.Context, rule automation.Rule, at time.Time,
) error {
	if rule.CreatedBy.IsZero() {
		// A rule written by something with no account behind it. There is nobody to tell, and
		// inventing a recipient would be worse than the silence.
		return nil
	}

	written, err := domain.New(domain.NewInput{
		ID:          r.IDs.NewID(),
		TenantID:    rule.TenantID,
		RecipientID: rule.CreatedBy,
		Category:    domain.CategoryIntegration,
		Channel:     domain.ChannelEmail,
		// The rule stands in for the item: it is what the message is about, and it is what a
		// renderer needs to name. Never its name - a title is user content, and the record is read
		// by a renderer that looks the rule up (rule 10).
		ItemID: rule.ID,
		At:     at,
	})
	if err != nil {
		return err
	}

	decision, err := decideFor(ctx, r.Accounts, r.Preferences, written, rule.CreatedBy, domain.CategoryIntegration)
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
		Kind:     queue.KindNotificationDeliver,
		TenantID: written.TenantID,
		// The record's own identifier, so a retried write cannot queue a second send of one
		// message - the same key every other notification uses.
		DedupeKey: written.ID.String(),
		RunAt:     at,
		Payload:   map[string]any{"notification_id": written.ID.String()},
	})
	return err
}
