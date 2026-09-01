// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// queueClock is the time the queue is told about. Nothing here waits for a lease to expire in
// real time: the lease is a value in a column, and the claim compares it against a reading the
// caller supplies.
var queueClock = clock.Fixed(time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC))

func jobQueue(t *testing.T) postgres.Queue {
	t.Helper()
	return postgres.NewQueue(clockadapter.NewUUIDv7(clockadapter.System{}), queueClock)
}

// systemWrite runs fn under the system scope: no tenant, and therefore nothing visible but the one
// table the schema deliberately left without a policy.
func systemWrite(ctx context.Context, t *testing.T, fn func(context.Context) error) error {
	t.Helper()
	return postgres.NewUnitOfWork(appPool(ctx, t)).Within(ctx, persistence.SystemScope(), fn)
}

// enqueue puts one job in and discards the identifier it answers: these tests read a job back
// through its deduplication key, which is what a worker does.
func enqueue(ctx context.Context, t *testing.T, request queue.Request) {
	t.Helper()
	if err := systemWrite(ctx, t, func(ctx context.Context) error {
		_, err := jobQueue(t).Enqueue(ctx, request)
		return err
	}); err != nil {
		t.Fatalf("enqueueing a %s job: %v", request.Kind, err)
	}
}

// claim takes a batch under a lease that ends at now+lease.
func claim(ctx context.Context, t *testing.T, now time.Time, lease time.Duration, batch int) []queue.Job {
	t.Helper()
	var claimed []queue.Job
	if err := systemWrite(ctx, t, func(ctx context.Context) error {
		var err error
		claimed, err = jobQueue(t).Claim(ctx, queue.Lease{Now: now, Until: now.Add(lease), Batch: batch})
		return err
	}); err != nil {
		t.Fatalf("claiming: %v", err)
	}
	return claimed
}

// jobRow reads a job the way an operator would: straight from the table, past every adapter.
func jobRow(ctx context.Context, t *testing.T, id shared.ID) (state string, attempts int, lastError *string, tenant *string, runAt time.Time) {
	t.Helper()
	admin := adminPool(ctx, t)
	if err := admin.QueryRow(ctx,
		`SELECT state, attempts, last_error, tenant_id::text, run_at FROM job WHERE id = $1`, id.String()).
		Scan(&state, &attempts, &lastError, &tenant, &runAt); err != nil {
		t.Fatalf("reading job %s: %v", id, err)
	}
	return state, attempts, lastError, tenant, runAt
}

// A kind of this suite's own, so that a test never collides with the dispatch jobs the write path
// creates for itself.
const testKind = queue.Kind("test.claim")

// Two workers claiming at the same moment divide the queue instead of queueing behind each other.
// That is what FOR UPDATE SKIP LOCKED buys, and it only shows with two transactions open at once -
// a second claim after the first has committed would be skipped by the state alone.
func TestTwoWorkersClaimingAtOnceGetDisjointBatches(t *testing.T) {
	ctx := context.Background()
	dedupe := freshID(t).String()
	enqueue(ctx, t, queue.Request{Kind: testKind, DedupeKey: dedupe + "-1", RunAt: queueClock.Now()})
	enqueue(ctx, t, queue.Request{Kind: testKind, DedupeKey: dedupe + "-2", RunAt: queueClock.Now()})

	// The first claim stays open while the second runs, which is the only arrangement in which
	// the row lock of the first is visible to the second.
	firstClaimed := make(chan []queue.Job, 1)
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	concurrency.Go(ctx, "test.queue.first", func(ctx context.Context) {
		defer wg.Done()
		var jobs []queue.Job
		err := systemWrite(ctx, t, func(txCtx context.Context) error {
			var err error
			jobs, err = jobQueue(t).Claim(txCtx, queue.Lease{
				Now: queueClock.Now(), Until: queueClock.Now().Add(time.Minute), Batch: 1,
			})
			firstClaimed <- jobs
			<-release
			return err
		})
		if err != nil {
			t.Errorf("the first claim failed: %v", err)
		}
	})

	first := <-firstClaimed
	second := claim(ctx, t, queueClock.Now(), time.Minute, 1)
	close(release)
	wg.Wait()

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("claims of %d and %d jobs, want one each", len(first), len(second))
	}
	if first[0].ID == second[0].ID {
		t.Errorf("both workers claimed the same job %s", first[0].ID)
	}
}

// The mechanism behind RT-3, one layer down: a claim is a lease, and a lease that ran out makes
// the job claimable again. Without it, a job whose worker was killed would stay RUNNING forever.
func TestAJobWhoseLeaseRanOutIsClaimedAgain(t *testing.T) {
	ctx := context.Background()
	now := queueClock.Now()
	enqueue(ctx, t, queue.Request{Kind: testKind, DedupeKey: freshID(t).String(), RunAt: now})

	first := claim(ctx, t, now, time.Second, 1)
	if len(first) != 1 {
		t.Fatalf("%d jobs claimed, want one", len(first))
	}
	if first[0].Attempts != 1 {
		t.Errorf("attempts = %d on the first claim, want 1", first[0].Attempts)
	}

	// Still leased: another worker passing by leaves it alone.
	if again := claim(ctx, t, now, time.Minute, 10); containsJob(again, first[0].ID) {
		t.Error("a job was claimed twice while its lease was still running")
	}

	// The lease ends, and with it the claim.
	after := now.Add(2 * time.Second)
	recovered := claim(ctx, t, after, time.Minute, 10)
	if !containsJob(recovered, first[0].ID) {
		t.Fatal("a job whose lease ran out was not picked up again")
	}
	for _, job := range recovered {
		if job.ID == first[0].ID && job.Attempts != 2 {
			t.Errorf("attempts = %d on the second claim, want 2", job.Attempts)
		}
	}
}

// The fence. A worker that fell so far behind that its lease expired must not be able to finish
// the job somebody else is now running - its transaction is refused and rolls back, which is what
// keeps the work from being applied twice.
func TestAWorkerWhoseLeaseExpiredCannotFinishTheJob(t *testing.T) {
	ctx := context.Background()
	now := queueClock.Now()
	enqueue(ctx, t, queue.Request{Kind: testKind, DedupeKey: freshID(t).String(), RunAt: now})

	slow := claim(ctx, t, now, time.Second, 1)[0]
	// Somebody else takes it over after the lease ends.
	taken := claim(ctx, t, now.Add(2*time.Second), time.Minute, 10)
	if !containsJob(taken, slow.ID) {
		t.Fatal("the successor did not get the job")
	}

	err := systemWrite(ctx, t, func(ctx context.Context) error {
		return jobQueue(t).Complete(ctx, slow)
	})
	if err == nil || shared.AsError(err).DetailCode != "queue.lease_lost" {
		t.Fatalf("error %v, want the completion to be refused", err)
	}

	state, attempts, _, _, _ := jobRow(ctx, t, slow.ID)
	if state != "RUNNING" {
		t.Errorf("state = %s after the stale completion, want the successor's RUNNING", state)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want the successor's 2", attempts)
	}
}

// A dead letter is only useful if it says enough to act on: which kind, whose, how often it was
// tried, and the code of the last failure - and never a message, which could carry what the job
// was working on (rule 10).
func TestADeadLetterKeepsTheContextAnOperatorNeeds(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	now := queueClock.Now()
	enqueue(ctx, t, queue.Request{
		Kind: testKind, TenantID: tenantA, DedupeKey: freshID(t).String(), RunAt: now, MaxAttempts: 1,
	})

	job := onlyClaimOf(t, claim(ctx, t, now, time.Minute, 10), testKind)
	if err := systemWrite(ctx, t, func(ctx context.Context) error {
		return jobQueue(t).Fail(ctx, queue.Failure{Job: job, Code: "outbox.subscriber_failed"})
	}); err != nil {
		t.Fatalf("recording the failure: %v", err)
	}

	state, attempts, lastError, tenant, _ := jobRow(ctx, t, job.ID)
	if state != "DEAD_LETTER" {
		t.Errorf("state = %s, want DEAD_LETTER", state)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	if lastError == nil || *lastError != "outbox.subscriber_failed" {
		t.Errorf("last error = %v, want the code that caused it", lastError)
	}
	if tenant == nil || *tenant != tenantA.String() {
		t.Errorf("tenant = %v, want the one the job belongs to", tenant)
	}
}

// A failure with attempts left goes back to the queue at the time the caller worked out, and the
// attempts made are kept: the backoff grows because the count survives.
func TestAFailureWithAttemptsLeftReturnsToTheQueue(t *testing.T) {
	ctx := context.Background()
	now := queueClock.Now()
	enqueue(ctx, t, queue.Request{Kind: testKind, DedupeKey: freshID(t).String(), RunAt: now, MaxAttempts: 5})

	job := onlyClaimOf(t, claim(ctx, t, now, time.Minute, 10), testKind)
	retryAt := now.Add(90 * time.Second)
	if err := systemWrite(ctx, t, func(ctx context.Context) error {
		return jobQueue(t).Fail(ctx, queue.Failure{Job: job, Code: "dependency.unavailable", RetryAt: retryAt})
	}); err != nil {
		t.Fatalf("recording the failure: %v", err)
	}

	state, attempts, _, _, runAt := jobRow(ctx, t, job.ID)
	if state != "PENDING" {
		t.Errorf("state = %s, want PENDING", state)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want the attempt that failed to be remembered", attempts)
	}
	if !runAt.UTC().Equal(retryAt) {
		t.Errorf("next attempt at %v, want %v", runAt.UTC(), retryAt)
	}
}

// The deduplication that makes "make sure this tenant is being dispatched" cost one insert that
// usually does nothing - and that pulls a sleeping job's wake-up forward rather than adding a
// second row beside it.
func TestADedupeKeyCollapsesRepeatedRequestsAndPullsTheWakeUpForward(t *testing.T) {
	ctx := context.Background()
	key := freshID(t).String()
	late, early := queueClock.Now().Add(time.Hour), queueClock.Now().Add(time.Minute)

	enqueue(ctx, t, queue.Request{Kind: testKind, DedupeKey: key, RunAt: late})
	enqueue(ctx, t, queue.Request{Kind: testKind, DedupeKey: key, RunAt: early})

	admin := adminPool(ctx, t)
	var rows int
	var runAt time.Time
	if err := admin.QueryRow(ctx,
		`SELECT count(*), min(run_at) FROM job WHERE kind = $1 AND dedupe_key = $2`,
		string(testKind), key).Scan(&rows, &runAt); err != nil {
		t.Fatalf("counting: %v", err)
	}

	if rows != 1 {
		t.Errorf("%d jobs for one dedupe key, want one", rows)
	}
	if !runAt.UTC().Equal(early) {
		t.Errorf("the wake-up is at %v, want it pulled forward to %v", runAt.UTC(), early)
	}
}

// The queue is the one table without a tenant boundary, and this is the test that keeps that
// deliberate: a worker sees the jobs of every tenant, and each job says whose it is - which is how
// the transaction that runs it gets opened under the right one.
func TestTheQueueCrossesTenantsAndEveryJobSaysWhose(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	now := queueClock.Now()

	enqueue(ctx, t, queue.Request{Kind: testKind, TenantID: tenantA, DedupeKey: freshID(t).String(), RunAt: now})
	enqueue(ctx, t, queue.Request{Kind: testKind, TenantID: tenantB, DedupeKey: freshID(t).String(), RunAt: now})

	claimed := claim(ctx, t, now, time.Minute, 50)

	tenants := map[shared.ID]bool{}
	for _, job := range claimed {
		if job.Kind == testKind {
			tenants[job.TenantID] = true
		}
	}
	if !tenants[tenantA] || !tenants[tenantB] {
		t.Errorf("a worker saw the jobs of %v, want both tenants", tenants)
	}
}

// The write path leaves its own wake-up (ADR-0007). One event or twenty, the tenant ends up with a
// single dispatch job - anything else would be a queue that grows with the traffic it is meant to
// carry.
func TestWritingAnEventLeavesExactlyOneDispatchJobForTheTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	admin := adminPool(ctx, t)
	var before int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM job WHERE kind = $1 AND tenant_id = $2 AND state IN ('PENDING','RUNNING')`,
		queue.KindOutboxDispatch.String(), tenantA.String()).Scan(&before); err != nil {
		t.Fatalf("counting: %v", err)
	}

	for range 3 {
		container := containerIn(tenantA, authorA, freshID(t), freshName(t), "a0")
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return postgres.NewOutbox(jobQueue(t)).Append(ctx, announcement(t, tenantA, container))
		}); err != nil {
			t.Fatalf("writing the event: %v", err)
		}
	}

	var after int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM job WHERE kind = $1 AND tenant_id = $2 AND state IN ('PENDING','RUNNING')`,
		queue.KindOutboxDispatch.String(), tenantA.String()).Scan(&after); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if after > 1 || (before == 0 && after != 1) {
		t.Errorf("%d dispatch jobs for the tenant (was %d), want exactly one", after, before)
	}
}

func containsJob(jobs []queue.Job, id shared.ID) bool {
	for _, job := range jobs {
		if job.ID == id {
			return true
		}
	}
	return false
}

// onlyClaimOf picks this suite's job out of a batch that may also carry the dispatch jobs the
// other tests in this package leave behind.
func onlyClaimOf(t *testing.T, jobs []queue.Job, kind queue.Kind) queue.Job {
	t.Helper()
	var found []queue.Job
	for _, job := range jobs {
		if job.Kind == kind {
			found = append(found, job)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d jobs of kind %s in the batch, want one", len(found), kind)
	}
	return found[0]
}

// The fairness of H-08 (multi-tenancy.md §4): two tenants due at once share the batch - the
// claim takes everybody's first job before anybody's second, so a storm from one workspace
// cannot monopolise the workers and both make progress.
func TestAFloodingTenantDoesNotMonopoliseTheClaim(t *testing.T) {
	ctx := context.Background()

	flooder := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fa01")
	modest := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fa02")
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name)
		VALUES ($1, 'flooder', 'Flooder'), ($2, 'modest', 'Modest')
		ON CONFLICT (id) DO NOTHING`, flooder.String(), modest.String()); err != nil {
		t.Fatalf("seeding tenants: %v", err)
	}

	moment := time.Now().UTC().Add(-time.Minute)
	// The storm first, so age alone would hand it the whole batch.
	for i := 0; i < 20; i++ {
		enqueue(ctx, t, queue.Request{
			Kind: queue.Kind("test.fairness"), TenantID: flooder,
			DedupeKey: "flood-" + strconv.Itoa(i), RunAt: moment,
		})
	}
	for i := 0; i < 3; i++ {
		enqueue(ctx, t, queue.Request{
			Kind: queue.Kind("test.fairness"), TenantID: modest,
			DedupeKey: "modest-" + strconv.Itoa(i), RunAt: moment.Add(time.Second),
		})
	}

	claimed := claim(ctx, t, time.Now().UTC(), time.Minute, 10)
	if len(claimed) != 10 {
		t.Fatalf("claimed %d, want the full batch", len(claimed))
	}
	counts := map[shared.ID]int{}
	for _, job := range claimed {
		counts[job.TenantID]++
	}
	// Round-robin: both make progress and neither claims the whole pool. The assertions are
	// deliberately about the property rather than exact shares - the suite's other tests leave
	// their own tenants' due jobs behind, and those share the batch too (each with their own
	// first-place rows), which is the fairness working, not the test flaking. What must hold
	// whatever the crowd: the flooder's twenty-job head start buys it no more than one round's
	// head start over the modest tenant, and both are in the batch.
	if counts[modest] < 1 {
		t.Errorf("the modest tenant got nothing (flooder %d) - age alone decided", counts[flooder])
	}
	if counts[flooder] < 1 {
		t.Error("the flooding tenant starved - fairness is sharing, not punishment")
	}
	if counts[flooder] == len(claimed) {
		t.Error("the flooding tenant claimed the whole pool")
	}
	// The flooder may pull ahead only once the modest tenant has nothing left to claim - its
	// whole three are in the batch. Anything else means the storm bought priority.
	if counts[flooder] > counts[modest]+1 && counts[modest] != 3 {
		t.Errorf("the flooder took %d while the modest tenant still had work (%d of 3 claimed)",
			counts[flooder], counts[modest])
	}
}
