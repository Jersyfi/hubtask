// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/application/service/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// PasswordOpeningEnd is the queue's way into recording the end of an operator's opening of the
// password (ADR-0078 §3). Not detached: it reads and writes one workspace's rows and nothing
// outside the database, so the runner's transaction in that workspace is the right one. While the
// opening it finds is still running - a later one replaced the one it was seeded for - it comes back
// at that opening's end.
type PasswordOpeningEnd struct {
	End admin.EndPasswordOpening
}

var _ queue.Handler = PasswordOpeningEnd{}

// Run ends what is due.
func (h PasswordOpeningEnd) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// An opening is one workspace's; a job without one is a programming error, not a no-op.
		return queue.Result{}, shared.ErrInternal.WithDetail("admin.password_opening_without_tenant")
	}
	again, err := h.End.Execute(ctx, job.TenantID)
	if err != nil || again <= 0 {
		return queue.Result{}, err
	}
	return queue.Result{Repeat: true, RepeatAfter: again}, nil
}

// PasswordOpeningMessage sends one administrator the notice of an opening or of its end.
//
// Detached, PasswordResetMessage's reasoning: it reaches an SMTP server, and a transaction held open
// across somebody else's machine is what observability-reliability.md §8 forbids.
type PasswordOpeningMessage struct {
	Send notification.SendPasswordOpening
}

var (
	_ queue.Handler  = PasswordOpeningMessage{}
	_ queue.Detached = PasswordOpeningMessage{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (PasswordOpeningMessage) OwnsItsTransactions() {}

// Run sends the notice the job names.
func (h PasswordOpeningMessage) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("notifications.job_without_tenant")
	}
	accountID, err := payloadID(job, "account_id")
	if err != nil {
		return queue.Result{}, err
	}
	notice := notification.OpeningNotice{
		AccountID: accountID, Event: payloadString(job, "event"), Ended: payloadString(job, "ended"),
	}
	if raw := payloadString(job, "until"); raw != "" {
		until, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return queue.Result{}, shared.ErrInternal.
				WithDetail("notifications.payload_malformed").
				WithParams(map[string]string{"field": "until"})
		}
		notice.Until = until
	}
	return queue.Result{}, h.Send.Execute(ctx, job.TenantID, notice)
}
