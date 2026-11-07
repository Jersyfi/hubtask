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

// RecordAgentNotice tells an agent's owner that it did what its policy lets it do with them told
// (UC-AGT-02, NOTIFY). Written in the gate's transaction, so the ledger's row and the record commit
// together. INTEGRATION's switch decides, as for a rule that stopped: the agent is the person's
// integration. The record names the agent and nothing else (rule 10); what it did is its ledger.
type RecordAgentNotice struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          Queue
	IDs           clock.IDGenerator
	Signals       Signals
}

// AgentActed writes the owner's record and queues the delivery.
func (r RecordAgentNotice) AgentActed(ctx context.Context, tenant, agent, owner shared.ID, at time.Time) error {
	if owner.IsZero() {
		return nil
	}
	written, err := domain.New(domain.NewInput{
		ID: r.IDs.NewID(), TenantID: tenant, RecipientID: owner,
		Category: domain.CategoryIntegration, Channel: domain.ChannelEmail, AgentID: agent, At: at,
	})
	if err != nil {
		return err
	}
	decision, err := decideFor(ctx, r.Accounts, r.Preferences, written, owner, domain.CategoryIntegration)
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
		Kind: queue.KindAgentNoticeEmail, TenantID: written.TenantID, DedupeKey: written.ID.String(),
		RunAt: at, MaxAttempts: queue.PageReviewAttempts,
		Payload: map[string]any{"notification_id": written.ID.String()},
	})
	return err
}
