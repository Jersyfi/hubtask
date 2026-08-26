// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/job"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The caller's side of the job table (E-01), against a real database. The `job` table is the one
// table without row level security, so every one of these methods states the tenant condition
// itself - which is exactly why the boundary is proved per method here rather than trusted to a
// policy that is not there (gate SG-3).

// A kind of this file's own, so that nothing here collides with the dispatch jobs the write path
// creates for itself.
const pollableKind = queue.Kind("test.pollable")

func jobRepo() postgres.JobRepository { return postgres.NewJobRepository() }

// enqueueFor puts one job in for the tenant and answers the identifier the queue gave it. The
// identifier is read back through the deduplication key rather than returned by Enqueue: a caller
// that needed it from the port would be a caller reaching around the claim, and this file is the
// one place that has to name a row.
func enqueueFor(ctx context.Context, t *testing.T, tenant shared.ID, dedupe string) shared.ID {
	t.Helper()

	enqueue(ctx, t, queue.Request{
		Kind: pollableKind, TenantID: tenant, DedupeKey: dedupe, RunAt: queueClock.Now(),
	})

	var id string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT id::text FROM job WHERE kind = $1 AND dedupe_key = $2`,
		pollableKind.String(), dedupe).Scan(&id); err != nil {
		t.Fatalf("reading back the job %q: %v", dedupe, err)
	}
	return shared.MustParseID(id)
}

func findJob(ctx context.Context, t *testing.T, tenant, id shared.ID) (domain.Job, error) {
	t.Helper()

	var job domain.Job
	err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		job, err = jobRepo().Find(ctx, id)
		return err
	})
	return job, err
}

func cancelJob(ctx context.Context, t *testing.T, tenant, id shared.ID) (domain.Job, error) {
	t.Helper()

	var job domain.Job
	err := write(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		job, err = jobRepo().Cancel(ctx, id, queueClock.Now().Add(time.Minute))
		return err
	})
	return job, err
}

func TestAQueuedJobIsReadBackByTheTenantThatAskedForIt(t *testing.T) {
	ctx := context.Background()
	id := enqueueFor(ctx, t, tenantA, freshName(t))

	job, err := findJob(ctx, t, tenantA, id)
	if err != nil {
		t.Fatalf("reading the job: %v", err)
	}

	switch {
	case job.ID != id:
		t.Fatalf("read back job %s, want %s", job.ID, id)
	case job.TenantID != tenantA:
		t.Fatalf("the job named tenant %s", job.TenantID)
	case job.State != domain.StateQueued:
		t.Fatalf("state %q, want QUEUED - PENDING is the queue's word for it", job.State)
	case job.CreatedAt.IsZero():
		t.Fatal("the job carries no creation time")
	case !job.FinishedAt.IsZero():
		t.Fatal("a queued job carries a finishing time")
	case job.ErrorCode != "":
		t.Fatalf("a queued job carries the error code %q", job.ErrorCode)
	case job.Progress != nil:
		t.Fatal("progress is a number nobody computed rather than null")
	}
}

// The cross-tenant negative for Find. There is no policy underneath this statement, so this test
// is the whole of the proof.
func TestAJobOfAnotherTenantIsIndistinguishableFromOneThatNeverExisted(t *testing.T) {
	ctx := context.Background()
	id := enqueueFor(ctx, t, tenantA, freshName(t))

	_, err := findJob(ctx, t, tenantB, id)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("tenant B read tenant A's job: %v", err)
	}
	if code := shared.AsError(err).DetailCode; code != "jobs.not_found" {
		t.Fatalf("detail code %q, want jobs.not_found", code)
	}

	// The same answer for one that never existed, down to the code: a caller that could tell the
	// two apart could enumerate what other tenants are running.
	_, missing := findJob(ctx, t, tenantB, freshID(t))
	if shared.AsError(missing).DetailCode != shared.AsError(err).DetailCode {
		t.Fatal("a foreign job and an absent job answer differently")
	}
}

// The queue's own housekeeping belongs to no tenant, and instance administration has no surface
// on this API. Until it does, such a job is answered to nobody here.
func TestAJobBelongingToNoTenantIsNotAnsweredToAnyTenant(t *testing.T) {
	ctx := context.Background()
	id := enqueueFor(ctx, t, shared.ID(""), freshName(t))

	if _, err := findJob(ctx, t, tenantA, id); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a tenant read the instance's job: %v", err)
	}
	if _, err := cancelJob(ctx, t, tenantA, id); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a tenant cancelled the instance's job: %v", err)
	}
}

func TestCancellingAQueuedJobStopsItAndClearsItsLease(t *testing.T) {
	ctx := context.Background()
	id := enqueueFor(ctx, t, tenantA, freshName(t))

	job, err := cancelJob(ctx, t, tenantA, id)
	if err != nil {
		t.Fatalf("cancelling: %v", err)
	}
	if job.State != domain.StateCancelled {
		t.Fatalf("state %q after cancelling", job.State)
	}
	if job.FinishedAt.IsZero() {
		t.Fatal("a cancelled job carries no finishing time")
	}

	// Past every adapter: the row itself, and the lease it no longer holds.
	var state string
	var lockedUntil *time.Time
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT state, locked_until FROM job WHERE id = $1`, id.String()).
		Scan(&state, &lockedUntil); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if state != domain.StoredCancelled {
		t.Fatalf("the row says %q", state)
	}
	if lockedUntil != nil {
		t.Fatal("the cancelled job still holds a lease, so nothing may claim it until it expires")
	}
}

// The cross-tenant negative for Cancel. A separate test from the read's, because they are separate
// statements and each carries its own condition.
func TestAnotherTenantCannotCancelAJob(t *testing.T) {
	ctx := context.Background()
	id := enqueueFor(ctx, t, tenantA, freshName(t))

	if _, err := cancelJob(ctx, t, tenantB, id); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("tenant B cancelled tenant A's job: %v", err)
	}

	job, err := findJob(ctx, t, tenantA, id)
	if err != nil {
		t.Fatalf("reading the job back: %v", err)
	}
	if job.State != domain.StateQueued {
		t.Fatalf("the refused cancellation left the job in %q", job.State)
	}
}

func TestCancellingATerminalJobIsAConflict(t *testing.T) {
	ctx := context.Background()
	id := enqueueFor(ctx, t, tenantA, freshName(t))

	if _, err := cancelJob(ctx, t, tenantA, id); err != nil {
		t.Fatalf("the first cancellation: %v", err)
	}

	_, err := cancelJob(ctx, t, tenantA, id)
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("cancelling a cancelled job: %v, want a conflict", err)
	}
	if code := shared.AsError(err).DetailCode; code != "jobs.already_finished" {
		t.Fatalf("detail code %q, want jobs.already_finished", code)
	}
}

// What "cooperative" means once it is a row: cancelling does not interrupt a pass that is under
// way, and the pass discovers at its next write that the job is no longer its own. The fence is
// the same one a lease that expired uses (test RT-3) - the work is discarded rather than applied.
func TestAPassUnderWayFindsItsJobTakenFromItAndWritesNothing(t *testing.T) {
	ctx := context.Background()
	id := enqueueFor(ctx, t, tenantA, freshName(t))

	claimed := claim(ctx, t, queueClock.Now(), time.Minute, 20)
	var mine queue.Job
	for _, job := range claimed {
		if job.ID == id {
			mine = job
		}
	}
	if mine.ID.IsZero() {
		t.Fatal("the job was not claimed, so there is no pass to take it from")
	}

	if _, err := cancelJob(ctx, t, tenantA, id); err != nil {
		t.Fatalf("cancelling the running job: %v", err)
	}

	err := systemWrite(ctx, t, func(ctx context.Context) error {
		return jobQueue(t).Complete(ctx, mine)
	})
	if err == nil {
		t.Fatal("the pass completed a job that had been cancelled under it")
	}

	var state string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT state FROM job WHERE id = $1`, id.String()).Scan(&state); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if state != domain.StoredCancelled {
		t.Fatalf("the row says %q, so the pass overwrote the cancellation", state)
	}
}
