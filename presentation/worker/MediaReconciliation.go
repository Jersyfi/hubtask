// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// MediaReconciliation is the queue's way into the reclamation of uploaded files: an inbound
// adapter, like every other handler, that translates a job into a call on the application layer and
// its answer into the queue's vocabulary (C-06).
//
// It reschedules itself rather than being rescheduled, for the reason the retention sweep does:
// nothing in this system may enumerate tenants, so a scheduler cannot create one job per tenant
// even if it wanted to. The first job is created by a staging - an upload is the first thing that
// can ever need reclaiming - and a deletion pulls the next round forward.
//
// The one handler that runs outside the runner's transaction (queue.Detached). It has to: the pass
// deletes bytes from a bucket between two writes, and a transaction held open across that call is
// exactly what observability-reliability.md §8 forbids. What it gives up in return is the atomicity
// of its own completion, which it can afford because the pass is safe to run twice - the argument
// is written out at media.ReconcileMedia.
type MediaReconciliation struct {
	Reconciliation media.ReconcileMedia
	// Interval is the wait after a pass that found nothing left to reclaim. It is what a quiet
	// tenant pays for having the machinery at all.
	Interval time.Duration
	// Continuation is the wait after a pass that filled its batch, and is short: there is known
	// work left, and the only reason not to do it now is that a batch is where one pass ends.
	Continuation time.Duration
}

var (
	_ queue.Handler  = MediaReconciliation{}
	_ queue.Detached = MediaReconciliation{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for. See the type's comment for what is
// being asserted and why this pass may.
func (r MediaReconciliation) OwnsItsTransactions() {}

// Run does one pass for the tenant the job names.
func (r MediaReconciliation) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// Every transaction the pass opens is opened for the tenant the job names. Without one
		// there is nothing to reconcile - a media job without a tenant is a programming error, not
		// an empty pass.
		return queue.Result{}, shared.ErrInternal.WithDetail("media.reconcile_without_tenant")
	}

	// The system acting for a tenant: a tenant, and no account. That is what tells anyone reading
	// the deletion journal afterwards that a file went because nothing pointed at it, rather than
	// because somebody removed it.
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}

	outcome, err := r.Reconciliation.Execute(ctx, actor)
	if err != nil {
		return queue.Result{}, err
	}

	// Back at once while there is known work left, and at the long interval once the batch came
	// back short. The job is never finished for good: the next abandoned staging is always coming,
	// and a row that removed itself would leave the tenant with no reconciliation until its next
	// upload.
	after := r.Interval
	if !r.Reconciliation.Exhausted(outcome) {
		after = r.Continuation
	}
	return queue.Result{Repeat: true, RepeatAfter: after}, nil
}
