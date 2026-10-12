// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AddressConfirmationMessage is the queue's way into sending the link that confirms a managed
// account's new address (ADR-0074 §6). Detached, PasswordResetMessage's reasoning: it mints a
// credential, then reaches an SMTP server, and no transaction is held across somebody else's
// machine. A retry mints again, and the newest mail's link is the one that works.
type AddressConfirmationMessage struct {
	Send notification.SendAddressConfirmation
}

var (
	_ queue.Handler  = AddressConfirmationMessage{}
	_ queue.Detached = AddressConfirmationMessage{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (h AddressConfirmationMessage) OwnsItsTransactions() {}

// Run sends the link of the request the job names.
func (h AddressConfirmationMessage) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("notifications.job_without_tenant")
	}
	requestID, err := payloadID(job, "request_id")
	if err != nil {
		return queue.Result{}, err
	}
	if err := h.Send.Execute(ctx, job.TenantID, requestID); err != nil {
		return queue.Result{}, err
	}
	return queue.Result{}, nil
}
