// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	service "github.com/Jersyfi/hubtask/core/application/service/audit"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AuditAnchoring is one tenant's daily anchoring (A-2, P-13, audit.md §3): the chain's end
// written to the backup target the workspace named, then the next day's moment.
//
// The backup schedule's shape - one job per tenant, seeded by the configuration's write,
// rescheduling itself, finishing when the workspace names no target - and Detached for the
// export's reason: the write goes to somebody else's machine, and the row that records it is
// written in a short transaction of its own afterwards.
type AuditAnchoring struct {
	Anchoring service.Anchoring
	// Fallback bounds the wait: the next moment is tomorrow's, and a lease longer than a day
	// deserves nothing.
	Fallback time.Duration
}

var (
	_ queue.Handler  = AuditAnchoring{}
	_ queue.Detached = AuditAnchoring{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (h AuditAnchoring) OwnsItsTransactions() {}

// Run does one round for the tenant the job names.
func (h AuditAnchoring) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("audit.anchoring_without_tenant")
	}
	outcome, err := h.Anchoring.Run(ctx, job.TenantID)
	if err != nil {
		return queue.Result{}, err
	}
	if outcome.NextDue.IsZero() {
		// The workspace names no target. The job finishes; the next configuration seeds one.
		return queue.Result{}, nil
	}
	return queue.Result{
		Repeat: true, RepeatAfter: waitUntil(h.Anchoring.Clock.Now(), outcome.NextDue, h.Fallback),
	}, nil
}
