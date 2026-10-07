// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// PasswordResetMessage is the queue's way into sending a reset link (ADR-0068 §6): an inbound
// adapter, like every other handler, translating a job into a call on the application layer.
//
// Detached, NotificationDelivery's reasoning verbatim: it mints a credential, then reaches an SMTP
// server, and a transaction held open across somebody else's machine is what
// observability-reliability.md §8 forbids. What is given up is the same atomicity and the trade is
// the same one - a retry that mints a second token invalidates nothing, because the newest token is
// the one the mail that arrived carries.
type PasswordResetMessage struct {
	Reset notification.SendPasswordReset
}

var (
	_ queue.Handler  = PasswordResetMessage{}
	_ queue.Detached = PasswordResetMessage{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (h PasswordResetMessage) OwnsItsTransactions() {}

// Run sends the link the job names.
func (h PasswordResetMessage) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// An address is per workspace, and so is the account behind it. A job without one is a
		// programming error, not a link to nowhere.
		return queue.Result{}, shared.ErrInternal.WithDetail("notifications.job_without_tenant")
	}

	accountID, err := payloadID(job, "account_id")
	if err != nil {
		return queue.Result{}, err
	}

	if err := h.Reset.Execute(ctx, job.TenantID, accountID); err != nil {
		return queue.Result{}, err
	}
	return queue.Result{}, nil
}
