// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"errors"
	"log/slog"

	service "github.com/Jersyfi/hubtask/core/application/service/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// BackupRestore is the queue's way into reading an archive back (E-06, backup-restore.md §8.3).
//
// Detached, and it has to be. The restore streams an archive from somebody else's machine and
// writes it in batches, each of which is its own transaction - which is what §8.3 step 5 means by
// "rollback within a transaction per batch size". Doing that inside the runner's own transaction
// would mean the runner's staying open for the length of the restore, on the connection pool the
// API shares.
//
// What it gives up is the atomicity of its own completion, which it can afford because the run is
// safe to repeat: the run row is claimed by identity, and the progress of each batch is recorded in
// the transaction that wrote it, so the attempt that takes over continues where the last one
// stopped rather than starting a second restore (BK-7).
type BackupRestore struct {
	Applier service.Applier
	// Progress is how the restore says how far along it is. Optional: losing a reading is worth a
	// log line rather than a restore.
	Progress queue.Reporter
}

var (
	_ queue.Handler  = BackupRestore{}
	_ queue.Detached = BackupRestore{}
	_ queue.Releaser = BackupRestore{}
)

// Release closes the restore row of a job the queue has given up on (queue.Releaser, #207).
//
// The open row holds the one-restore-per-tenant lock, and with the job in the dead letter nothing
// else will ever close it: every later restore in the tenant would be refused for ever. Closed as
// FAILED under its own code; a row already terminal is left as it is.
func (h BackupRestore) Release(ctx context.Context, job queue.Job) {
	restoreID, err := identifierIn(job, "restore_id")
	if err != nil || job.TenantID.IsZero() {
		return
	}
	if err := h.Applier.Abandon(ctx, restoreID, job.TenantID); err != nil {
		slog.WarnContext(ctx, "an abandoned restore could not be closed; the workspace stays locked for restores",
			slog.String("restore_id", restoreID.String()),
			slog.String("error", shared.AsError(err).Code))
	}
}

// OwnsItsTransactions is the assertion queue.Detached asks for. See the type's comment.
func (h BackupRestore) OwnsItsTransactions() {}

// Run reads one archive back.
func (h BackupRestore) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	restoreID, err := identifierIn(job, "restore_id")
	if err != nil {
		return queue.Result{}, err
	}
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.Internalf("backup: a restore job without a tenant")
	}

	in := service.ApplyInput{
		RestoreID: restoreID, TenantID: job.TenantID,
		Report: h.reporter(ctx, job),
	}

	report, err := h.Applier.Apply(ctx, in)
	if err != nil {
		// Another restore holds the tenant, or this one is already over. Not a failure to retry
		// into: the work the caller asked for is either happening or finished.
		if restoreBusy(err) {
			slog.InfoContext(ctx, "a restore was already running in that workspace",
				slog.String("restore_id", restoreID.String()))
			return queue.Result{}, nil
		}
		return queue.Result{}, err
	}

	// Counts, never content (rule 10). What an operator needs from the log is the size of what
	// happened; what it did to which item is in the report on the run.
	slog.InfoContext(ctx, "a restore finished",
		slog.String("restore_id", restoreID.String()),
		slog.Int("new", report.New),
		slog.Int("overwritten", report.Overwritten),
		slog.Int("skipped", report.Skipped),
		slog.Int("withheld", report.Deleted()))
	return queue.Result{}, nil
}

func (h BackupRestore) reporter(ctx context.Context, job queue.Job) func(float64) {
	if h.Progress == nil {
		return nil
	}
	return func(fraction float64) {
		if err := h.Progress.Report(ctx, job, fraction); err != nil {
			slog.DebugContext(ctx, "the progress of a restore was not recorded",
				slog.String("job_id", job.ID.String()), slog.String("error", err.Error()))
		}
	}
}

func restoreBusy(err error) bool {
	var domainErr *shared.Error
	return errors.As(err, &domainErr) && domainErr.DetailCode == domain.CodeRestoreTargetBusy
}
