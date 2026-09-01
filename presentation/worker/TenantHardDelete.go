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

// TenantHardDelete is the grace job's way into §5's final act (H-06): seeded by the deletion
// request's own write, due when the 30-day deadline the row carries has passed, and translated
// here into one call on the application layer.
//
// Detached, for MediaReconciliation's reason: the pass deletes bytes from a bucket between two
// writes, and a transaction held open across that call is exactly what
// observability-reliability.md §8 forbids. What it gives up in return it can afford - the pass
// is safe to run twice, and the argument is written out at admin.HardDeleteTenant.
type TenantHardDelete struct {
	Deletion admin.HardDeleteTenant
}

var (
	_ queue.Handler  = TenantHardDelete{}
	_ queue.Detached = TenantHardDelete{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (TenantHardDelete) OwnsItsTransactions() {}

// Run performs the one deletion the job names.
func (h TenantHardDelete) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// A hard delete without a tenant is a programming error, not an empty pass.
		return queue.Result{}, shared.ErrInternal.WithDetail("admin.purge_without_tenant")
	}

	outcome, err := h.Deletion.Execute(ctx, job.TenantID, job.ID)
	if err != nil {
		return queue.Result{}, err
	}

	// Counts and identifiers only (rule 10) - the durable record is the evidence entry; this
	// line is for the operator watching the worker.
	slog.InfoContext(ctx, "tenant hard delete finished",
		slog.Bool("deleted", outcome.Deleted),
		slog.Int64("items", outcome.Footprint.Items),
		slog.Int64("containers", outcome.Footprint.Containers),
		slog.Int64("media_objects", outcome.Footprint.MediaObjects),
		slog.Int64("media_bytes", outcome.Footprint.MediaBytes),
		slog.Int("byte_objects_removed", outcome.BytesObjects),
		slog.Int64("trail_entries", outcome.TrailEntries),
		slog.Int("jobs_removed", outcome.JobsRemoved),
	)
	return queue.Result{}, nil
}
