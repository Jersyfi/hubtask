// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// PasswordResetRequest is the queue's way into a forgotten-password request: the job every request
// leaves behind, whether or not its address held an account (T-02).
//
// Not detached, InvitationMessage's reasoning: it records the request and queues the mail inside the
// transaction the runner opened, so a process that dies halfway leaves neither. Reaching the mail
// server is the next job's.
type PasswordResetRequest struct {
	Record identity.RecordResetRequest
}

var _ queue.Handler = PasswordResetRequest{}

// Run records the request the job names.
func (h PasswordResetRequest) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("notifications.job_without_tenant")
	}
	if err := h.Record.Execute(ctx, job.TenantID, job.Payload); err != nil {
		return queue.Result{}, err
	}
	return queue.Result{}, nil
}
