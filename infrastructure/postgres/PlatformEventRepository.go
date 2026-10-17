// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PlatformEventRepository keeps the installation's events (repository.PlatformEvents).
type PlatformEventRepository struct{}

// NewPlatformEventRepository builds the repository.
func NewPlatformEventRepository() PlatformEventRepository { return PlatformEventRepository{} }

var _ repository.PlatformEvents = PlatformEventRepository{}

// Append numbers the event under the installation's lock and writes it.
func (PlatformEventRepository) Append(ctx context.Context, envelope event.Envelope, dedupeKey string) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(envelope.ID)
	if err != nil {
		return false, err
	}
	workspace, err := uuidOf(envelope.TenantID)
	if err != nil {
		return false, err
	}
	actor, err := optionalUUID(envelope.Actor.ID)
	if err != nil {
		return false, err
	}
	payload, err := json.Marshal(envelope.Payload)
	if err != nil {
		return false, shared.ErrInternal.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("encoding an installation event: %w", err))
	}
	params := sqlc.InsertPlatformEventParams{
		ID: id, EventType: string(envelope.Type), TenantID: workspace,
		OccurredAt: pgtype.Timestamptz{Time: envelope.OccurredAt.UTC(), Valid: true},
		ActorType:  string(envelope.Actor.Kind), ActorID: actor, Payload: payload,
	}
	if dedupeKey != "" {
		params.DedupeKey = &dedupeKey
	}

	if err := queries.LockPlatformEventNumbering(ctx); err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("taking the installation events' numbering lock: %w", err))
	}
	written, err := queries.InsertPlatformEvent(ctx, params)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing an installation event: %w", err))
	}
	return written == 1, nil
}

// After reads the events numbered after `after`.
func (PlatformEventRepository) After(ctx context.Context, after int64, limit int) ([]repository.PlatformEvent, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PlatformEventsAfter(ctx, sqlc.PlatformEventsAfterParams{After: after, PageSize: int32(limit)}) //nolint:gosec // bounded by the use case
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the installation's events: %w", err))
	}

	events := make([]repository.PlatformEvent, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		workspace, err := idFrom(row.TenantID)
		if err != nil {
			return nil, err
		}
		var actor shared.ID
		if row.ActorID.Valid {
			if actor, err = idFrom(row.ActorID); err != nil {
				return nil, err
			}
		}
		var payload map[string]any
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return nil, shared.ErrInternal.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("decoding an installation event: %w", err))
		}
		at := row.OccurredAt.Time.UTC()
		events = append(events, repository.PlatformEvent{
			Number: row.Number,
			Envelope: event.Envelope{
				ID: id, Type: event.Type(row.EventType), TenantID: workspace,
				Subject:    "workspace/" + workspace.String(),
				Actor:      event.Actor{Kind: shared.ActorKind(row.ActorType), ID: actor},
				OccurredAt: at, ReceivedAt: at, CorrelationID: id, Payload: payload,
			},
		})
	}
	return events, nil
}

// ForgetReference removes the reference from the transaction's workspace's events.
func (PlatformEventRepository) ForgetReference(ctx context.Context) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if _, err := queries.ForgetPlatformEventReference(ctx); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing the reference from the installation's events: %w", err))
	}
	return nil
}

// Sweep removes a batch of expired events.
func (PlatformEventRepository) Sweep(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	removed, err := queries.DeleteExpiredPlatformEvents(ctx, sqlc.DeleteExpiredPlatformEventsParams{
		Cutoff: pgtype.Timestamptz{Time: cutoff.UTC(), Valid: true}, Batch: int32(batch), //nolint:gosec // bounded by the caller
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("sweeping the installation's events: %w", err))
	}
	return int(removed), nil
}
