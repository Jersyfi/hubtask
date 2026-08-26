// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"

	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
)

// SchedulerSignals is the slice of the metrics adapter the scheduler uses.
//
// Both of these are measurements of the installation rather than of a process, which is why the
// leader takes them: every replica reporting the same queue depth would leave a dashboard summing
// one number over N instances and calling it a backlog.
type SchedulerSignals interface {
	QueueDepth(ctx context.Context, kind string, pending int64)
	SchedulerTickLag(ctx context.Context, seconds float64)
	// BackupLastSuccess is when a target last had a backup that worked - alert A-12's number
	// (E-05, observability-reliability.md §10).
	BackupLastSuccess(ctx context.Context, targetID string, at time.Time)
}

// InstanceBackups is the slice of the schedule pass the leader runs: the instance-wide schedules,
// which belong to no tenant and which nothing else can fire.
type InstanceBackups interface {
	Run(ctx context.Context, scope persistence.Scope) (backupservice.PassResult, error)
}

// BackupFreshness answers when each target last had a backup that worked.
type BackupFreshness interface {
	LastSuccessPerTarget(ctx context.Context) (map[shared.ID]time.Time, error)
}

// AuditPartitions keeps the audit trail's partitions conforming (E-09, audit.md §3).
type AuditPartitions interface {
	Ensure(ctx context.Context, month time.Time) (string, error)
}

// Scheduler is the role that may run in several replicas but act in only one (ADR-0008).
//
// It does no work itself beyond deciding what has to happen: everything it decides becomes a job,
// which is what lets the workers scale while the deciding stays single. That split is also what
// keeps the tick short - a leader holding a lock for a long-running task is a leader nobody can
// replace while it holds it.
//
// In 0.1.0 the tick has one duty, and the rest of the schedule - reminders, recurrence,
// retention - registers here as those arrive. The duty it has is not a placeholder: the queue
// depth is the number alert A-06 watches, and it is a property of the installation, so exactly one
// process should be reporting it.
type Scheduler struct {
	Leadership queue.Leadership
	Queue      queue.Queue
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Signals    SchedulerSignals
	// Kinds are the job kinds this installation knows. They are published at zero when the queue
	// holds none of them, because a gauge that has never been written has no series - and an
	// alert on a backlog that never appears is an alert that reads "no data" and is believed
	// (observability-reliability.md §4, alert A-06). The same reasoning seeds the panic counter.
	Kinds []queue.Kind

	// InstanceBackups fires the schedules that belong to no tenant. Optional: an installation
	// without it simply never has one, which is the state every installation is in today.
	InstanceBackups InstanceBackups
	// BackupFreshness is the reading behind alert A-12.
	BackupFreshness BackupFreshness
	// AuditPartitions makes sure next month's partition of `audit_log` exists before the first
	// entry of it does, and carries its own policy and its own revokes. Optional, like the two
	// above: an installation without it keeps writing into the default partition, which has both.
	AuditPartitions AuditPartitions

	// TickInterval is how often the leader looks at the clock. It is also how quickly a standby
	// notices that the leader is gone, because a standby tries the lock on every tick of its own.
	TickInterval time.Duration
}

// Run is the loop. It returns when the context ends, and gives leadership up on the way out so
// that a standby takes over at once rather than waiting for a socket to time out.
func (s Scheduler) Run(ctx context.Context) {
	if err := s.validate(); err != nil {
		slog.ErrorContext(ctx, "the scheduler did not start", slog.String("error", err.Error()))
		return
	}
	defer s.release(ctx)

	slog.InfoContext(ctx, "scheduler ready", slog.Duration("tick_interval", s.TickInterval))

	leading := false
	// The first tick is due immediately, and it is not late.
	due := s.Clock.Now()
	for {
		leading = s.tick(ctx, leading, due)

		due = due.Add(s.TickInterval)
		if !wait(ctx, s.TickInterval) {
			return
		}
	}
}

// tick is one round: confirm or take leadership, and if it is ours, do the duties. It reports
// whether this process is the leader, which is only used to keep the log quiet - the decision
// itself is made against the database every time.
func (s Scheduler) tick(ctx context.Context, wasLeading bool, due time.Time) bool {
	leadingCtx, cancel := context.WithTimeout(ctx, bookkeepingTimeout)
	defer cancel()

	leading, err := s.Leadership.Confirm(leadingCtx)
	if err != nil {
		slog.WarnContext(ctx, "confirming leadership failed", slog.String("error", shared.AsError(err).Code))
		return false
	}
	if !leading {
		leading, err = s.Leadership.Acquire(leadingCtx)
		if err != nil {
			slog.WarnContext(ctx, "acquiring leadership failed", slog.String("error", shared.AsError(err).Code))
			return false
		}
	}

	switch {
	case leading && !wasLeading:
		slog.InfoContext(ctx, "scheduler is the leader")
	case !leading && wasLeading:
		// The lock was lost rather than given up: the connection carrying it went away. Another
		// instance has it by now, and this one stands by - continuing to act would be exactly the
		// double execution the lock exists to prevent.
		slog.WarnContext(ctx, "scheduler lost leadership and is standing by")
	}
	if !leading {
		return false
	}

	// The lag is measured against when the tick was due, not against the last one: a tick that ran
	// late and then on time again would otherwise look punctual, and what an operator wants to know
	// is whether the schedule is drifting (observability-reliability.md §4).
	if s.Signals != nil {
		lag := s.Clock.Now().Sub(due)
		if lag < 0 {
			lag = 0
		}
		s.Signals.SchedulerTickLag(ctx, lag.Seconds())
	}

	s.sampleQueueDepth(ctx)
	s.fireInstanceBackups(ctx)
	s.sampleBackupFreshness(ctx)
	s.ensureAuditPartitions(ctx)
	return true
}

// fireInstanceBackups is the leader's one duty beyond measurement (E-05).
//
// An instance-wide backup schedule - `scope_kind = 'INSTANCE'`, `tenant_id IS NULL`, which
// `0001_init`'s check constraint ties together - is not a tenant's work at all, so nothing seeds a
// job for it and the index `backup_schedule_due_idx ON (next_run_at) WHERE enabled` exists for a
// leader to read. That is a legitimate leader duty and the first one this scheduler has beyond
// sampling. It is emphatically **not** a licence to enumerate tenants: every tenant-scoped schedule
// is fired by that tenant's own poller, seeded by the write that created it, and this pass cannot
// see one even if it wanted to.
//
// It cannot, and the reason is worth writing down where the code is. The pass runs under a system
// scope, which sets an empty tenant context; every tenant-scoped table compares `tenant_id =
// current_tenant_id()` and NULL matches nothing, so the only rows this could reach are the ones
// with no tenant. Today there are none: E-03 found the same for instance-wide *targets*, and until
// instance administration has a surface, nothing can create either. So this duty is correct, cheap,
// and finds nothing - which is the honest state rather than a stub.
func (s Scheduler) fireInstanceBackups(ctx context.Context) {
	if s.InstanceBackups == nil {
		return
	}
	passCtx, cancel := context.WithTimeout(ctx, bookkeepingTimeout)
	defer cancel()

	result, err := s.InstanceBackups.Run(passCtx, persistence.SystemScope())
	if err != nil {
		if ctx.Err() == nil {
			slog.WarnContext(ctx, "the instance-wide backup schedules could not be read",
				slog.String("error", shared.AsError(err).Code))
		}
		return
	}
	if result.Started > 0 {
		slog.InfoContext(ctx, "instance-wide backups started", slog.Int("count", result.Started))
	}
}

// ensureAuditPartitions is the leader's second real duty, and it is a correctness matter rather
// than housekeeping (E-09, audit.md §3).
//
// A partition of `audit_log` does not inherit the parent's row level security policy when it is
// addressed directly, and `REVOKE UPDATE, DELETE, TRUNCATE` on the parent does not reach it either
// - both measured and written down in `0001_init`. A partition created without them is a
// cross-tenant leak with a date on it, and an audit trail the application role can rewrite.
//
// The leader's rather than a tenant's, because a partition belongs to the installation: it covers
// every tenant's entries, and nothing in this system may enumerate tenants anyway. Next month as
// well as this one, so that the partition exists before the first entry of it does - a month whose
// entries have already gone into the default partition cannot be split out afterwards, and the duty
// says so by answering an empty name rather than by failing every minute.
//
// In the steady state it reads the catalogue and writes nothing, which is what makes it safe to run
// on every tick.
func (s Scheduler) ensureAuditPartitions(ctx context.Context) {
	if s.AuditPartitions == nil {
		return
	}
	dutyCtx, cancel := context.WithTimeout(ctx, bookkeepingTimeout)
	defer cancel()

	now := s.Clock.Now().UTC()
	for _, month := range []time.Time{now, now.AddDate(0, 1, 0)} {
		name, err := s.AuditPartitions.Ensure(dutyCtx, month)
		if err != nil {
			if ctx.Err() == nil {
				slog.WarnContext(ctx, "the audit partition could not be ensured",
					slog.String("month", month.Format("2006-01")),
					slog.String("error", shared.AsError(err).Code))
			}
			return
		}
		if name == "" {
			slog.InfoContext(ctx, "the audit entries of that month are in the default partition",
				slog.String("month", month.Format("2006-01")))
		}
	}
}

// sampleBackupFreshness publishes when each target last had a backup that worked - the number alert
// A-12 has been watching since 0.2.0 with nothing behind it (observability-reliability.md §10).
//
// The leader takes it for the reason it takes the queue depth: it is a measurement of the
// installation rather than of a process, and every replica reporting the same value would leave a
// dashboard summing one number over N instances.
//
// A target that has never had a successful backup is absent rather than zero. A gauge of zero reads
// as 1970 on every dashboard, which is an alert that fires for a target nobody has ever backed up -
// true, but not what A-12 means, and the `/meta/health` warning already says that one.
func (s Scheduler) sampleBackupFreshness(ctx context.Context) {
	if s.Signals == nil || s.BackupFreshness == nil {
		return
	}
	sampleCtx, cancel := context.WithTimeout(ctx, bookkeepingTimeout)
	defer cancel()

	moments, err := s.BackupFreshness.LastSuccessPerTarget(sampleCtx)
	if err != nil {
		if ctx.Err() == nil {
			slog.WarnContext(ctx, "sampling the backup freshness failed",
				slog.String("error", shared.AsError(err).Code))
		}
		return
	}
	for targetID, at := range moments {
		s.Signals.BackupLastSuccess(ctx, targetID.String(), at)
	}
}

// sampleQueueDepth publishes the backlog per job kind.
func (s Scheduler) sampleQueueDepth(ctx context.Context) {
	if s.Signals == nil {
		return
	}
	sampleCtx, cancel := context.WithTimeout(ctx, bookkeepingTimeout)
	defer cancel()

	var depths []queue.Depth
	err := s.UnitOfWork.WithinReadOnly(sampleCtx, persistence.SystemScope(), func(txCtx context.Context) error {
		var err error
		depths, err = s.Queue.Depth(txCtx)
		return err
	})
	if err != nil {
		if ctx.Err() == nil {
			slog.WarnContext(ctx, "sampling the queue depth failed", slog.String("error", shared.AsError(err).Code))
		}
		return
	}

	measured := map[queue.Kind]bool{}
	for _, depth := range depths {
		measured[depth.Kind] = true
		s.Signals.QueueDepth(ctx, depth.Kind.String(), int64(depth.Pending))
	}
	// A kind with nothing waiting is not absent from the queue - it is empty, and that is the
	// state an operator most wants to be able to see.
	for _, kind := range s.Kinds {
		if !measured[kind] {
			s.Signals.QueueDepth(ctx, kind.String(), 0)
		}
	}
}

// release gives leadership up on the way out.
//
// It strips the cancellation from the loop's context rather than taking a fresh one: the context
// that arrives here is the one that just ended, and a release that cannot reach the database is
// the difference between a standby taking over now and one taking over when the socket dies. What
// is kept is everything else the context carries - the trace, the request identifiers - because
// this is still the same shutdown.
func (s Scheduler) release(ctx context.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
	defer cancel()

	if err := s.Leadership.Release(ctx); err != nil {
		slog.WarnContext(ctx, "releasing leadership failed", slog.String("error", shared.AsError(err).Code))
	}
}

func (s Scheduler) validate() error {
	switch {
	case s.Leadership == nil || s.Queue == nil || s.UnitOfWork == nil || s.Clock == nil:
		return shared.ErrInternal.WithDetail("queue.scheduler_incomplete")
	case s.TickInterval <= 0:
		return shared.ErrInternal.WithDetail("queue.tick_interval_invalid")
	}
	return nil
}
