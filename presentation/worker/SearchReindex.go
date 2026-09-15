// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// SearchReindex rewrites one workspace's stale search documents, batch by batch (M-09).
//
// The embedding pass's shape without the provider: a batch in its own transaction, then a
// decision about whether to come straight back - and unlike the embedding pass it *finishes*,
// because what it does is a backlog with an end. Detached, so that each batch commits on its own
// and a walk over a large workspace holds no row longer than one statement.
//
// Progress is what a client polls for: the rows still stale over the rows that were stale when
// the administrator asked, which the job's payload carries.
type SearchReindex struct {
	Rebuild  work.RebuildSearchIndex
	Progress queue.Reporter
	// Continuation is the wait between two batches. Short: there is known work left, and the
	// only reason not to do it now is that a batch is where one transaction ends.
	Continuation time.Duration
}

var (
	_ queue.Handler  = SearchReindex{}
	_ queue.Detached = SearchReindex{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (h SearchReindex) OwnsItsTransactions() {}

// Run rewrites one batch for the tenant the job names.
func (h SearchReindex) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("search.reindex_without_tenant")
	}
	// The system acting for a workspace, at an administrator's request: the index is the
	// workspace's, and the rows it rewrites are read by nobody in particular.
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}

	outcome, err := h.Rebuild.Execute(ctx, actor)
	if err != nil {
		return queue.Result{}, err
	}

	if h.Progress != nil {
		if err := h.Progress.Report(ctx, job, fractionDone(job, outcome.Remaining)); err != nil {
			slog.DebugContext(ctx, "the progress of a search reindex was not recorded",
				slog.String("job_id", job.ID.String()), slog.String("error", err.Error()))
		}
	}
	if outcome.Remaining > 0 {
		return queue.Result{Repeat: true, RepeatAfter: h.Continuation}, nil
	}
	// Counts only (rule 10): how many, never which.
	slog.InfoContext(ctx, "search reindex finished",
		slog.String("job_id", job.ID.String()),
		slog.String("tenant_id", job.TenantID.String()),
		slog.Int64("rewritten_last_batch", outcome.Rewritten))
	return queue.Result{}, nil
}

// fractionDone reads the stale count the ask recorded and answers how far the walk has got. A
// payload without one - or one the walk has since overtaken, because rows went stale in the
// meantime - answers what it can without going backwards.
func fractionDone(job queue.Job, remaining int64) float64 {
	initial := payloadInt(job.Payload["stale"])
	if initial <= 0 || remaining >= initial {
		if remaining == 0 {
			return 1
		}
		return 0
	}
	return float64(initial-remaining) / float64(initial)
}

func payloadInt(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}
