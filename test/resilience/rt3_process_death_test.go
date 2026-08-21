// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build resilience

package resilience

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	envport "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/worker"
)

// RT-3, from observability-reliability.md §12: a worker is killed in the middle of a job, and the
// job takes effect exactly once.
//
// The kill is real. A helper process claims the job, applies its effect, and is then sent SIGKILL
// with the transaction still open - no defer runs, no rollback is sent, and the database learns of
// the death only when the socket closes. That is the failure the whole lease mechanism exists for,
// and simulating it inside one process would simulate the part that matters away.
//
// The effect is a row in the queue itself. It is the one table a system job can write - and it
// means the test needs no tenant, no fixture and no driver of its own: counting the effect is
// asking the queue how deep it is.
const (
	// effectKind is the job under test.
	effectKind = queue.Kind("test.effect")
	// markerKind is what its handler writes. Scheduled far into the future so that no runner ever
	// claims one: a marker is evidence, not work.
	markerKind = queue.Kind("test.marker")

	helperEnv    = "HUBTASK_RT3_HELPER_DSN"
	helperSignal = "RT3_EFFECT_APPLIED"

	// jobTimeout and jobLease are short, because the test waits for the lease in real time. The
	// lease still outlasts the job, which is the rule the runner refuses to start without.
	jobTimeout = time.Second
	jobLease   = 3 * time.Second
)

// openUnitOfWork opens a pool the way the server does and hands back the only thing this package
// is allowed to reach the database through. The pool itself stays inside: the driver belongs to
// infrastructure/postgres, and a test that named its type would be holding it (ADR-0010).
func openUnitOfWork(ctx context.Context, t *testing.T, dsn string) *postgres.UnitOfWork {
	t.Helper()
	pool, err := postgres.NewPool(ctx, poolConfig(dsn), envport.RoleWorker)
	if err != nil {
		t.Fatalf("opening the pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return postgres.NewUnitOfWork(pool)
}

func TestRT3AJobSurvivesTheDeathOfItsWorker(t *testing.T) {
	if os.Getenv(helperEnv) != "" {
		t.Skip("this process is the helper")
	}
	ctx := context.Background()
	dsn := testDSN(t)

	unitOfWork := openUnitOfWork(ctx, t, dsn)
	jobs := postgres.NewQueue(clockadapter.NewUUIDv7(clockadapter.System{}), clockadapter.System{})

	if err := unitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		return jobs.Enqueue(ctx, queue.Request{Kind: effectKind, MaxAttempts: 5})
	}); err != nil {
		t.Fatalf("enqueueing the job: %v", err)
	}

	// --- A worker takes the job and is killed while holding it -------------------------------
	helper := exec.CommandContext(ctx, os.Args[0], "-test.run=TestRT3WorkerHelper", "-test.v")
	helper.Env = append(os.Environ(), helperEnv+"="+dsn)
	// Both streams through one pipe: when the helper fails to get as far as its effect, what it
	// said on the way is the only explanation there is.
	output, writer := io.Pipe()
	helper.Stdout, helper.Stderr = writer, writer
	if err := helper.Start(); err != nil {
		t.Fatalf("starting the helper: %v", err)
	}
	// A helper left behind by a failed assertion would hold its claim until its lease expired and
	// would keep a connection open for as long as the suite runs.
	t.Cleanup(func() { _ = helper.Process.Kill() })

	waitForSignal(ctx, t, output)
	if err := helper.Process.Kill(); err != nil {
		t.Fatalf("killing the helper: %v", err)
	}
	_ = helper.Wait()

	// Nothing of the killed attempt survived: the effect was written inside the transaction that
	// would have completed the job, and neither was committed.
	if applied := effectsApplied(ctx, t, unitOfWork); applied != 0 {
		t.Fatalf("%d effects survived a killed worker, want none", applied)
	}

	// --- The lease expires, and another worker finishes the job ------------------------------
	time.Sleep(jobLease)

	runner := worker.Runner{
		Queue:      jobs,
		UnitOfWork: unitOfWork,
		Handlers:   map[queue.Kind]queue.Handler{effectKind: effectHandler{jobs: jobs}},
		Clock:      clockadapter.System{},
		Batch:      5,
		JobTimeout: jobTimeout,
		Lease:      jobLease,
		// One attempt is enough here; the retry policy is not what this test is about.
		PollInterval: 50 * time.Millisecond,
		NextAttempt:  func(int) time.Duration { return time.Second },
	}
	successor, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	runUntil(successor, runner, func() bool { return effectsApplied(ctx, t, unitOfWork) > 0 })

	if applied := effectsApplied(ctx, t, unitOfWork); applied != 1 {
		t.Errorf("the job took effect %d times, want exactly once", applied)
	}
	if pending := depthOf(ctx, t, unitOfWork, effectKind); pending != 0 {
		t.Errorf("%d of the job are still waiting, want it finished", pending)
	}
}

// TestRT3WorkerHelper is the process that gets killed. It is a test function because that is how a
// Go test binary re-executes itself; it does nothing unless the parent points it at a database.
func TestRT3WorkerHelper(t *testing.T) {
	dsn := os.Getenv(helperEnv)
	if dsn == "" {
		t.Skip("not the helper")
	}
	ctx := context.Background()

	jobs := postgres.NewQueue(clockadapter.NewUUIDv7(clockadapter.System{}), clockadapter.System{})

	runner := worker.Runner{
		Queue:        jobs,
		UnitOfWork:   openUnitOfWork(ctx, t, dsn),
		Handlers:     map[queue.Kind]queue.Handler{effectKind: dyingHandler{jobs: jobs}},
		Clock:        clockadapter.System{},
		Batch:        1,
		PollInterval: 50 * time.Millisecond,
		// The job budget stays under the lease, as every runner requires. Neither bounds this
		// process: the handler ignores its context on purpose, and the parent is what ends it.
		JobTimeout:  jobTimeout,
		Lease:       jobLease,
		NextAttempt: func(int) time.Duration { return time.Second },
	}
	// The parent kills this process; the context is only here so that a helper left behind by a
	// failed test does not run forever.
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	runner.Run(bounded)
}

// dyingHandler applies the effect and then never returns. What it leaves behind is an open
// transaction holding an uncommitted write and a claimed job - the exact state a SIGKILL produces
// in the middle of real work.
type dyingHandler struct{ jobs postgres.Queue }

func (h dyingHandler) Run(ctx context.Context, _ queue.Job) (queue.Result, error) {
	if err := applyEffect(ctx, h.jobs); err != nil {
		return queue.Result{}, err
	}
	// The parent reads this line to know when to kill this process.
	os.Stdout.WriteString(helperSignal + "\n")

	// Deliberately not select{} on the context: a handler that noticed the shutdown would end the
	// transaction cleanly, which is the case this test is not about.
	time.Sleep(2 * time.Minute)
	return queue.Result{}, nil
}

// effectHandler is the same work without the dying.
type effectHandler struct{ jobs postgres.Queue }

func (h effectHandler) Run(ctx context.Context, _ queue.Job) (queue.Result, error) {
	return queue.Result{}, applyEffect(ctx, h.jobs)
}

// applyEffect writes the one row that stands for "the job happened". It runs in the transaction
// the runner opened, which is what ties it to the job's completion.
func applyEffect(ctx context.Context, jobs postgres.Queue) error {
	return jobs.Enqueue(ctx, queue.Request{
		Kind: markerKind,
		// Far enough out that no worker claims it. A marker is evidence, not work.
		RunAt: time.Now().Add(24 * time.Hour),
	})
}

// effectsApplied counts the markers, by asking the queue how deep it is at a moment far enough in
// the future for the markers to be due.
func effectsApplied(ctx context.Context, t *testing.T, unitOfWork *postgres.UnitOfWork) int {
	t.Helper()
	return depthAt(ctx, t, unitOfWork, markerKind, clockport.Fixed(time.Now().Add(48*time.Hour)))
}

func depthOf(ctx context.Context, t *testing.T, unitOfWork *postgres.UnitOfWork, kind queue.Kind) int {
	t.Helper()
	return depthAt(ctx, t, unitOfWork, kind, clockadapter.System{})
}

func depthAt(ctx context.Context, t *testing.T, unitOfWork *postgres.UnitOfWork, kind queue.Kind, at clockport.Clock) int {
	t.Helper()
	var depths []queue.Depth
	err := unitOfWork.WithinReadOnly(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		var err error
		depths, err = postgres.NewQueue(clockadapter.NewUUIDv7(clockadapter.System{}), at).Depth(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("reading the queue depth: %v", err)
	}
	for _, depth := range depths {
		if depth.Kind == kind {
			return depth.Pending
		}
	}
	return 0
}

// runUntil runs the worker loop until the condition holds or the context ends.
func runUntil(ctx context.Context, runner worker.Runner, done func() bool) {
	loop, cancel := context.WithCancel(ctx)
	defer cancel()

	finished := make(chan struct{})
	concurrency.Go(loop, "test.rt3.successor", func(ctx context.Context) {
		runner.Run(ctx)
		close(finished)
	})

	for {
		if done() {
			cancel()
			<-finished
			return
		}
		select {
		case <-ctx.Done():
			<-finished
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// waitForSignal reads the helper's output until it says the effect has been written.
//
// It has a deadline of its own: a helper that dies before it gets that far leaves its end of the
// pipe held by this process, so a plain read would wait for a line nobody is going to write.
func waitForSignal(ctx context.Context, t *testing.T, output io.Reader) {
	t.Helper()

	said := make(chan string, 64)
	found := make(chan struct{})
	reading, stop := context.WithCancel(ctx)
	defer stop()

	concurrency.Go(reading, "test.rt3.helper-output", func(context.Context) {
		lines := bufio.NewScanner(output)
		for lines.Scan() {
			select {
			case said <- lines.Text():
			default:
			}
			if strings.Contains(lines.Text(), helperSignal) {
				close(found)
				return
			}
		}
	})

	select {
	case <-found:
		return
	case <-time.After(90 * time.Second):
	}

	var transcript []string
	for {
		select {
		case line := <-said:
			transcript = append(transcript, line)
		default:
			t.Fatalf("the helper never applied its effect. It said:\n%s", strings.Join(transcript, "\n"))
		}
	}
}

// poolConfig is the configuration a pool needs and nothing else: this suite is not testing the
// configuration surface.
func poolConfig(dsn string) envport.Config {
	return envport.Config{
		Database: envport.DatabaseConfig{
			DSN:                    secret.New(dsn),
			MaxConns:               5,
			MinConns:               1,
			MaxConnLifetime:        time.Hour,
			MaxConnIdleTime:        time.Minute,
			ConnectTimeout:         10 * time.Second,
			StatementTimeout:       10 * time.Second,
			WorkerStatementTimeout: 30 * time.Second,
		},
	}
}
