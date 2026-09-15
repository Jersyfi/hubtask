// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// SyncOpLog is the operation log's two statements (N-04): what a push did with each op_id, kept
// for the offline window. Both run inside the caller's transaction (rule 3).
type SyncOpLog struct{}

func NewSyncOpLog() SyncOpLog { return SyncOpLog{} }

var _ repository.OpLog = SyncOpLog{}

// Find answers a processed operation's record, and false for one this workspace has not seen.
func (SyncOpLog) Find(ctx context.Context, opID shared.ID) (repository.OpRecord, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.OpRecord{}, false, err
	}
	key, err := uuidOf(opID)
	if err != nil {
		return repository.OpRecord{}, false, err
	}
	row, err := queries.FindSyncOp(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return repository.OpRecord{}, false, nil
		}
		return repository.OpRecord{}, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the operation log: %w", err))
	}
	deviceID, err := optionalID(row.DeviceID)
	if err != nil {
		return repository.OpRecord{}, false, err
	}
	entityID, err := optionalID(row.EntityID)
	if err != nil {
		return repository.OpRecord{}, false, err
	}
	record := repository.OpRecord{
		OpID: opID, DeviceID: deviceID, Result: syncdomain.ResultKind(row.Result),
		EntityID: entityID, AppliedAt: timeFrom(row.AppliedAt),
	}
	if len(row.Response) > 0 {
		if err := json.Unmarshal(row.Response, &record.Response); err != nil {
			return repository.OpRecord{}, false, shared.ErrInternal.
				WithDetail("sync.payload_unreadable").
				WithCause(fmt.Errorf("reading a stored push result: %w", err))
		}
	}
	return record, true, nil
}

// Record writes one processed operation; a repeat that raced this one is left standing.
func (SyncOpLog) Record(ctx context.Context, record repository.OpRecord) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	opID, err := uuidOf(record.OpID)
	if err != nil {
		return err
	}
	deviceID, err := optionalUUID(record.DeviceID)
	if err != nil {
		return err
	}
	entityID, err := optionalUUID(record.EntityID)
	if err != nil {
		return err
	}
	var response []byte
	if record.Response != nil {
		if response, err = json.Marshal(record.Response); err != nil {
			return shared.ErrInternal.
				WithDetail("sync.payload_unserialisable").
				WithCause(fmt.Errorf("serialising a push result: %w", err))
		}
	}
	if err := queries.RecordSyncOp(ctx, sqlc.RecordSyncOpParams{
		OpID: opID, DeviceID: deviceID, Result: string(record.Result), EntityID: entityID,
		AppliedAt: timestampOf(record.AppliedAt), Response: response,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the operation: %w", err))
	}
	return nil
}

// TombstoneRepository answers whether an entity has been purged (offline-sync.md §7).
type TombstoneRepository struct{}

func NewTombstoneRepository() TombstoneRepository { return TombstoneRepository{} }

var _ repository.Tombstones = TombstoneRepository{}

// Holds reports a purged entity.
func (TombstoneRepository) Holds(ctx context.Context, entity string, id shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}
	held, err := queries.HoldsTombstone(ctx, sqlc.HoldsTombstoneParams{Entity: entity, EntityID: key})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the tombstone: %w", err))
	}
	return held, nil
}
