// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AddressChangeMessage is the queue's way into the mail that tells a member the workspace's address
// changed (multi-tenancy.md §3.1). Detached for PasswordOpeningMessage's reason: it sends.
type AddressChangeMessage struct {
	Send notification.SendAddressChange
}

var (
	_ queue.Handler  = AddressChangeMessage{}
	_ queue.Detached = AddressChangeMessage{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (AddressChangeMessage) OwnsItsTransactions() {}

// Run sends the mail the job names.
func (h AddressChangeMessage) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("notifications.job_without_tenant")
	}
	accountID, err := payloadID(job, "account_id")
	if err != nil {
		return queue.Result{}, err
	}
	return queue.Result{}, h.Send.Execute(ctx, job.TenantID, notification.AddressNotice{
		AccountID: accountID, From: payloadString(job, "from"), To: payloadString(job, "to"),
	})
}
