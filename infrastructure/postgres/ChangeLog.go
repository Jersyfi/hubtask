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

var _ changelog.ChangeLog = ChangeLog{}

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
