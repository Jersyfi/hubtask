// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// DeadLetterRepository reads and requeues the job table's dead letters for the installation's
// operator. The table has no row-level policy (the job table's precedent); the operator's door is
// the only reader, under the installation's scope.
type DeadLetterRepository struct{}

// NewDeadLetterRepository builds the repository.
func NewDeadLetterRepository() DeadLetterRepository { return DeadLetterRepository{} }

var _ repository.DeadLetters = DeadLetterRepository{}

func deadLetterFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("%s a dead letter: %w", what, err))
}

// Page answers them newest first.
func (DeadLetterRepository) Page(ctx context.Context, kind string, before shared.ID, size int) ([]repository.DeadLetter, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	params := sqlc.PageDeadLettersParams{Size: int32(size)} //nolint:gosec // bounded by the use case
	if kind != "" {
		params.Kind = &kind
	}
	if !before.IsZero() {
		if params.Before, err = uuidOf(before); err != nil {
			return nil, err
		}
	}
	rows, err := queries.PageDeadLetters(ctx, params)
	if err != nil {
		return nil, deadLetterFailed("listing", err)
	}
	letters := make([]repository.DeadLetter, 0, len(rows))
	for _, row := range rows {
		letter, err := deadLetterOf(row.ID, row.TenantID, row.Kind, row.Attempts, row.MaxAttempts, row.LastError, row.CreatedAt)
		if err != nil {
			return nil, err
		}
		letter.FinishedAt = timeFrom(row.FinishedAt)
		letters = append(letters, letter)
	}
	return letters, nil
}

// Requeue returns one to the queue.
func (DeadLetterRepository) Requeue(ctx context.Context, id shared.ID, now time.Time) (repository.DeadLetter, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.DeadLetter{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return repository.DeadLetter{}, shared.ErrNotFound.WithDetail("admin.dead_letter_not_found")
	}
	row, err := queries.RequeueDeadLetter(ctx, sqlc.RequeueDeadLetterParams{ID: key, Now: momentOf(now)})
	if err != nil {
		if !IsNoRows(err) {
			return repository.DeadLetter{}, deadLetterFailed("requeuing", err)
		}
		dead, err := queries.IsDeadLetter(ctx, key)
		if err != nil {
			return repository.DeadLetter{}, deadLetterFailed("reading", err)
		}
		if dead {
			return repository.DeadLetter{}, shared.ErrConflict.WithDetail("admin.job_already_open")
		}
		return repository.DeadLetter{}, shared.ErrNotFound.WithDetail("admin.dead_letter_not_found")
	}
	letter, err := deadLetterOf(row.ID, row.TenantID, row.Kind, row.Attempts, row.MaxAttempts, row.LastError, row.CreatedAt)
	letter.Requeued = true
	return letter, err
}

func deadLetterOf(
	id, tenant pgtype.UUID, kind string, attempts, maxAttempts int32, lastError *string, created pgtype.Timestamptz,
) (repository.DeadLetter, error) {
	letterID, err := idFrom(id)
	if err != nil {
		return repository.DeadLetter{}, err
	}
	tenantID, err := optionalID(tenant)
	if err != nil {
		return repository.DeadLetter{}, err
	}
	letter := repository.DeadLetter{
		ID: letterID, TenantID: tenantID, Kind: kind, Attempts: int(attempts), MaxAttempts: int(maxAttempts),
		CreatedAt: timeFrom(created),
	}
	if lastError != nil {
		letter.ErrorCode = *lastError
	}
	return letter, nil
}
