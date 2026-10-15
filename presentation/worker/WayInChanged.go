// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// WayInChangedMessage tells a person that a way into their account changed (ADR-0078 §6).
//
// Detached, PasswordResetMessage's reasoning: it reaches an SMTP server, and a transaction held open
// across somebody else's machine is what observability-reliability.md §8 forbids.
type WayInChangedMessage struct {
	Send notification.SendWayInChanged
}

var (
	_ queue.Handler  = WayInChangedMessage{}
	_ queue.Detached = WayInChangedMessage{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (WayInChangedMessage) OwnsItsTransactions() {}

// Run sends the notice the job names.
func (h WayInChangedMessage) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("notifications.job_without_tenant")
	}
	accountID, err := payloadID(job, "account_id")
	if err != nil {
		return queue.Result{}, err
	}
	at, err := time.Parse(time.RFC3339Nano, payloadString(job, "at"))
	if err != nil {
		return queue.Result{}, shared.ErrInternal.
			WithDetail("notifications.payload_malformed").
			WithParams(map[string]string{"field": "at"})
	}
	change := identity.WayInChange{
		Event: identity.WayInEvent(payloadString(job, "event")), At: at,
		ProviderName: payloadString(job, "provider_name"),
	}
	if payloadString(job, "provider_id") != "" {
		if change.ProviderID, err = payloadID(job, "provider_id"); err != nil {
			return queue.Result{}, err
		}
	}
	return queue.Result{}, h.Send.Execute(ctx, job.TenantID, notification.WayInNotice{
		AccountID: accountID, Change: change,
	})
}
