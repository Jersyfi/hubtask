// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package worker is the inbound adapter of the queue channel.
//
// It sits beside rest and mcp rather than in infrastructure for the same reason those do: it
// drives the application layer rather than serving it. A job arrives, a handler runs, something
// happens - the difference to an HTTP request is who asked, not what the layer does with it. That
// is also why nothing here reaches for an adapter: the queue, the transaction, the clock and the
// backoff policy all arrive as ports or as functions, wired at the composition root
// (project-structure.md §2).
//
// What the runner owes the rest of the system, in order of importance:
//
//   - A job takes effect exactly once. The handler's writes and the job's completion are one
//     transaction; a process that dies in between leaves neither, and the lease expiring is what
//     hands the work to somebody else (test RT-3).
//   - Nothing runs without an end. Every claim, every job and every recorded failure carries its
//     own deadline (ADR-0016), and the job deadline is shorter than the lease, so a worker that
//     overran cannot still be writing when its successor starts.
//   - Nothing fails silently. A failure is a retry with backoff, then a dead letter with the code
//     that caused it - and a metric either way (observability-reliability.md §1).
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// Signals is the slice of the metrics adapter this package uses. An interface rather than the
// adapter, so the presentation layer keeps pointing inwards (project-structure.md §2). Nil is
// allowed - a test that only cares about what happened to a job says so by leaving it out.
type Signals interface {
	JobFinished(ctx context.Context, kind string, seconds float64)
	JobFailed(ctx context.Context, kind, attemptClass string)
	JobDeadLettered(ctx context.Context, kind string)
}

// Runner claims jobs and runs them until its context ends.
type Runner struct {
	Queue      queue.Queue
	UnitOfWork persistence.UnitOfWork
	// Handlers is what this process can run. A kind with no handler here is not an error at
	// claim time: during a rolling update the old pods have not learned the new kinds yet, and
	// the job simply waits for a pod that has.
	Handlers map[queue.Kind]queue.Handler
	Clock    clock.Clock
	Signals  Signals

	// Batch is how many jobs one round claims.
	Batch int
	// PollInterval is the wait after a round that found nothing. A full batch is followed by the
	// next round at once - a backlog is not a reason to wait.
	PollInterval time.Duration
	// JobTimeout bounds one job. It has to be shorter than Lease, and the constructor refuses a
	// configuration where it is not: a job still running when its lease expires is a job two
	// workers are doing.
	JobTimeout time.Duration
	// Lease is how long a claim holds. Long enough to outlast a job, short enough that a killed
	// worker's jobs do not sit still for minutes.
	Lease time.Duration
	// NextAttempt is the backoff between attempts, given the number of attempts made so far. It
	// is injected rather than computed here because the policy belongs to the resilience adapter
	// and this layer does not know it (observability-reliability.md §6).
	NextAttempt func(attempt int) time.Duration
	// Observe wraps one job run in its trace span. Injected as a function for the same reason as
	// NextAttempt: this layer knows no adapter, and a tracer is one. Nil leaves the run
	// untraced - which is what a test wants and what a process without tracing configured gets
	// anyway, since the no-op tracer costs nothing.
	Observe func(ctx context.Context, kind string, fn func(context.Context) error) error
}

// bookkeepingTimeout bounds the statements that are not the job itself: claiming a batch, and
// recording a failure. Short on purpose - both are single statements, and a worker that waits a
// minute for one of them is a worker doing nothing while the queue grows.
const bookkeepingTimeout = 15 * time.Second

// Run is the loop. It returns when the context ends, which is what a graceful shutdown does to it:
// no new claim is made, and the job in flight keeps the context it already has until its own
// deadline.
func (r Runner) Run(ctx context.Context) {
	if err := r.validate(); err != nil {
		// A misconfigured runner is not something to discover one lost job at a time.
		slog.ErrorContext(ctx, "the worker did not start", slog.String("error", err.Error()))
		return
	}

	slog.InfoContext(ctx, "worker ready",
		slog.Int("batch", r.Batch),
		slog.Duration("poll_interval", r.PollInterval),
		slog.Duration("job_timeout", r.JobTimeout))

	for {
		claimed := r.round(ctx)

		// A full batch means there is probably more. Anything less means the queue is drained,
		// and asking again immediately would only be polling faster.
		if claimed >= r.Batch {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		if !wait(ctx, r.PollInterval) {
			return
		}
	}
}

// round claims a batch and runs it. Every failure is logged rather than returned: a loop that
// stopped on the first failed claim would stop for a database that was restarting.
func (r Runner) round(ctx context.Context) int {
	claimCtx, cancel := context.WithTimeout(ctx, bookkeepingTimeout)
	defer cancel()

	var jobs []queue.Job
	err := r.UnitOfWork.Within(claimCtx, persistence.SystemScope(), func(txCtx context.Context) error {
		now := r.Clock.Now()
		claimed, err := r.Queue.Claim(txCtx, queue.Lease{
			Now:   now,
			Until: now.Add(r.Lease),
			Batch: r.Batch,
		})
		jobs = claimed
		return err
	})
	if err != nil {
		if ctx.Err() == nil {
			slog.WarnContext(ctx, "claiming jobs failed", slog.String("error", shared.AsError(err).Code))
		}
		return 0
	}

	for _, job := range jobs {
		r.execute(ctx, job)
	}
	return len(jobs)
}

// execute runs one job in one transaction, together with the record that it was run.
func (r Runner) execute(ctx context.Context, job queue.Job) {
	handler, known := r.Handlers[job.Kind]
	if !known {
		// Not this process's job. It goes back to the queue with a code that says so, and a pod
		// that knows the kind picks it up.
		r.fail(ctx, job, "queue.handler_missing")
		return
	}

	started := r.Clock.Now()
	// The job's deadline is its own, not the loop's. A shutdown stops the claiming, and what has
	// already been claimed is finished rather than cut in half - which is what the pod's grace
	// period is sized for (deployment.md §5). The deadline still bounds it, so a handler that
	// ignores its context cannot hold the shutdown open indefinitely.
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.JobTimeout)
	defer cancel()

	// The span covers the transaction, not just the handler: what the run cost includes what it
	// took to record that the run happened.
	err := r.observe(jobCtx, job.Kind.String(), func(obsCtx context.Context) error {
		return r.UnitOfWork.Within(obsCtx, scopeOf(job), func(txCtx context.Context) error {
			result, err := run(txCtx, handler, job)
			if err != nil {
				return err
			}
			if result.Repeat {
				// A poller: the same row goes back to the queue for its next round rather than
				// finishing, so the deduplication of a pending job keeps it a single row.
				return r.Queue.Repeat(txCtx, job, r.Clock.Now().Add(result.RepeatAfter))
			}
			return r.Queue.Complete(txCtx, job)
		})
	})
	if err != nil {
		r.fail(ctx, job, shared.AsError(err).DetailCode)
		return
	}

	if r.Signals != nil {
		r.Signals.JobFinished(ctx, job.Kind.String(), r.Clock.Now().Sub(started).Seconds())
	}
}

// fail writes down what happened: another attempt when the budget allows one, the dead letter when
// it does not.
//
// It runs on a context of its own. The job's context may be cancelled - by its own deadline, or by
// the shutdown that killed the job - and a failure nobody could record is a job that stays
// RUNNING until its lease expires, which is a delay for no reason.
func (r Runner) fail(ctx context.Context, job queue.Job, code string) {
	if code == "" {
		code = "queue.job_failed"
	}

	var retryAt time.Time
	attemptClass := "final"
	if !job.LastAttempt() {
		retryAt = r.Clock.Now().Add(r.NextAttempt(job.Attempts))
		attemptClass = "retry"
	}

	if r.Signals != nil {
		r.Signals.JobFailed(ctx, job.Kind.String(), attemptClass)
		if attemptClass == "final" {
			r.Signals.JobDeadLettered(ctx, job.Kind.String())
		}
	}

	// The code and the identifiers only. What the job was working on may be user content, and a
	// log line is not the place for it (rule 10).
	slog.WarnContext(ctx, "job failed",
		slog.String("job_kind", job.Kind.String()),
		slog.String("job_id", job.ID.String()),
		slog.Int("attempt", job.Attempts),
		slog.String("error_code", code),
		slog.Bool("dead_letter", attemptClass == "final"))

	failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
	defer cancel()

	err := r.UnitOfWork.Within(failCtx, scopeOf(job), func(txCtx context.Context) error {
		return r.Queue.Fail(txCtx, queue.Failure{Job: job, Code: code, RetryAt: retryAt})
	})
	if err != nil {
		// Nothing more to do about it here. The job keeps its lease, and the next worker to come
		// past after it expires tries again - at-least-once covers this too.
		slog.WarnContext(ctx, "recording a job failure failed",
			slog.String("job_id", job.ID.String()),
			slog.String("error", shared.AsError(err).Code))
	}
}

// observe applies the injected span wrapper, or runs fn plainly when there is none.
func (r Runner) observe(ctx context.Context, kind string, fn func(context.Context) error) error {
	if r.Observe == nil {
		return fn(ctx)
	}
	return r.Observe(ctx, kind, fn)
}

// run is the panic guard around a handler. A panic in a job must not take the process with it
// (rule 5), and it must be indistinguishable from a failure for everything downstream: the job is
// retried, then dead-lettered, exactly like one that returned an error.
//
// The recovered value is reported rather than put into the error: it can carry anything, user
// content included, and the redacting logger is where such a thing belongs (rule 10).
func run(ctx context.Context, handler queue.Handler, job queue.Job) (result queue.Result, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			concurrency.Report(ctx, "worker.job."+job.Kind.String(), recovered)
			err = shared.ErrInternal.WithDetail("queue.handler_panicked")
		}
	}()
	return handler.Run(ctx, job)
}

// scopeOf opens the job's transaction under the tenant the job names. Work that belongs to no
// tenant runs under the system scope, which sees no tenant's data at all - a system job that
// tried to read a work item gets nothing, not everything.
func scopeOf(job queue.Job) persistence.Scope {
	if job.TenantID.IsZero() {
		return persistence.SystemScope()
	}
	return persistence.Scope{TenantID: job.TenantID}
}

// wait sleeps unless the context ends first. It reports whether waiting is still worth it.
func wait(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// validate refuses a configuration whose parts contradict each other. Every one of these is a
// defect at the composition root rather than an operator's mistake - the configuration surface
// bounds what an operator can set (infrastructure/environment).
func (r Runner) validate() error {
	switch {
	case r.Queue == nil || r.UnitOfWork == nil || r.Clock == nil || r.NextAttempt == nil:
		return shared.ErrInternal.WithDetail("queue.runner_incomplete")
	case r.Batch < 1:
		return shared.ErrInternal.WithDetail("queue.batch_invalid")
	case r.JobTimeout <= 0 || r.Lease <= 0:
		return shared.ErrInternal.WithDetail("queue.timeout_invalid")
	case r.Lease <= r.JobTimeout:
		// The lease has to outlast the job. Otherwise the successor starts while the predecessor
		// is still working, and both write - which is the one thing the lease is for.
		return shared.ErrInternal.WithDetail("queue.lease_shorter_than_job")
	}
	return nil
}
