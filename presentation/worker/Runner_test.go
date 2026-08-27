// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

var (
	now      = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	tenantID = shared.ID("018f3a1c-0000-7000-8000-00000000000a")
	jobID    = shared.ID("018f3a1c-0000-7000-8000-000000000001")
)

// unitOfWork records the scope every transaction was opened under, because that is where the
// tenant boundary of a job lives: the job names a tenant, and the work happens inside it.
type unitOfWork struct {
	scopes []persistence.Scope
}

func (u *unitOfWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, scope, fn)
}

type queueDouble struct {
	claimable []queue.Job
	held      []shared.ID
	completed []shared.ID
	repeated  map[shared.ID]time.Time
	failures  []queue.Failure
}

func newQueue(jobs ...queue.Job) *queueDouble {
	return &queueDouble{claimable: jobs, repeated: map[shared.ID]time.Time{}}
}

func (q *queueDouble) Enqueue(context.Context, queue.Request) (shared.ID, error) { return "", nil }

func (q *queueDouble) Claim(_ context.Context, lease queue.Lease) ([]queue.Job, error) {
	claimed := q.claimable
	if len(claimed) > lease.Batch {
		claimed = claimed[:lease.Batch]
	}
	q.claimable = nil
	return claimed, nil
}

func (q *queueDouble) Hold(_ context.Context, job queue.Job) error {
	q.held = append(q.held, job.ID)
	return nil
}

func (q *queueDouble) Complete(_ context.Context, job queue.Job) error {
	q.completed = append(q.completed, job.ID)
	return nil
}

func (q *queueDouble) Repeat(_ context.Context, job queue.Job, runAt time.Time) error {
	q.repeated[job.ID] = runAt
	return nil
}

func (q *queueDouble) Fail(_ context.Context, failure queue.Failure) error {
	q.failures = append(q.failures, failure)
	return nil
}

func (q *queueDouble) Depth(context.Context) ([]queue.Depth, error) { return nil, nil }

type handlerFunc func(context.Context, queue.Job) (queue.Result, error)

func (h handlerFunc) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	return h(ctx, job)
}

type signalsDouble struct {
	finished    []string
	failed      []string
	deadLetters []string
}

func (s *signalsDouble) JobFinished(_ context.Context, kind string, _ float64) {
	s.finished = append(s.finished, kind)
}

func (s *signalsDouble) JobFailed(_ context.Context, kind, attemptClass string) {
	s.failed = append(s.failed, kind+"/"+attemptClass)
}

func (s *signalsDouble) JobDeadLettered(_ context.Context, kind string) {
	s.deadLetters = append(s.deadLetters, kind)
}

func job(attempts int) queue.Job {
	return queue.Job{
		ID:          jobID,
		TenantID:    tenantID,
		Kind:        queue.KindOutboxDispatch,
		Attempts:    attempts,
		MaxAttempts: 3,
		Lease:       now.Add(time.Minute),
	}
}

func runner(jobs *queueDouble, work *unitOfWork, handler queue.Handler, signals Signals) Runner {
	return Runner{
		Queue:        jobs,
		UnitOfWork:   work,
		Handlers:     map[queue.Kind]queue.Handler{queue.KindOutboxDispatch: handler},
		Clock:        clock.Fixed(now),
		Signals:      signals,
		Batch:        10,
		PollInterval: time.Second,
		JobTimeout:   30 * time.Second,
		Lease:        time.Minute,
		NextAttempt:  func(attempt int) time.Duration { return time.Duration(attempt) * time.Second },
	}
}

// The ordinary case: the handler runs and the job is finished in the same transaction. That the
// two are one transaction is what a real database has to prove (test RT-3); what this checks is
// that the runner asks for it at all.
func TestAFinishedJobIsCompletedInsideTheHandlersTransaction(t *testing.T) {
	jobs := newQueue()
	work := &unitOfWork{}
	signals := &signalsDouble{}

	var sawTransaction bool
	handler := handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
		sawTransaction = len(work.scopes) == 1
		return queue.Result{}, nil
	})

	runner(jobs, work, handler, signals).execute(t.Context(), job(1))

	if !sawTransaction {
		t.Error("the handler ran outside a transaction")
	}
	if len(jobs.completed) != 1 || jobs.completed[0] != jobID {
		t.Errorf("completed = %v, want the job", jobs.completed)
	}
	if len(signals.finished) != 1 {
		t.Errorf("finished signals = %v, want one", signals.finished)
	}
}

// A poller does not finish. Its row goes back to the queue for the next round, because a job that
// completed would have to be created again - and the deduplication of a pending job is what keeps
// one dispatcher per tenant rather than a growing pile of them.
func TestAPollerIsRescheduledRatherThanCompleted(t *testing.T) {
	jobs := newQueue()
	handler := handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
		return queue.Result{Repeat: true, RepeatAfter: 5 * time.Second}, nil
	})

	runner(jobs, &unitOfWork{}, handler, nil).execute(t.Context(), job(1))

	if len(jobs.completed) != 0 {
		t.Error("a poller was completed")
	}
	if runAt, ok := jobs.repeated[jobID]; !ok || !runAt.Equal(now.Add(5*time.Second)) {
		t.Errorf("next round at %v, want %v", runAt, now.Add(5*time.Second))
	}
}

// A failure with attempts left is a retry, and the wait between attempts comes from the injected
// policy rather than from here.
func TestAFailureWithAttemptsLeftGoesBackToTheQueue(t *testing.T) {
	jobs := newQueue()
	signals := &signalsDouble{}
	handler := handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
		return queue.Result{}, shared.ErrUnavailable.WithDetail("dependency.unavailable")
	})

	runner(jobs, &unitOfWork{}, handler, signals).execute(t.Context(), job(1))

	if len(jobs.failures) != 1 {
		t.Fatalf("%d failures recorded, want one", len(jobs.failures))
	}
	failure := jobs.failures[0]
	if failure.Code != "dependency.unavailable" {
		t.Errorf("code %q, want the handler's", failure.Code)
	}
	if !failure.RetryAt.Equal(now.Add(time.Second)) {
		t.Errorf("retry at %v, want %v", failure.RetryAt, now.Add(time.Second))
	}
	if len(signals.deadLetters) != 0 {
		t.Error("a retry was counted as a dead letter")
	}
}

// The last attempt is the dead letter: no retry time, a counter that an alert watches, and the
// code that caused it kept on the row for whoever has to act on it (alert A-07).
func TestTheLastAttemptEndsInTheDeadLetter(t *testing.T) {
	jobs := newQueue()
	signals := &signalsDouble{}
	handler := handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
		return queue.Result{}, shared.ErrInternal.WithDetail("outbox.dispatch_without_tenant")
	})

	runner(jobs, &unitOfWork{}, handler, signals).execute(t.Context(), job(3))

	if len(jobs.failures) != 1 {
		t.Fatalf("%d failures recorded, want one", len(jobs.failures))
	}
	if !jobs.failures[0].RetryAt.IsZero() {
		t.Errorf("retry at %v, want the dead letter", jobs.failures[0].RetryAt)
	}
	if len(signals.deadLetters) != 1 {
		t.Errorf("dead letter signals = %v, want one", signals.deadLetters)
	}
}

// releasingHandler records what the runner asked it to let go of (queue.Releaser).
type releasingHandler struct {
	handlerFunc
	released []queue.Job
}

func (h *releasingHandler) Release(_ context.Context, job queue.Job) {
	h.released = append(h.released, job)
}

// A handler holding a lock beyond the job row is asked to let it go exactly when the queue gives
// up - at the dead letter, and not on an attempt that will be retried (#207). A run row left
// RUNNING by a dead job would hold its target's lock for ever.
func TestADeadLetteredJobReleasesWhatItHolds(t *testing.T) {
	failing := func(context.Context, queue.Job) (queue.Result, error) {
		return queue.Result{}, shared.ErrInternal.WithDetail("outbox.dispatch_without_tenant")
	}

	t.Run("the dead letter releases", func(t *testing.T) {
		jobs := newQueue()
		handler := &releasingHandler{handlerFunc: failing}

		runner(jobs, &unitOfWork{}, handler, &signalsDouble{}).execute(t.Context(), job(3))

		if len(handler.released) != 1 {
			t.Fatalf("released %d times, want once", len(handler.released))
		}
		if handler.released[0].ID != jobID {
			t.Errorf("released job %s, want %s", handler.released[0].ID, jobID)
		}
	})

	t.Run("a retry does not", func(t *testing.T) {
		jobs := newQueue()
		handler := &releasingHandler{handlerFunc: failing}

		runner(jobs, &unitOfWork{}, handler, &signalsDouble{}).execute(t.Context(), job(1))

		if len(handler.released) != 0 {
			t.Errorf("released %d times on an attempt that will be retried", len(handler.released))
		}
	})
}

// A panic in a job is a failed job, not a dead process (rule 5). It is retried like any other
// failure, and the value that was thrown does not travel into the error - it can carry anything.
func TestAPanickingHandlerFailsTheJobRatherThanTheProcess(t *testing.T) {
	jobs := newQueue()
	handler := handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
		panic("a nil map, a bad cast, anything at all")
	})

	runner(jobs, &unitOfWork{}, handler, nil).execute(t.Context(), job(1))

	if len(jobs.failures) != 1 {
		t.Fatalf("%d failures recorded, want one", len(jobs.failures))
	}
	if jobs.failures[0].Code != "queue.handler_panicked" {
		t.Errorf("code %q, want queue.handler_panicked", jobs.failures[0].Code)
	}
}

// A kind this process does not know is not an error worth a dead letter on the first attempt:
// during a rolling update the old pods have not learned the new kinds yet, and the job waits for
// one that has.
func TestAnUnknownKindGoesBackToTheQueue(t *testing.T) {
	jobs := newQueue()
	unknown := job(1)
	unknown.Kind = queue.Kind("reminder.fire")

	runner(jobs, &unitOfWork{}, handlerFunc(nil), nil).execute(t.Context(), unknown)

	if len(jobs.failures) != 1 {
		t.Fatalf("%d failures recorded, want one", len(jobs.failures))
	}
	if jobs.failures[0].Code != "queue.handler_missing" {
		t.Errorf("code %q, want queue.handler_missing", jobs.failures[0].Code)
	}
	if jobs.failures[0].RetryAt.IsZero() {
		t.Error("the job was dead-lettered although it has attempts left")
	}
}

// The tenant of the job is the tenant of the transaction. Work that belongs to no tenant runs
// under the system scope, which sees no tenant's data at all.
func TestTheTransactionIsOpenedForTheJobsTenant(t *testing.T) {
	cases := []struct {
		name  string
		job   queue.Job
		scope persistence.Scope
	}{
		{"a tenant's job", job(1), persistence.Scope{TenantID: tenantID}},
		{"system work", queue.Job{ID: jobID, Kind: queue.KindOutboxDispatch, MaxAttempts: 3, Lease: now.Add(time.Minute)}, persistence.SystemScope()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			work := &unitOfWork{}
			handler := handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
				return queue.Result{}, nil
			})

			runner(newQueue(), work, handler, nil).execute(t.Context(), c.job)

			if len(work.scopes) != 1 {
				t.Fatalf("%d transactions, want one", len(work.scopes))
			}
			if work.scopes[0] != c.scope {
				t.Errorf("scope %+v, want %+v", work.scopes[0], c.scope)
			}
		})
	}
}

// A round claims and runs. The batch bounds the claim, and the loop asks for the next round
// without waiting only when it got a full one.
func TestARoundRunsWhatItClaimed(t *testing.T) {
	first, second := job(1), job(1)
	second.ID = shared.ID("018f3a1c-0000-7000-8000-000000000002")
	jobs := newQueue(first, second)

	var ran int
	handler := handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
		ran++
		return queue.Result{}, nil
	})

	if claimed := runner(jobs, &unitOfWork{}, handler, nil).round(t.Context()); claimed != 2 {
		t.Errorf("claimed = %d, want 2", claimed)
	}
	if ran != 2 {
		t.Errorf("%d jobs ran, want 2", ran)
	}
}

// The contradictions a composition root can produce. The one that matters is the last: a lease
// that expires while the job is still running is a job two workers are doing.
func TestAContradictoryConfigurationDoesNotStart(t *testing.T) {
	valid := runner(newQueue(), &unitOfWork{}, handlerFunc(nil), nil)

	cases := []struct {
		name string
		with func(Runner) Runner
		want string
	}{
		{"a runner without its ports", func(r Runner) Runner { r.Queue = nil; return r }, "queue.runner_incomplete"},
		{"a batch of nothing", func(r Runner) Runner { r.Batch = 0; return r }, "queue.batch_invalid"},
		{"no job deadline", func(r Runner) Runner { r.JobTimeout = 0; return r }, "queue.timeout_invalid"},
		{"a lease shorter than the job", func(r Runner) Runner { r.Lease = time.Second; return r }, "queue.lease_shorter_than_job"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.with(valid).validate()
			if err == nil || shared.AsError(err).DetailCode != c.want {
				t.Errorf("error %v, want %s", err, c.want)
			}
		})
	}

	if err := valid.validate(); err != nil {
		t.Errorf("the valid configuration was refused: %v", err)
	}
}

// A shutdown stops the claiming, not the work. The job that was already claimed runs to its own
// deadline and is completed, because the alternative is a transaction rolled back at the moment a
// pod is replaced - work that is not lost, but is done twice for no reason (deployment.md §5).
func TestAClaimedJobIsFinishedEvenWhenTheLoopIsShuttingDown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	jobs := newQueue()
	var ran bool
	handler := handlerFunc(func(jobCtx context.Context, _ queue.Job) (queue.Result, error) {
		ran = jobCtx.Err() == nil
		return queue.Result{}, nil
	})

	runner(jobs, &unitOfWork{}, handler, nil).execute(ctx, job(1))

	if !ran {
		t.Error("the handler was handed a context that had already ended")
	}
	if len(jobs.completed) != 1 {
		t.Errorf("completed = %v, want the claimed job to have been finished", jobs.completed)
	}
}

// The loop ends when its context does. A worker that kept claiming through a shutdown would take
// work it cannot finish before the grace period runs out.
func TestTheLoopStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Through SafeGo like every other goroutine in this repository (rule 5) - the architecture
	// gate reads test files too, and rightly so: a panic in a test goroutine is as fatal as one
	// anywhere else.
	done := make(chan struct{})
	concurrency.Go(ctx, "test.worker.loop", func(context.Context) {
		runner(newQueue(), &unitOfWork{}, handlerFunc(nil), nil).Run(ctx)
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the loop is still running after its context ended")
	}
}

// A job run is observed, and the observation wraps the transaction rather than sitting beside it:
// what a run cost includes what it took to record that the run happened (gate RT-12, extended to
// job handlers).
func TestAJobRunIsObservedAroundItsTransaction(t *testing.T) {
	work := &unitOfWork{}
	jobs := newQueue()

	var observedKind string
	var transactionsWhenObserved int
	handler := handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
		return queue.Result{}, nil
	})

	r := runner(jobs, work, handler, nil)
	r.Observe = func(ctx context.Context, kind string, fn func(context.Context) error) error {
		observedKind = kind
		transactionsWhenObserved = len(work.scopes)
		return fn(ctx)
	}
	r.execute(t.Context(), job(1))

	if observedKind != queue.KindOutboxDispatch.String() {
		t.Errorf("observed kind %q, want %q", observedKind, queue.KindOutboxDispatch)
	}
	if transactionsWhenObserved != 0 {
		t.Error("the observation started inside the transaction rather than around it")
	}
	if len(jobs.completed) != 1 {
		t.Error("the observed job was not completed")
	}
}

// The failure reaches the observer too - a span that ended without its error is a span that says
// the job was fine.
func TestAFailingJobIsObservedAsFailing(t *testing.T) {
	handler := handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
		return queue.Result{}, shared.ErrUnavailable.WithDetail("dependency.unavailable")
	})

	var observed error
	r := runner(newQueue(), &unitOfWork{}, handler, nil)
	r.Observe = func(ctx context.Context, _ string, fn func(context.Context) error) error {
		observed = fn(ctx)
		return observed
	}
	r.execute(t.Context(), job(1))

	if observed == nil {
		t.Error("the observer saw a successful run of a job that failed")
	}
}

// detachedHandler is a handler that declares it opens its own transactions (queue.Detached).
type detachedHandler struct{ handlerFunc }

func (detachedHandler) OwnsItsTransactions() {}

// The one exception to the transaction the runner otherwise holds around a handler: a pass that has
// to reach a bucket between two writes cannot run inside one (observability-reliability.md §8).
func TestADetachedHandlerRunsOutsideTheRunnersTransaction(t *testing.T) {
	jobs := newQueue()
	work := &unitOfWork{}

	var openWhileRunning int
	handler := detachedHandler{handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
		openWhileRunning = len(work.scopes)
		return queue.Result{}, nil
	})}

	runner(jobs, work, handler, nil).execute(t.Context(), job(1))

	if openWhileRunning != 0 {
		t.Errorf("%d transactions were open while the handler ran, want none", openWhileRunning)
	}
	// The completion is still a transaction, and still the job's own scope: what the handler gives
	// up is atomicity with its own work, not the tenant boundary.
	if len(jobs.completed) != 1 {
		t.Errorf("completed = %v, want the job", jobs.completed)
	}
	if len(work.scopes) != 1 || work.scopes[0].TenantID != tenantID {
		t.Errorf("the completion ran as %+v", work.scopes)
	}
}

// A detached poller is rescheduled exactly as a wrapped one is.
func TestADetachedPollerIsRescheduled(t *testing.T) {
	jobs := newQueue()
	handler := detachedHandler{handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
		return queue.Result{Repeat: true, RepeatAfter: time.Minute}, nil
	})}

	runner(jobs, &unitOfWork{}, handler, nil).execute(t.Context(), job(1))

	if len(jobs.repeated) != 1 {
		t.Fatalf("repeated = %v, want the job", jobs.repeated)
	}
	if len(jobs.completed) != 0 {
		t.Error("a poller was completed")
	}
}

// A detached handler that fails is failed like any other: the runner records the attempt on a
// context of its own, so nothing about the exception changes what happens to a bad pass.
func TestADetachedFailureStillGoesBackToTheQueue(t *testing.T) {
	jobs := newQueue()
	handler := detachedHandler{handlerFunc(func(context.Context, queue.Job) (queue.Result, error) {
		return queue.Result{}, shared.ErrUnavailable.WithDetail("dependency.object_storage_unavailable")
	})}

	runner(jobs, &unitOfWork{}, handler, nil).execute(t.Context(), job(1))

	if len(jobs.failures) != 1 {
		t.Fatalf("failures = %v, want the job", jobs.failures)
	}
	if jobs.failures[0].Code != "dependency.object_storage_unavailable" {
		t.Errorf("the failure is recorded as %q", jobs.failures[0].Code)
	}
}
