// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"

	service "github.com/Jersyfi/hubtask/core/application/service/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// Import is the queue's way into landing a file somebody exported elsewhere (P-08).
//
// Detached, for the reason a restore is: it reads the file back from the object store and applies
// its records in batches of their own, and doing that inside the runner's transaction would hold
// one open for as long as the file is long. Safe to repeat, which is what makes it a job: the run
// records how far the applier got, a resumed attempt continues there, and the identities are
// derived from the source, so an attempt taking over after a worker died writes what the first
// one did not rather than a second copy of what it did.
type Import struct {
	Runner service.Runner
	// Progress records how far the applier got on the job row, for whoever polls it.
	Progress queue.Reporter
}

var (
	_ queue.Handler  = Import{}
	_ queue.Detached = Import{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for. See the type's comment.
func (h Import) OwnsItsTransactions() {}

// Run performs one import.
func (h Import) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.Internalf("import: a job without a tenant")
	}
	raw, _ := job.Payload["import_id"].(string)
	importID, err := shared.ParseID(raw)
	if err != nil {
		return queue.Result{}, shared.Internalf("import: a job without an import: %w", err)
	}
	report := func(fraction float64) {
		if h.Progress == nil {
			return
		}
		if err := h.Progress.Report(ctx, job, fraction); err != nil {
			slog.DebugContext(ctx, "the progress of an import was not recorded",
				slog.String("job_id", job.ID.String()), slog.Any("error", err))
		}
	}
	if err := h.Runner.Run(ctx, service.RunInput{ImportID: importID, TenantID: job.TenantID, Report: report}); err != nil {
		return queue.Result{}, err
	}
	// The identifier and nothing of the file: what an operator watching the worker needs is that
	// the import ran, and a row's title is content (rule 10).
	slog.InfoContext(ctx, "an import ran", slog.String("import_id", importID.String()))
	return queue.Result{}, nil
}
