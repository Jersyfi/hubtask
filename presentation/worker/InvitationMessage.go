// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// InvitationMessage is the queue's way into the invitation B-02 has been queueing since the
// account was created (C-09).
//
// Not detached, unlike the delivery beside it, and that is the whole design: this handler writes a
// record and queues the send, both inside the transaction the runner opened, so a process that
// dies halfway leaves neither. The reaching-outwards is the next job's problem, which is where the
// retries and the dead letter already live.
type InvitationMessage struct {
	Invitation notification.RecordInvitation
}

var _ queue.Handler = InvitationMessage{}

// Run records the invitation the job names.
func (h InvitationMessage) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("notifications.job_without_tenant")
	}

	accountID, err := payloadID(job, "account_id")
	if err != nil {
		return queue.Result{}, err
	}
	// The inviter is optional in a way the account is not: a seat created by the control plane has
	// no person behind it, and a message that named nobody is still an invitation.
	invitedBy, err := optionalPayloadID(job, "invited_by")
	if err != nil {
		return queue.Result{}, err
	}

	if err := h.Invitation.Execute(ctx, job.TenantID, accountID, invitedBy); err != nil {
		return queue.Result{}, err
	}
	return queue.Result{}, nil
}

// optionalPayloadID reads an identifier that may be absent. Absent is zero; present and malformed
// is still a defect.
func optionalPayloadID(job queue.Job, key string) (shared.ID, error) {
	if _, present := job.Payload[key]; !present {
		return "", nil
	}
	return payloadID(job, key)
}
