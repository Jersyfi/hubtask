// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"

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
