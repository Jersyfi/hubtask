// SPDX-License-Identifier: Apache-2.0
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
	"errors"
	"log/slog"
	"sort"
	"sync"
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

	// Only and Except narrow the kinds this runner claims (queue.Lease); both empty claims all.
	Only, Except []queue.Kind

	// Batch is how many jobs one round claims at most.
	Batch int
	// Concurrency is how many jobs run at once. A round claims no more than there are free
	// slots, so a long job holds one slot rather than every job behind it; each running job holds
	// one connection of the background pool. Below one runs one at a time.
	Concurrency int
	// PollInterval is the wait after a round that found nothing. A full batch is followed by the
	// next round at once - a backlog is not a reason to wait.
	PollInterval time.Duration
	// JobTimeout bounds one job. It has to be shorter than Lease, and the constructor refuses a
	// configuration where it is not: a job still running when its lease expires is a job two
	// workers are doing.
	JobTimeout time.Duration
	// LongJobTimeout bounds one pass of a queue.LongRunning handler, whose lease is kept alive
	// while it runs. Zero gives such a handler the ordinary JobTimeout.
	LongJobTimeout time.Duration
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
	// Woken is the doorbell an enqueue rings, so that a job does not wait out the poll interval
	// before anybody looks at it (ADR-0007: "an adaptive polling interval plus LISTEN/NOTIFY as a
	// wake-up"). A channel rather than a listener type, because this layer knows no adapter.
	//
	// Nil is a supported configuration and not a degraded one in any way that matters: the loop
	// then waits out its interval exactly as it always did. That is deliberate - the doorbell is
	// latency, never delivery, and a runner that stopped working when the notification channel
	// did would have turned an optimisation into a dependency.
	Woken <-chan struct{}
	// Diagnose names what a failure's cause was, in attributes safe for a log line: a database
	// error's SQLSTATE and the constraint it broke, never its message, which quotes the row.
	// Injected, because the cause is an adapter's error type and this layer knows no adapter;
	// nil logs the code alone, which is what the rule-10-safe line always carried. What it buys
	// is the difference between "postgres.query_failed" and knowing which statement.
	Diagnose func(error) []slog.Attr
	// SchemaBehind reports whether the database lacks a migration this build embeds. While it does,
	// nothing is claimed: a job run against the older schema may need a column that is not there
	// yet, and the readiness probe already keeps the API from serving (deployment.md §5). The jobs
	// wait for a process whose schema is complete - this one, once its migration has run. Nil
	// claims as before.
	SchemaBehind func(ctx context.Context) bool
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

	slots := newSlots(max(r.Concurrency, 1))
	// A shutdown ends the claiming; the jobs already running finish under their own deadlines
	// before Run returns, as one job did when they ran one after another.
	defer slots.drain()

	slog.InfoContext(ctx, "worker ready",
		slog.Int("batch", r.Batch),
		slog.Int("concurrency", slots.size()),
		slog.Duration("poll_interval", r.PollInterval),
		slog.Duration("job_timeout", r.JobTimeout),
		slog.Bool("woken_by_notification", r.Woken != nil))

	for {
		if !slots.awaitFree(ctx) {
			return
		}
		want := min(r.Batch, slots.free())
		claimed := r.claimAndRun(ctx, want, slots)

		// As many as asked for means there is probably more. Anything less means the queue is
		// drained, and asking again immediately would only be polling faster.
		if claimed >= want {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		if !r.idle(ctx) {
			return
		}
	}
}

// idle waits for the poll interval, or for the doorbell, whichever comes first. It reports whether
// carrying on is still worth it.
//
// The interval is kept rather than replaced: a notification that was missed - a listener between
// connections, a job written by something that is not this application, a trigger dropped by hand -
// costs one interval of latency instead of costing the job. That is what "the poll is the fallback"
// means in practice, and it is why the two are an either/or here rather than a choice made once at
// startup.
func (r Runner) idle(ctx context.Context) bool {
	if r.Woken == nil {
		return wait(ctx, r.PollInterval)
	}
	if r.PollInterval <= 0 {
		select {
		case <-ctx.Done():
			return false
		case <-r.Woken:
			return true
		}
	}

	timer := time.NewTimer(r.PollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	case <-r.Woken:
		return true
	}
}

// round claims a batch and runs it, one job after another.
func (r Runner) round(ctx context.Context) int {
	return r.claimAndRun(ctx, r.Batch, nil)
}

// claimAndRun claims up to want jobs and runs them: each in a slot of its own, or, without
// slots, one after another before it returns. Every failure is logged rather than returned: a
// loop that stopped on the first failed claim would stop for a database that was restarting.
func (r Runner) claimAndRun(ctx context.Context, want int, slots *slots) int {
	claimCtx, cancel := context.WithTimeout(ctx, bookkeepingTimeout)
	defer cancel()

	if r.SchemaBehind != nil && r.SchemaBehind(claimCtx) {
		slog.WarnContext(ctx, "not claiming jobs: the database lacks migrations this build needs")
		return 0
	}

	var jobs []queue.Job
	err := r.UnitOfWork.Within(claimCtx, persistence.SystemScope(), func(txCtx context.Context) error {
		now := r.Clock.Now()
		claimed, err := r.Queue.Claim(txCtx, queue.Lease{
			Now:   now,
			Until: now.Add(r.Lease),
			Batch: want,
			Only:  r.Only, Except: r.Except,
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
		if slots == nil {
			r.start(ctx, job)
			continue
		}
		slots.run(ctx, "worker.job", func(ctx context.Context) { r.start(ctx, job) })
	}
	return len(jobs)
}

// start runs one claimed job, unless its lease could not be made to cover it.
func (r Runner) start(ctx context.Context, job queue.Job) {
	held, ok := r.leased(ctx, job)
	if !ok {
		return
	}
	r.execute(ctx, held)
}

// leased makes sure a job of the batch starts with at least its own timeout of lease left.
//
// The batch shares one lease taken at the claim, and its jobs run one after another: behind two
// jobs that each ran to their timeout, a third would start after its lease had run out - claimable
// again, and run twice, its mail sent twice. Extending is fenced, so a job another worker took over
// meanwhile is skipped rather than run (queue.lease_lost).
func (r Runner) leased(ctx context.Context, job queue.Job) (queue.Job, bool) {
	now := r.Clock.Now()
	if now.Add(r.JobTimeout).Before(job.Lease) {
		return job, true
	}
	extendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
	defer cancel()
	extended := job
	err := r.UnitOfWork.Within(extendCtx, scopeOf(job), func(txCtx context.Context) error {
		var err error
		extended, err = r.Queue.Extend(txCtx, job, now.Add(r.Lease))
		return err
	})
	if err != nil {
		if shared.AsError(err).DetailCode == leaseLost {
			r.letGo(ctx, job)
		} else {
			// Not run, and nothing written: the lease runs out and the next claim takes it.
			slog.WarnContext(ctx, "extending a job's lease failed",
				slog.String("job_id", job.ID.String()),
				slog.String("error", shared.AsError(err).Code))
		}
		return job, false
	}
	return extended, true
}

// execute runs one job in one transaction, together with the record that it was run.
func (r Runner) execute(ctx context.Context, job queue.Job) {
	handler, known := r.Handlers[job.Kind]
	if !known {
		// Not this process's job. It goes back to the queue with a code that says so, and a pod
		// that knows the kind picks it up.
		r.fail(ctx, job, "queue.handler_missing", nil)
		return
	}

	started := r.Clock.Now()
	// The job's deadline is its own, not the loop's. A shutdown stops the claiming, and what has
	// already been claimed is finished rather than cut in half - which is what the pod's grace
	// period is sized for (deployment.md §5). The deadline still bounds it, so a handler that
	// ignores its context cannot hold the shutdown open indefinitely.
	budget := r.JobTimeout
	_, long := handler.(queue.LongRunning)
	if long && r.LongJobTimeout > budget {
		budget = r.LongJobTimeout
	}
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
	defer cancel()

	held := &heldJob{job: job}
	if long && budget > r.JobTimeout {
		r.keepLeased(jobCtx, cancel, held)
		defer held.settle()
	}

	// The span covers the transaction, not just the handler: what the run cost includes what it
	// took to record that the run happened.
	err := r.observe(jobCtx, job.Kind.String(), func(obsCtx context.Context) error {
		if _, detached := handler.(queue.Detached); detached {
			return r.executeDetached(obsCtx, handler, job, held)
		}
		return r.UnitOfWork.Within(obsCtx, scopeOf(job), func(txCtx context.Context) error {
			result, err := run(txCtx, handler, job)
			if err != nil {
				return err
			}
			logCounts(txCtx, job, result)
			if result.Refusal != nil {
				return r.refuse(txCtx, job, *result.Refusal)
			}
			if result.Repeat {
				// A poller: the same row goes back to the queue for its next round rather than
				// finishing, so the deduplication of a pending job keeps it a single row.
				return r.Queue.Repeat(txCtx, job, r.Clock.Now().Add(result.RepeatAfter))
			}
			return r.Queue.Complete(txCtx, job, result.Outcome)
		})
	})
	job = held.settle()
	if err != nil {
		if shared.AsError(err).DetailCode == leaseLost || held.lost() {
			r.letGo(ctx, job)
			return
		}
		r.fail(ctx, job, shared.AsError(err).DetailCode, err)
		return
	}

	if r.Signals != nil {
		r.Signals.JobFinished(ctx, job.Kind.String(), r.Clock.Now().Sub(started).Seconds())
	}
}

// executeDetached runs a handler that opens its own transactions, and records the outcome in a
// short one afterwards (queue.Detached).
//
// The trade the interface names, made concrete: the handler's writes are already committed by the
// time the job is completed, so a process that dies in between leaves the work done and the job
// claimable again. That is why only an idempotent pass may declare itself detached - and why the
// completion is a transaction of its own rather than an afterthought, so a failure to record it is
// a failure the runner handles like any other.
func (r Runner) executeDetached(ctx context.Context, handler queue.Handler, job queue.Job, held *heldJob) error {
	result, err := run(ctx, handler, job)
	if err != nil {
		return err
	}
	logCounts(ctx, job, result)
	// The completion is fenced by the lease as it stands once the heartbeat has stopped moving it.
	job = held.settle()

	return r.UnitOfWork.Within(ctx, scopeOf(job), func(txCtx context.Context) error {
		if result.Refusal != nil {
			return r.refuse(txCtx, job, *result.Refusal)
		}
		if result.Repeat {
			return r.Queue.Repeat(txCtx, job, r.Clock.Now().Add(result.RepeatAfter))
		}
		return r.Queue.Complete(txCtx, job, result.Outcome)
	})
}

// heldJob is the job as its lease currently stands: a long job's heartbeat moves the lease, and the
// completion or failure has to be fenced by the moved one.
type heldJob struct {
	mu   sync.Mutex
	job  queue.Job
	gone bool
	stop func()
	once sync.Once
}

// settle stops the heartbeat, if there is one, and answers the job as its lease then stands.
func (h *heldJob) settle() queue.Job {
	h.once.Do(func() {
		if h.stop != nil {
			h.stop()
		}
	})
	return h.get()
}

func (h *heldJob) get() queue.Job {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.job
}

func (h *heldJob) lost() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.gone
}

// keepLeased extends a long job's lease every third of a lease while it runs, so a killed worker's
// pass is taken over within one lease instead of after the whole long budget. A lease another
// worker took over cancels the pass (queue.lease_lost): two workers must never run it at once.
// held.settle stops the heartbeat and waits for it.
func (r Runner) keepLeased(ctx context.Context, cancel context.CancelFunc, held *heldJob) {
	beatCtx, stopBeat := context.WithCancel(ctx)
	stopped := make(chan struct{})
	held.stop = func() {
		stopBeat()
		<-stopped
	}
	concurrency.Go(beatCtx, "worker.job_lease", func(beatCtx context.Context) {
		defer close(stopped)
		ticker := time.NewTicker(max(r.Lease/3, time.Millisecond))
		defer ticker.Stop()
		for {
			select {
			case <-beatCtx.Done():
				return
			case <-ticker.C:
			}
			extendCtx, done := context.WithTimeout(context.WithoutCancel(beatCtx), bookkeepingTimeout)
			current := held.get()
			var extended queue.Job
			err := r.UnitOfWork.Within(extendCtx, scopeOf(current), func(txCtx context.Context) error {
				var err error
				extended, err = r.Queue.Extend(txCtx, current, r.Clock.Now().Add(r.Lease))
				return err
			})
			done()
			if err == nil {
				held.mu.Lock()
				held.job = extended
				held.mu.Unlock()
				continue
			}
			if shared.AsError(err).DetailCode == leaseLost {
				held.mu.Lock()
				held.gone = true
				held.mu.Unlock()
				cancel()
				return
			}
			// A database that did not answer this once: the lease still has two thirds left,
			// and the next beat tries again.
			slog.WarnContext(beatCtx, "extending a long job's lease failed",
				slog.String("job_id", current.ID.String()),
				slog.String("error", shared.AsError(err).Code))
		}
	})
}

// refuse ends a job that met a refusal (queue.Result.Refusal): FAILED with the refusal's code and
// parameters, in the transaction that would have completed it.
//
// Not through fail, deliberately. fail is for work that broke - a retry, then the dead letter, a
// warning and the count alert A-07 pages on - and a refusal is an answer: asking again meets the
// same one, and there is nothing for an operator to repair. Logged at info, code and identifiers
// only (rule 10).
func (r Runner) refuse(ctx context.Context, job queue.Job, refusal queue.Refusal) error {
	slog.InfoContext(ctx, "job refused",
		slog.String("job_kind", job.Kind.String()),
		slog.String("job_id", job.ID.String()),
		slog.String("error_code", refusal.Code))
	return r.Queue.Fail(ctx, queue.Failure{
		Job: job, Code: refusal.Code, Refused: true, Params: refusal.Params,
	})
}

// leaseLost is the fence answering: the job is no longer this pass's - somebody stopped it, or the
// lease ran out and another worker took it over (infrastructure/postgres Queue.leaseHeld).
const leaseLost = "queue.lease_lost"

// letGo ends a pass whose job is no longer its own, and writes nothing about the job.
//
// Not a failure: nothing broke. A person stopped a question they asked (UC-AI-04 check 9), or the
// lease ran out and the job is somebody else's now - either way the row says what is true, and a
// failure written over it would turn CANCELLED back into a retry, or count a stop as an incident
// in the metric A-07 pages on. One info line, the code and the identifiers only (rule 10).
func (r Runner) letGo(ctx context.Context, job queue.Job) {
	slog.InfoContext(ctx, "job no longer held",
		slog.String("job_kind", job.Kind.String()),
		slog.String("job_id", job.ID.String()),
		slog.String("error_code", leaseLost))
}

// fail writes down what happened: another attempt when the budget allows one, the dead letter when
// it does not.
//
// It runs on a context of its own. The job's context may be cancelled - by its own deadline, or by
// the shutdown that killed the job - and a failure nobody could record is a job that stays
// RUNNING until its lease expires, which is a delay for no reason.
func (r Runner) fail(ctx context.Context, job queue.Job, code string, cause error) {
	if code == "" {
		code = "queue.job_failed"
	}

	var retryAt time.Time
	attemptClass := "final"
	if !job.LastAttempt() {
		retryAt = r.Clock.Now().Add(r.NextAttempt(job.Attempts))
		attemptClass = "retry"
	} else if _, duty := r.Handlers[job.Kind].(queue.Perpetual); duty && transient(cause) {
		r.holdDuty(ctx, job, code, cause)
		return
	}

	if r.Signals != nil {
		r.Signals.JobFailed(ctx, job.Kind.String(), attemptClass)
		if attemptClass == "final" {
			r.Signals.JobDeadLettered(ctx, job.Kind.String())
		}
	}

	// The dead letter is where the queue's account of the work ends, so a handler holding a lock
	// beyond the job row - a run row saying RUNNING - lets it go now rather than for ever.
	// Before the failure is recorded, on the same detached context: a released lock whose dead
	// letter was lost costs one repeated release, while a dead letter whose release was lost
	// costs a target nobody can back up again.
	if attemptClass == "final" {
		if releaser, holds := r.Handlers[job.Kind].(queue.Releaser); holds {
			releaseCtx, cancelRelease := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
			releaser.Release(releaseCtx, job)
			cancelRelease()
		}
	}

	// The code and the identifiers only. What the job was working on may be user content, and a
	// log line is not the place for it (rule 10). The diagnosis adds what the cause's adapter
	// can say without quoting anything: which constraint, which SQLSTATE.
	attrs := []slog.Attr{
		slog.String("job_kind", job.Kind.String()),
		slog.String("job_id", job.ID.String()),
		slog.Int("attempt", job.Attempts),
		slog.String("error_code", code),
		slog.Bool("dead_letter", attemptClass == "final"),
	}
	if r.Diagnose != nil && cause != nil {
		attrs = append(attrs, r.Diagnose(cause)...)
	}
	slog.LogAttrs(ctx, slog.LevelWarn, "job failed", attrs...)

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

// transient is a failure that may answer differently later: the dependency's, a rate limit, or an
// untyped transport or deadline error. A typed refusal, a missing thing or a defect will not.
func transient(cause error) bool {
	if cause == nil {
		return false
	}
	var typed *shared.Error
	if !errors.As(cause, &typed) {
		return true
	}
	return typed.Category == shared.CategoryUnavailable || typed.Category == shared.CategoryRateLimited
}

// holdDuty sends a duty's last failed attempt (queue.Perpetual) to its next round at the backoff's
// ceiling, with its attempts cleared, instead of to the dead letter. Counted as a retry, so the
// failure stays visible on the job metrics; logged like a failure, code and identifiers only.
func (r Runner) holdDuty(ctx context.Context, job queue.Job, code string, cause error) {
	if r.Signals != nil {
		r.Signals.JobFailed(ctx, job.Kind.String(), "retry")
	}
	attrs := []slog.Attr{
		slog.String("job_kind", job.Kind.String()),
		slog.String("job_id", job.ID.String()),
		slog.Int("attempt", job.Attempts),
		slog.String("error_code", code),
		slog.Bool("dead_letter", false),
		slog.Bool("duty_held", true),
	}
	if r.Diagnose != nil {
		attrs = append(attrs, r.Diagnose(cause)...)
	}
	slog.LogAttrs(ctx, slog.LevelWarn, "job failed", attrs...)

	holdCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
	defer cancel()
	next := r.Clock.Now().Add(r.NextAttempt(job.Attempts))
	err := r.UnitOfWork.Within(holdCtx, scopeOf(job), func(txCtx context.Context) error {
		return r.Queue.Repeat(txCtx, job, next)
	})
	if err != nil {
		// As in fail: the job keeps its lease, and the next worker after it expires tries again.
		slog.WarnContext(ctx, "recording a job failure failed",
			slog.String("job_id", job.ID.String()),
			slog.String("error", shared.AsError(err).Code))
	}
}

// logCounts writes down what a handler counted about a job that succeeded, in the job's own
// terms and nothing else: the kind, the identifier, and the numbers by name. Only where there is
// something to say - a job that counted nothing logs nothing.
func logCounts(ctx context.Context, job queue.Job, result queue.Result) {
	if len(result.Counts) == 0 {
		return
	}
	attributes := make([]any, 0, 2+len(result.Counts))
	attributes = append(attributes,
		slog.String("job_kind", job.Kind.String()), slog.String("job_id", job.ID.String()))
	names := make([]string, 0, len(result.Counts))
	for name := range result.Counts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		attributes = append(attributes, slog.Int(name, result.Counts[name]))
	}
	slog.InfoContext(ctx, "job counted", attributes...)
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
