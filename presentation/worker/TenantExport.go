// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"

	"github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// TenantExport is the queue's way into the workspace export (H-07): an inbound adapter that
// translates the job the control plane seeded into one call on the archivist.
//
// Detached, the audit export's reason: the pass streams an archive to a target between reads,
// and a transaction held open across that is what observability-reliability.md §8 forbids. The
// archivist opens its own snapshot transaction for the read and nothing else.
type TenantExport struct {
	Archivist admin.TenantExportArchivist
}

var (
	_ queue.Handler  = TenantExport{}
	_ queue.Detached = TenantExport{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (TenantExport) OwnsItsTransactions() {}

// Run writes the one archive the job names.
func (h TenantExport) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// The snapshot is opened for the tenant the job names; without one there is nothing to
		// export - a programming error, not an empty pass.
		return queue.Result{}, shared.ErrInternal.WithDetail("admin.export_without_tenant")
	}

	request, err := admin.ExportRequestOf(job.Payload, job.TenantID)
	if err != nil {
		return queue.Result{}, err
	}

	manifest, err := h.Archivist.Write(ctx, request)
	if err != nil {
		return queue.Result{}, err
	}

	// Counts and identifiers only (rule 10): the archive's content is at the target, and this
	// line is for the operator watching the worker.
	records := int64(0)
	for _, count := range manifest.Counts {
		records += count
	}
	slog.InfoContext(ctx, "tenant export finished",
		slog.String("archive_id", manifest.ArchiveID),
		slog.Int64("records", records),
		slog.Int64("media_count", manifest.MediaCount),
		slog.Int64("media_bytes", manifest.MediaBytes),
	)
	return queue.Result{}, nil
}
