// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/job"
	domain "github.com/Jersyfi/hubtask/core/domain/model/job"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// JobRepository is the caller's side of the job table (E-01).
//
// It reads the same rows as Queue and shares none of its statements, because the two ask
// different questions: the queue claims work across tenants and this one answers for exactly one.
// The tenant is taken from the transaction the unit of work opened, never from an argument -
// `job` is the one table without row level security (db/migrations/0001_init.sql), so the
// condition that a policy applies everywhere else is applied here, in one place, and a method that
// forgot it would be reading across the boundary.
type JobRepository struct{}

func NewJobRepository() JobRepository { return JobRepository{} }

var _ repository.Jobs = JobRepository{}

// Find answers the job the caller's tenant owns.
func (r JobRepository) Find(ctx context.Context, id shared.ID) (domain.Job, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Job{}, err
	}
	jobID, err := uuidOf(id)
	if err != nil {
		return domain.Job{}, err
	}
	tenantID, err := tenantOfTransaction(ctx)
	if err != nil {
		return domain.Job{}, err
	}

	row, err := queries.FindJob(ctx, sqlc.FindJobParams{ID: jobID, TenantID: tenantID})
	if err != nil {
		if IsNoRows(err) {
			return domain.Job{}, notFoundJob(id)
		}
		return domain.Job{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading job %s: %w", id, err))
	}
	job, err := jobRowOf(row.ID, row.TenantID, row.State, row.LastError, row.CreatedAt, row.FinishedAt)
	if err != nil {
		return domain.Job{}, err
	}
	// Nullable on the way through, because null is the answer E-01 documented for a job that
	// cannot compute one - and most still cannot.
	if row.Progress != nil {
		fraction := float64(*row.Progress)
		job.Progress = &fraction
	}
	return job, nil
}

// Cancel stops a queued or running job, in one statement that carries both conditions.
//
// The state condition is in the statement rather than only in the caller's check, because the
// caller's check ran a moment earlier: a job that succeeded in between must not read back as
// cancelled. No lease is named - the caller holds none, and a pass that is under way finds at its
// next write that the row is no longer RUNNING under its lease and rolls back.
func (r JobRepository) Cancel(
	ctx context.Context, id shared.ID, now time.Time,
) (domain.Job, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Job{}, err
	}
	jobID, err := uuidOf(id)
	if err != nil {
		return domain.Job{}, err
	}
	tenantID, err := tenantOfTransaction(ctx)
	if err != nil {
		return domain.Job{}, err
	}

	row, err := queries.CancelJob(ctx, sqlc.CancelJobParams{
		Now: timestampOf(now), ID: jobID, TenantID: tenantID,
	})
	if err != nil {
		if IsNoRows(err) {
			// Nothing was updated, and the two reasons are told apart by asking again: the job is
			// in another tenant or gone, or it reached a terminal state between the read and this
			// write. Find answers the first as a 404 and the second as the job it now is, which
			// the application layer turns into the conflict.
			return domain.Job{}, r.raced(ctx, id)
		}
		return domain.Job{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("cancelling job %s: %w", id, err))
	}
	return jobRowOf(row.ID, row.TenantID, row.State, row.LastError, row.CreatedAt, row.FinishedAt)
}

// raced turns an update that matched nothing into the reason it matched nothing.
func (r JobRepository) raced(ctx context.Context, id shared.ID) error {
	job, err := r.Find(ctx, id)
	if err != nil {
		return err
	}
	if err := job.Cancellable(); err != nil {
		return err
	}
	// Cancellable and yet not cancelled: the row changed under a condition this code cannot
	// name, and reporting success would be a guess.
	return shared.ErrConflict.
		WithDetail("jobs.cancellation_raced").
		WithParams(map[string]string{"job_id": id.String()})
}

// tenantOfTransaction is the condition the missing row level security policy would have applied.
//
// It reads the scope this package put on the context when it opened the transaction, so it cannot
// be supplied by a caller - the same value that became `SET LOCAL app.tenant_id` for every other
// table. A repository reached outside a unit of work gets an error rather than an unbounded query.
func tenantOfTransaction(ctx context.Context) (pgtype.UUID, error) {
	scope, ok := scopeFromContext(ctx)
	if !ok || scope.TenantID.IsZero() {
		return pgtype.UUID{}, shared.ErrInternal.WithDetail("postgres.no_tenant_in_context")
	}
	return uuidOf(scope.TenantID)
}

// notFoundJob is the one answer for a job in another tenant, a job belonging to no tenant, and a
// job that never existed.
func notFoundJob(id shared.ID) error {
	return shared.ErrNotFound.
		WithDetail("jobs.not_found").
		WithParams(map[string]string{"job_id": id.String()})
}

// jobRowOf maps a row to the resource. The two row types are structurally identical and sqlc
// generates one per statement, so the columns are passed rather than the struct.
func jobRowOf(
	id, tenantID pgtype.UUID, state string, lastError *string,
	createdAt, finishedAt pgtype.Timestamptz,
) (domain.Job, error) {
	jobID, err := idFrom(id)
	if err != nil {
		return domain.Job{}, err
	}
	tenant, err := idFrom(tenantID)
	if err != nil {
		return domain.Job{}, err
	}
	translated, err := domain.StateOf(state)
	if err != nil {
		return domain.Job{}, err
	}

	job := domain.Job{
		ID: jobID, TenantID: tenant, State: translated, CreatedAt: createdAt.Time,
	}
	if lastError != nil {
		job.ErrorCode = *lastError
	}
	if finishedAt.Valid {
		job.FinishedAt = finishedAt.Time
	}
	return job, nil
}
