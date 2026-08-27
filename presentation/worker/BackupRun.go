// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	service "github.com/Jersyfi/hubtask/core/application/service/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// BackupRun is the queue's way into writing an archive (E-05, backup-restore.md §5).
//
// Detached, and it has to be. The run holds a REPEATABLE READ snapshot while it streams to somebody
// else's machine, and doing that inside the runner's own transaction would mean two open at once -
// the runner's for minutes, on the connection pool the API shares. What it gives up is the
// atomicity of its own completion, which it can afford because the run is safe to repeat: the run
// row is claimed by identity, so the attempt that takes over after a worker died continues its own
// run rather than starting a second one (BK-7).
type BackupRun struct {
	Performer service.Performer
	// Progress is how the run says how far along it is. Optional: losing a reading is worth a log
	// line rather than a backup.
	Progress queue.Reporter
	// Expiry applies the generation plan after a run that worked. It is here rather than inside
	// the run because §6 makes the ordering a rule: a failed run deletes nothing, so the plan is
	// something that happens *after* a success rather than part of one.
	Expiry BackupExpiry
}

var (
	_ queue.Handler  = BackupRun{}
	_ queue.Detached = BackupRun{}
	_ queue.Releaser = BackupRun{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for. See the type's comment.
func (h BackupRun) OwnsItsTransactions() {}

// Run writes one archive.
func (h BackupRun) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	in, err := performInputOf(job)
	if err != nil {
		return queue.Result{}, err
	}
	in.Report = h.reporter(ctx, job)

	run, err := h.Performer.Perform(ctx, in)
	if err != nil {
		// Another run holds the target. Not a failure to retry into - the work is happening, and
		// a second archive at the same moment is what the lock exists to prevent - but not a
		// success either: this job was asked for an archive and has not written one. It comes
		// back when the target should be free and writes it then, so the job's final state never
		// says SUCCEEDED over work that did not happen (#207).
		if busy(err) {
			slog.InfoContext(ctx, "a backup was already running at that target; coming back",
				slog.String("target_id", in.TargetID.String()))
			return queue.Result{Repeat: true, RepeatAfter: busyRetryDelay}, nil
		}
		return queue.Result{}, err
	}

	h.Expiry.After(ctx, run)
	return queue.Result{}, nil
}

// busyRetryDelay is how long a run waits for its target to free up before it looks again.
//
// Long enough that a legitimate hours-long backup is polled a handful of times rather than
// hammered, short enough that a freed target is picked up the same night the schedule meant. The
// looking is one refused insert - cheap - so the exact number matters less than that there is one.
const busyRetryDelay = 5 * time.Minute

// Release closes the run row of a job the queue has given up on (queue.Releaser, #207).
//
// A run left RUNNING holds the one-run-per-target lock for ever: every later backup at that target
// is refused, and the refusals come back as repeats that never end. The row is closed as FAILED
// with its own code, which frees the target and puts the truth where GetBackupRun reads - and a
// run that is already terminal, or that never claimed its row, answers a conflict this treats as
// done.
func (h BackupRun) Release(ctx context.Context, job queue.Job) {
	in, err := performInputOf(job)
	if err != nil {
		return
	}
	if err := h.Performer.Abandon(ctx, in.RunID, in.TenantID); err != nil {
		slog.WarnContext(ctx, "an abandoned backup run could not be closed; its target stays locked",
			slog.String("run_id", in.RunID.String()),
			slog.String("error", shared.AsError(err).Code))
	}
}

// reporter turns the queue's fence into the callback the performer takes, and swallows what it
// cannot write.
//
// A progress reading is for whoever is watching. A worker that lost its lease writes nothing, and
// a database that refused the update has already failed the run at the next real write - neither
// is a reason to stop a backup that is otherwise going fine.
func (h BackupRun) reporter(ctx context.Context, job queue.Job) func(float64) {
	if h.Progress == nil {
		return nil
	}
	return func(fraction float64) {
		if err := h.Progress.Report(ctx, job, fraction); err != nil {
			slog.DebugContext(ctx, "the progress of a backup was not recorded",
				slog.String("job_id", job.ID.String()), slog.String("error", err.Error()))
		}
	}
}

// BackupVerify is the queue's way into checking an archive at its target.
//
// Detached for the same reason and with less at stake: it reads every member of an archive over
// somebody else's network, which is minutes, and it changes one row at the end.
type BackupVerify struct {
	Performer service.Performer
}

var (
	_ queue.Handler  = BackupVerify{}
	_ queue.Detached = BackupVerify{}
)

func (h BackupVerify) OwnsItsTransactions() {}

// Run checks one archive.
func (h BackupVerify) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	runID, err := identifierIn(job, "run_id")
	if err != nil {
		return queue.Result{}, err
	}

	sound, err := h.Performer.Verify(ctx, runID, job.TenantID)
	if err != nil {
		return queue.Result{}, err
	}
	if !sound {
		// The finding is recorded on the run, and the job succeeded: it was asked to check, and it
		// checked. A failed job would send the answer to the dead letter and retry it seven times
		// against an archive that is not going to heal.
		slog.WarnContext(ctx, "an archive did not verify",
			slog.String("run_id", runID.String()))
	}
	return queue.Result{}, nil
}

// performInputOf reads the payload. A payload the handler cannot read is a defect rather than
// something to retry: the row outlives the process that wrote it, and a shape that changed in
// between is a bug in the change rather than a transient failure.
func performInputOf(job queue.Job) (service.PerformInput, error) {
	runID, err := identifierIn(job, "run_id")
	if err != nil {
		return service.PerformInput{}, err
	}
	targetID, err := identifierIn(job, "target_id")
	if err != nil {
		return service.PerformInput{}, err
	}
	if job.TenantID.IsZero() {
		return service.PerformInput{}, shared.Internalf("backup: a run job without a tenant")
	}

	in := service.PerformInput{
		RunID: runID, TargetID: targetID, TenantID: job.TenantID,
		Mode:         domain.Mode(textIn(job, "mode")),
		Trigger:      domain.Trigger(textIn(job, "trigger")),
		IncludeMedia: flagIn(job, "include_media"),
		IncludeAudit: flagIn(job, "include_audit"),
	}
	if !in.Mode.Valid() {
		return service.PerformInput{}, shared.Internalf("backup: a run job with mode %q", in.Mode)
	}
	if !in.Trigger.Valid() {
		in.Trigger = domain.TriggerAPI
	}
	for name, into := range map[string]*shared.ID{
		"schedule_id": &in.ScheduleID, "parent_run_id": &in.ParentRunID,
	} {
		if raw := textIn(job, name); raw != "" {
			id, err := shared.ParseID(raw)
			if err != nil {
				return service.PerformInput{}, shared.Internalf("backup: a run job naming %s %q", name, raw)
			}
			*into = id
		}
	}
	return in, nil
}

func identifierIn(job queue.Job, field string) (shared.ID, error) {
	raw := textIn(job, field)
	if raw == "" {
		return "", shared.Internalf("backup: a job without %s", field)
	}
	id, err := shared.ParseID(raw)
	if err != nil {
		return "", shared.Internalf("backup: a job naming %s %q", field, raw)
	}
	return id, nil
}

func textIn(job queue.Job, field string) string {
	value, _ := job.Payload[field].(string)
	return value
}

func flagIn(job queue.Job, field string) bool {
	value, _ := job.Payload[field].(bool)
	return value
}

func busy(err error) bool {
	var domainErr *shared.Error
	return errors.As(err, &domainErr) && domainErr.DetailCode == domain.CodeTargetBusy
}

// BackupExpiry applies the generation plan after a run that worked (backup-restore.md §6, BK-8).
//
// **A run with no schedule behind it deletes nothing**, and that is a decision rather than an
// omission. The plan lives on a schedule; a backup somebody asked for by hand has none, and
// inventing a default for it would mean a manual run - the thing people do before something
// risky - could delete the archives they were making it alongside.
//
// Its failures are logged and not returned. The archive is written and recorded by the time this
// runs; failing the job would retry the whole backup in order to retry a deletion, which is the
// wrong operation by several orders of magnitude. The next successful run meets the same archives
// and tries again.
type BackupExpiry struct {
	Performer service.Performer
	Schedules ScheduleSource
}

// ScheduleSource is the slice of the schedules an expiry pass needs: the plan a run was made under.
type ScheduleSource interface {
	Plan(ctx context.Context, tenantID, scheduleID shared.ID) (domain.Retention, string, error)
}

// After applies the plan to the target the run wrote to.
func (e BackupExpiry) After(ctx context.Context, run domain.Run) {
	if e.Schedules == nil || run.ScheduleID.IsZero() {
		return
	}

	plan, zone, err := e.Schedules.Plan(ctx, run.TenantID, run.ScheduleID)
	if err != nil {
		slog.WarnContext(ctx, "the retention plan of a backup schedule could not be read",
			slog.String("schedule_id", run.ScheduleID.String()), slog.String("error", err.Error()))
		return
	}

	expiry, err := e.Performer.Expire(ctx, service.ExpireInput{
		TargetID: run.TargetID, TenantID: run.TenantID, Plan: plan, TimeZone: zone,
	})
	if err != nil {
		slog.WarnContext(ctx, "the generation plan could not be applied at a backup target",
			slog.String("target_id", run.TargetID.String()), slog.String("error", err.Error()))
		return
	}
	if expiry.FloorHeld {
		// The sentence an operator needs when a target fills up: the plan wanted to delete more
		// and was not allowed to.
		slog.InfoContext(ctx, "a retention plan was held at its floor",
			slog.String("target_id", run.TargetID.String()),
			slog.Int("kept", len(expiry.Keep)))
	}
	if len(expiry.ChainHeld) > 0 {
		slog.InfoContext(ctx, "archives were kept because something newer continues them",
			slog.String("target_id", run.TargetID.String()),
			slog.Int("held", len(expiry.ChainHeld)))
	}
}
