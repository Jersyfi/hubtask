// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/idempotency"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// IdempotencyStore keeps the answers of completed attempts in idempotency_key.
//
// No method names a tenant. The reservation takes its tenant from current_tenant_id(), which is
// the value row level security compares against, so a row cannot be written into the wrong tenant
// even by a caller that wanted to (ADR-0010).
type IdempotencyStore struct{}

func NewIdempotencyStore() IdempotencyStore { return IdempotencyStore{} }

var _ repository.Store = IdempotencyStore{}

// Reserve claims the key, and reads the existing record only when the claim was lost. The common
// path - a key nobody has used - is one round trip.
func (s IdempotencyStore) Reserve(
	ctx context.Context,
	key repository.Key,
	requestHash []byte,
) (repository.Record, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Record{}, false, err
	}

	claimed, err := queries.ReserveIdempotencyKey(ctx, sqlc.ReserveIdempotencyKeyParams{
		Key:         key.Key,
		Endpoint:    key.Endpoint,
		RequestHash: requestHash,
	})
	if err != nil {
		return repository.Record{}, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reserving the idempotency key: %w", err))
	}
	if claimed == 1 {
		return repository.Record{}, true, nil
	}

	row, err := queries.FindIdempotencyRecord(ctx, sqlc.FindIdempotencyRecordParams{
		Key:      key.Key,
		Endpoint: key.Endpoint,
	})
	if err != nil {
		if IsNoRows(err) {
			// The row was claimed by another tenant. Invisible here, and reported as a conflict
			// rather than as a missing record: from this tenant's side the key is simply not
			// usable, and saying more would confirm its existence elsewhere.
			return repository.Record{}, false, shared.ErrConflict.WithDetail("idempotency.key_reused")
		}
		return repository.Record{}, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the idempotency record: %w", err))
	}

	record := repository.Record{RequestHash: row.RequestHash, Body: row.ResponseBody}
	if row.ResponseCode != nil {
		record.Status = int(*row.ResponseCode)
	}
	return record, false, nil
}

// Complete stores the answer. A body that is not JSON is stored as no body at all: the column is
// jsonb, and this API answers JSON - anything else is a bug worth not compounding by failing the
// request that produced it.
func (s IdempotencyStore) Complete(ctx context.Context, key repository.Key, status int, body []byte) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	code := int32(status) //nolint:gosec // G115: an HTTP status is three digits by definition
	params := sqlc.CompleteIdempotencyRecordParams{
		Key:          key.Key,
		Endpoint:     key.Endpoint,
		ResponseCode: &code,
	}
	if len(body) > 0 && json.Valid(body) {
		params.ResponseBody = body
	}

	if err := queries.CompleteIdempotencyRecord(ctx, params); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("storing the idempotent answer: %w", err))
	}
	return nil
}

// Release lets a reservation go. The engine calls it for an action that claimed its key
// and then failed, in the same transaction: the claim and the failure commit together, so a
// replay of the run finds the key free and performs the action the first run never did.
func (s IdempotencyStore) Release(ctx context.Context, key repository.Key) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if err := queries.ReleaseIdempotencyKey(ctx, sqlc.ReleaseIdempotencyKeyParams{
		Key: key.Key, Endpoint: key.Endpoint,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("releasing the idempotency key: %w", err))
	}
	return nil
}

// DeleteExpired removes one batch of keys older than the cutoff (the IDEMPOTENCY_KEY data kind,
// data-retention.md §3). The tenant is the transaction's, through row level security.
func (s IdempotencyStore) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	removed, err := queries.DeleteExpiredIdempotencyKeys(ctx, sqlc.DeleteExpiredIdempotencyKeysParams{
		Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true},
		Batch:  int32(batch), //nolint:gosec // G115: the batch size is a small configuration value
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("sweeping the idempotency keys: %w", err))
	}
	return int(removed), nil
}

// CountExpired answers how many keys are due, up to the ceiling.
func (s IdempotencyStore) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	due, err := queries.CountExpiredIdempotencyKeys(ctx, sqlc.CountExpiredIdempotencyKeysParams{
		Cutoff:  pgtype.Timestamptz{Time: cutoff, Valid: true},
		Ceiling: int32(ceiling), //nolint:gosec // G115: the ceiling is a batch size plus one
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the idempotency keys: %w", err))
	}
	return int(due), nil
}
