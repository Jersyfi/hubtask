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
)

// SchedulerSignals is the slice of the metrics adapter the scheduler uses.
//
// Both of these are measurements of the installation rather than of a process, which is why the
// leader takes them: every replica reporting the same queue depth would leave a dashboard summing
// one number over N instances and calling it a backlog.
type SchedulerSignals interface {
	QueueDepth(ctx context.Context, kind string, pending int64)
	SchedulerTickLag(ctx context.Context, seconds float64)
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
	return true
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
