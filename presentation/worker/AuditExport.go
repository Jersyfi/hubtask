// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"

	service "github.com/Jersyfi/hubtask/core/application/service/audit"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AuditExport is the queue's way into writing a period of the trail to a backup target (E-09,
// audit.md §5).
//
// Detached, for the reason a backup run is: it streams as much evidence as the tenant produced to
// somebody else's machine, and doing that inside the runner's own transaction would hold one open
// for minutes on the pool the API shares.
//
// Safe to repeat, which is what makes it a job at all. An export is a pure read followed by a write
// under a name derived from the tenant and the period, so an attempt that takes over after a worker
// died writes the same archive over the same keys rather than a second one somewhere else.
type AuditExport struct {
	Archivist service.Archivist
}

var (
	_ queue.Handler  = AuditExport{}
	_ queue.Detached = AuditExport{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for. See the type's comment.
func (h AuditExport) OwnsItsTransactions() {}

// Run writes one export.
func (h AuditExport) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.Internalf("audit: an export job without a tenant")
	}

	request, err := service.ExportRequestOf(job.Payload, job.TenantID)
	if err != nil {
		return queue.Result{}, err
	}

	manifest, err := h.Archivist.Write(ctx, request)
	if err != nil {
		return queue.Result{}, err
	}

	// The counts rather than the entries: what an operator watching the worker needs is that the
	// archive was written and how much of the trail it holds. The entries themselves are evidence
	// and never appear in a log line (rule 10).
	slog.InfoContext(ctx, "an audit export was written",
		slog.String("export_id", request.ExportID.String()),
		slog.Int("entries", manifest.Entries),
		slog.Bool("signed", manifest.Signed))
	return queue.Result{}, nil
}
