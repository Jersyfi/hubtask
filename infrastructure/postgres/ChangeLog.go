// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// ChangeLog records state deltas for synchronising clients (offline-sync.md §10).
type ChangeLog struct{}

func NewChangeLog() ChangeLog { return ChangeLog{} }

var (
	_ changelog.ChangeLog = ChangeLog{}
	_ changelog.Changes   = ChangeLog{}
)

// Record writes one change inside the caller's transaction.
//
// The sequence number is assigned by the database. It is the cursor a client pages on, so it has
// to be gapless and monotonic per tenant; a value chosen in the application would leave a hole
// wherever a transaction rolled back, and a client would then wait for a change that is never
// coming.
func (c ChangeLog) Record(ctx context.Context, change changelog.Change) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if change.HLC.IsZero() {
		// Without a clock reading the entry cannot be merged against a concurrent edit, and a
		// client would have to guess which of the two came first (offline-sync.md §4.1).
		return shared.ErrInternal.WithDetail("sync.change_without_clock")
	}

	entityID, err := uuidOf(change.EntityID)
	if err != nil {
		return err
	}
	containerID, err := optionalUUID(change.ContainerID)
	if err != nil {
		return err
	}
	actorID, err := optionalUUID(change.ActorID)
	if err != nil {
		return err
	}
	deviceID, err := optionalUUID(change.DeviceID)
	if err != nil {
		return err
	}

	var payload []byte
	if change.Payload != nil {
		if payload, err = json.Marshal(change.Payload); err != nil {
			return shared.ErrInternal.
				WithDetail("sync.payload_unserialisable").
				WithCause(fmt.Errorf("serialising the change payload: %w", err))
		}
	}

	err = queries.RecordChange(ctx, sqlc.RecordChangeParams{
		Entity:      change.Entity,
		EntityID:    entityID,
		Op:          string(change.Op),
		ContainerID: containerID,
		ActorID:     actorID,
		DeviceID:    deviceID,
		Hlc:         change.HLC.String(),
		// The physical part of the clock reading rather than a second call to now(): it is the
		// partition key, and a row whose partition disagreed with its own clock would sort into
		// one month and read as another.
		OccurredAt: timestampOf(change.HLC.Physical),
		Payload:    payload,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the change log entry: %w", err))
	}
	return nil
}

// After returns up to batch entries past the cursor, oldest first.
//
// Read inside the caller's transaction like every repository method here, so the tenant comes from
// the transaction rather than from a parameter and row level security compares it (ADR-0010). A
// stream that passed a tenant would be a stream that could be asked for somebody else's.
func (c ChangeLog) After(
	ctx context.Context, after int64, batch int,
) ([]changelog.Recorded, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ReadChangesAfter(ctx, sqlc.ReadChangesAfterParams{
		After: after,
		Batch: int32(boundedInt32(batch)),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the change log after %d: %w", after, err))
	}

	recorded := make([]changelog.Recorded, 0, len(rows))
	for _, row := range rows {
		entry, err := recordedFrom(row)
		if err != nil {
			return nil, err
		}
		recorded = append(recorded, entry)
	}
	return recorded, nil
}

// Latest is where the log stands now. Zero for a workspace nothing has happened in.
func (c ChangeLog) Latest(ctx context.Context) (int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	latest, err := queries.LatestChangeSeq(ctx)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the head of the change log: %w", err))
	}
	return latest, nil
}

func recordedFrom(row sqlc.ReadChangesAfterRow) (changelog.Recorded, error) {
	entityID, err := idFrom(row.EntityID)
	if err != nil {
		return changelog.Recorded{}, err
	}
	containerID, err := optionalID(row.ContainerID)
	if err != nil {
		return changelog.Recorded{}, err
	}
	actorID, err := optionalID(row.ActorID)
	if err != nil {
		return changelog.Recorded{}, err
	}
	deviceID, err := optionalID(row.DeviceID)
	if err != nil {
		return changelog.Recorded{}, err
	}

	// Parsed rather than carried as text. The column is a string because that is how an HLC is
	// spelled, and a reader that handed the string on would push the parse into whoever compares
	// two of them - which is every merge (offline-sync.md §4.1).
	hlc, err := shared.ParseHLC(row.Hlc)
	if err != nil {
		return changelog.Recorded{}, shared.ErrInternal.
			WithDetail("sync.clock_unreadable").
			WithCause(fmt.Errorf("reading the clock of change %d: %w", row.Seq, err))
	}

	// Nil rather than an empty map where the column is NULL. A deletion carries no payload by
	// design, and `{}` would say "the change set is empty", which is a different statement.
	var payload map[string]any
	if len(row.Payload) > 0 {
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return changelog.Recorded{}, shared.ErrInternal.
				WithDetail("sync.payload_unreadable").
				WithCause(fmt.Errorf("reading the payload of change %d: %w", row.Seq, err))
		}
	}

	return changelog.Recorded{
		Change: changelog.Change{
			Entity:      row.Entity,
			EntityID:    entityID,
			Op:          changelog.Operation(row.Op),
			ContainerID: containerID,
			ActorID:     actorID,
			DeviceID:    deviceID,
			HLC:         hlc,
			Payload:     payload,
		},
		Seq:        row.Seq,
		OccurredAt: row.OccurredAt.Time.UTC(),
	}, nil
}
