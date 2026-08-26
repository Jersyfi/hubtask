// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// Queue is the job table behind the queue port (ADR-0008).
//
// It is the one repository that reads across tenants, because it has to: a worker claims the next
// job before it can know whose it is. That is why the table carries no row level security and why
// every statement here states its own conditions - see db/queries/Job.sql. The tenant boundary is
// not weakened by it: the job row names a tenant, and the transaction that runs the job is opened
// under that tenant, so the work itself is as bounded as any request.
type Queue struct {
	ids clock.IDGenerator
	now clock.Clock
}

func NewQueue(ids clock.IDGenerator, now clock.Clock) Queue { return Queue{ids: ids, now: now} }

var (
	_ queue.Queue    = Queue{}
	_ queue.Reporter = Queue{}
)

// maxBatch bounds a claim. Not a tuning limit but a safety one: the value reaches the driver as an
// int32, and a batch of four digits is a typo rather than a plan - it would also hold every row in
// it locked for the length of one worker's round.
const maxBatch = 1000

// maxJobAttempts bounds the retry budget. A job that has failed a hundred times is not going to
// work on the hundred and first; past that the queue is being used as a place to hide an outage.
const maxJobAttempts = 100

// Enqueue adds a job, or leaves the one that is already scheduled alone.
func (q Queue) Enqueue(ctx context.Context, request queue.Request) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	if request.Kind == "" {
		return "", shared.ErrInternal.WithDetail("queue.kind_missing")
	}

	id, err := uuidOf(q.ids.NewID())
	if err != nil {
		return "", err
	}
	tenantID, err := optionalUUID(request.TenantID)
	if err != nil {
		return "", err
	}

	// The payload is serialised here rather than by the caller: JSON is a wire format, and a
	// handler receives a map either way. An absent payload is an empty object, never SQL NULL -
	// the column is NOT NULL, and "null" would be a value a handler has to special-case.
	payload := request.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", shared.ErrInternal.
			WithDetail("queue.payload_unserialisable").
			WithCause(fmt.Errorf("serialising the payload of %s: %w", request.Kind, err))
	}

	runAt := request.RunAt
	if runAt.IsZero() {
		runAt = q.now.Now()
	}

	scheduled, err := queries.EnqueueJob(ctx, sqlc.EnqueueJobParams{
		ID:          id,
		TenantID:    tenantID,
		Kind:        request.Kind.String(),
		Payload:     encoded,
		DedupeKey:   optionalText(request.DedupeKey),
		RunAt:       timestampOf(runAt),
		MaxAttempts: boundedAttempts(request.MaxAttempts),
	})
	if err != nil {
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("enqueueing a %s job: %w", request.Kind, err))
	}
	if len(scheduled) == 1 {
		return idFrom(scheduled[0])
	}

	// Nothing came back: the dedupe key met a job that is RUNNING, where the update's condition
	// does not fire. The work the caller asked for is happening, and the job it is happening under
	// is the one to answer with.
	return q.jobNamedBy(ctx, queries, request)
}

// jobNamedBy answers the job a dedupe key already names.
func (q Queue) jobNamedBy(
	ctx context.Context, queries *sqlc.Queries, request queue.Request,
) (shared.ID, error) {
	if request.DedupeKey == "" {
		// Without a key there is nothing to have collided with, so an insert that wrote nothing is
		// a defect here rather than a state to recover from.
		return "", shared.Internalf("queue: a %s job was neither written nor named", request.Kind)
	}
	existing, err := queries.FindJobByDedupeKey(ctx, sqlc.FindJobByDedupeKeyParams{
		Kind: request.Kind.String(), DedupeKey: request.DedupeKey,
	})
	if err != nil {
		if IsNoRows(err) {
			// It finished between the two statements. The caller asked for work to happen and it
			// has; there is no job left to point at, and inventing one would be worse.
			return "", nil
		}
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the %s job already scheduled: %w", request.Kind, err))
	}
	return idFrom(existing)
}

// Claim takes the next batch and holds it until the lease expires.
func (q Queue) Claim(ctx context.Context, lease queue.Lease) ([]queue.Job, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	if !lease.Until.After(lease.Now) {
		// A lease that has already expired would be claimed again by the next worker while this
		// one is still working - the exact double execution the lease exists to prevent.
		return nil, shared.ErrInternal.WithDetail("queue.lease_not_in_the_future")
	}

	rows, err := queries.ClaimJobs(ctx, sqlc.ClaimJobsParams{
		LockedUntil: timestampOf(lease.Until),
		Now:         timestampOf(lease.Now),
		BatchSize:   boundedBatch(lease.Batch),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("claiming jobs: %w", err))
	}

	jobs := make([]queue.Job, 0, len(rows))
	for _, row := range rows {
		job, err := jobFrom(row)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// Hold takes the row lock on the job for the rest of the caller's transaction (queue.Queue.Hold).
//
// A missing row is the fence answering: the lease expired and somebody else has the job, so this
// pass has nothing to hold and nothing to write - the same answer Complete gives, so the caller
// rolls back rather than working on.
func (q Queue) Hold(ctx context.Context, job queue.Job) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, lease, err := fence(job)
	if err != nil {
		return err
	}

	if _, err := queries.HoldJob(ctx, sqlc.HoldJobParams{ID: id, Lease: lease}); err != nil {
		if IsNoRows(err) {
			return leaseHeld(0, job)
		}
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("holding job %s: %w", job.ID, err))
	}
	return nil
}

// Complete finishes a job for good, in the transaction that carries its effect.
func (q Queue) Complete(ctx context.Context, job queue.Job) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, lease, err := fence(job)
	if err != nil {
		return err
	}

	affected, err := queries.CompleteJob(ctx, sqlc.CompleteJobParams{
		Now:   timestampOf(q.now.Now()),
		ID:    id,
		Lease: lease,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("completing job %s: %w", job.ID, err))
	}
	return leaseHeld(affected, job)
}

// Report writes how far along a long job is (E-05).
//
// Fenced on the lease like every statement a handler runs, and silent when the fence does not hold:
// a worker that lost its job writes nothing, and that is not a failure to hand back. The number is
// for whoever is watching, and the clamp is in the statement rather than here because a fraction
// outside [0,1] on a progress bar is a rendering bug in every client at once.
func (q Queue) Report(ctx context.Context, job queue.Job, fraction float64) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, lease, err := fence(job)
	if err != nil {
		return err
	}

	err = queries.SetJobProgress(ctx, sqlc.SetJobProgressParams{
		Progress: float32(fraction), ID: id, Lease: lease,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the progress of job %s: %w", job.ID, err))
	}
	return nil
}

// Repeat sends a poller round back to the queue for its next one.
func (q Queue) Repeat(ctx context.Context, job queue.Job, runAt time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, lease, err := fence(job)
	if err != nil {
		return err
	}

	affected, err := queries.RepeatJob(ctx, sqlc.RepeatJobParams{
		RunAt: timestampOf(runAt),
		ID:    id,
		Lease: lease,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("rescheduling job %s: %w", job.ID, err))
	}
	return leaseHeld(affected, job)
}

// Fail records an attempt that did not work: back to the queue, or to the dead letter.
//
// It runs in a transaction of its own, because the handler's has just been rolled back - which is
// also why the failure has to be written from a code path that shares nothing with the failure
// itself.
func (q Queue) Fail(ctx context.Context, failure queue.Failure) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, lease, err := fence(failure.Job)
	if err != nil {
		return err
	}
	if failure.Code == "" {
		// An empty reason on a dead letter is an operator staring at a row with no idea what to
		// do with it. The caller knows why; there is no useful default here.
		return shared.ErrInternal.WithDetail("queue.failure_without_reason")
	}
	code := failure.Code

	var affected int64
	if failure.RetryAt.IsZero() {
		affected, err = queries.DeadLetterJob(ctx, sqlc.DeadLetterJobParams{
			Now:       timestampOf(q.now.Now()),
			LastError: &code,
			ID:        id,
			Lease:     lease,
		})
	} else {
		affected, err = queries.RetryJob(ctx, sqlc.RetryJobParams{
			RunAt:     timestampOf(failure.RetryAt),
			LastError: &code,
			ID:        id,
			Lease:     lease,
		})
	}
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the failure of job %s: %w", failure.Job.ID, err))
	}
	return leaseHeld(affected, failure.Job)
}

// Depth reports the backlog per kind across every tenant.
func (q Queue) Depth(ctx context.Context) ([]queue.Depth, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.JobQueueDepth(ctx, timestampOf(q.now.Now()))
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the queue: %w", err))
	}

	depths := make([]queue.Depth, 0, len(rows))
	for _, row := range rows {
		depths = append(depths, queue.Depth{Kind: queue.Kind(row.Kind), Pending: int(row.Pending)})
	}
	return depths, nil
}

// jobFrom rebuilds a claimed job. A row that cannot be read is a defect rather than a bad job:
// everything in it was written by this adapter.
func jobFrom(row sqlc.ClaimJobsRow) (queue.Job, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return queue.Job{}, err
	}
	tenantID, err := optionalID(row.TenantID)
	if err != nil {
		return queue.Job{}, err
	}

	payload := map[string]any{}
	if len(row.Payload) > 0 {
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return queue.Job{}, shared.ErrInternal.
				WithDetail("queue.payload_unreadable").
				WithCause(fmt.Errorf("reading the payload of job %s: %w", id, err))
		}
	}

	return queue.Job{
		ID:          id,
		TenantID:    tenantID,
		Kind:        queue.Kind(row.Kind),
		Payload:     payload,
		Attempts:    int(row.Attempts),
		MaxAttempts: int(row.MaxAttempts),
		Lease:       timeFrom(row.LockedUntil),
	}, nil
}

// fence turns a job into the two values every finishing statement matches on. A job without a
// lease was not claimed through Claim, and finishing it would be finishing somebody else's work.
func fence(job queue.Job) (id pgtype.UUID, lease pgtype.Timestamptz, err error) {
	if job.Lease.IsZero() {
		return id, lease, shared.ErrInternal.WithDetail("queue.job_without_lease")
	}
	id, err = uuidOf(job.ID)
	if err != nil {
		return id, lease, err
	}
	return id, timestampOf(job.Lease), nil
}

// leaseHeld turns "the update matched nothing" into the error it is: while this worker was
// working, its lease ran out and another worker took the job over. The caller's transaction then
// rolls back, which is what stops the work from being applied twice (test RT-3).
func leaseHeld(affected int64, job queue.Job) error {
	if affected == 0 {
		return shared.ErrConflict.
			WithDetail("queue.lease_lost").
			WithParams(map[string]string{"job": job.ID.String(), "kind": job.Kind.String()})
	}
	return nil
}

// boundedBatch narrows a batch size to int32 within provable bounds. Both comparisons are against
// constants and both return early, for the same reason as boundedPoolSize in Pool.go.
func boundedBatch(value int) int32 {
	if value < 1 {
		return 1
	}
	if value > maxBatch {
		return maxBatch
	}
	return int32(value)
}

// boundedAttempts narrows the retry budget. Zero means "whatever the column says", which is how a
// caller that has no opinion gets the schema's default of eight.
func boundedAttempts(value int) int32 {
	if value < 1 {
		return defaultJobAttempts
	}
	if value > maxJobAttempts {
		return maxJobAttempts
	}
	return int32(value)
}

// defaultJobAttempts mirrors the column default of job.max_attempts. It is repeated here because
// the insert names the column - passing NULL to a NOT NULL column with a default is not a way to
// ask for the default.
const defaultJobAttempts = 8
