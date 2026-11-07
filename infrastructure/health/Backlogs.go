// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

const (
	// DeadLettersCode is the report's warning while jobs wait in the dead letter for an operator:
	// the notice an installation without Prometheus has instead of alert A-07.
	DeadLettersCode = "queue.dead_letters"
	// UploadsHeldCode is the report's warning while uploads wait for a scanner that cannot answer:
	// the notice an installation without Prometheus has instead of alert A-23.
	UploadsHeldCode = "media.uploads_held"
	// UploadsHeldUnscannedCode is the same with scanning switched off: nothing will release them.
	UploadsHeldUnscannedCode = "media.uploads_held_without_scanner"
	// BackupMissingCode is the report's warning while an active workspace has no backup target or
	// no successful backup in port.BackupStaleDays: Hubtask creates neither by itself (UC-BAK-01).
	BackupMissingCode = "backup.workspaces_without_backup"

	// backlogsEvery is how old a count may be before a report starts the next one.
	backlogsEvery = time.Minute
	// backlogsTimeout bounds one count (rule 7).
	backlogsTimeout = 10 * time.Second
)

// BacklogReader counts the installation's queues: what is due, dead-lettered, waiting to be
// dispatched and parked for a retry. SampledAt is left to the registry.
type BacklogReader func(ctx context.Context) (port.Backlogs, error)

// ScanningSwitchedOff tells the report that this installation names no upload scanner, so that
// files still held are said as stuck rather than as waiting.
func (r *Registry) ScanningSwitchedOff() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scanningOff = true
}

// heldUploads is the warning for uploads held for a scan, if any are.
func heldUploads(counted port.Backlogs, scanningOff bool) []port.Warning {
	if counted.HeldUploadWorkspaces <= 0 {
		return nil
	}
	code, severity := UploadsHeldCode, "warn"
	if scanningOff {
		code, severity = UploadsHeldUnscannedCode, "critical"
	}
	return []port.Warning{{Code: code, Severity: severity, Params: map[string]string{
		"workspaces":     strconv.FormatInt(counted.HeldUploadWorkspaces, 10),
		"oldest_minutes": strconv.FormatInt(int64(counted.HeldUploadOldestSeconds/60), 10),
	}}}
}

// missingBackups is the warning while any active workspace has no target or no recent backup.
func missingBackups(counted port.Backlogs) []port.Warning {
	if counted.BackupWithoutTarget <= 0 && counted.BackupWithoutRecent <= 0 {
		return nil
	}
	return []port.Warning{{Code: BackupMissingCode, Severity: "warn", Params: map[string]string{
		"without_target": strconv.FormatInt(counted.BackupWithoutTarget, 10),
		"without_recent": strconv.FormatInt(counted.BackupWithoutRecent, 10),
		"days":           strconv.Itoa(port.BackupStaleDays),
	}}}
}

// SetBacklogs has the report serve the queues as last counted. Without it, no figures are served.
func (r *Registry) SetBacklogs(read BacklogReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backlogRead = read
	r.backlogs = port.Backlogs{}
}

// SampleBacklogs counts the queues now and keeps the count; a failed count keeps the last one.
// Called at start, and by a report finding the count stale.
func (r *Registry) SampleBacklogs(ctx context.Context) {
	r.mu.RLock()
	read := r.backlogRead
	r.mu.RUnlock()
	if read == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, backlogsTimeout)
	defer cancel()
	counted, err := read(ctx)
	if err != nil {
		slog.WarnContext(ctx, "the queues could not be counted for the health report",
			slog.String("error", err.Error()))
		return
	}
	counted.SampledAt = r.now()
	r.mu.Lock()
	r.backlogs = counted
	r.mu.Unlock()
}

// lastBacklogs answers the last count at once, and starts the next one in the background when it
// is stale: a report must answer even while the database is slow to count.
func (r *Registry) lastBacklogs(ctx context.Context) port.Backlogs {
	r.mu.Lock()
	defer r.mu.Unlock()
	last := r.backlogs
	if r.backlogRead != nil && !r.backlogsRunning &&
		(last.SampledAt.IsZero() || r.now().Sub(last.SampledAt) >= backlogsEvery) {
		r.backlogsRunning = true
		concurrency.Go(context.WithoutCancel(ctx), "health.backlogs", func(ctx context.Context) {
			r.SampleBacklogs(ctx)
			r.mu.Lock()
			r.backlogsRunning = false
			r.mu.Unlock()
		})
	}
	return last
}
