// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

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

// SyncLogSweeper removes the synchronisation's records past the offline window (N-09,
// data-retention.md §3, the SYNC_LOG kind): operation log rows and tombstones, one batch of each
// per pass, the device sweep's shape. The change log is not its business - its months fall as
// partitions (drop_stream_partition).
type SyncLogSweeper struct{}

func NewSyncLogSweeper() SyncLogSweeper { return SyncLogSweeper{} }

// DeleteExpired removes up to batch operation log rows and up to batch tombstones older than the
// cutoff, and reports how many rows went. A tombstone goes only when its own purge date has
// passed too: the date is the deletion plus the window as it stood, and the later of the two is
// what a device was promised.
func (SyncLogSweeper) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	at := pgtype.Timestamptz{Time: cutoff, Valid: true}
	ops, err := queries.DeleteAgedSyncOps(ctx, sqlc.DeleteAgedSyncOpsParams{
		Cutoff: at,
		Batch:  int32(batch), //nolint:gosec // G115: the batch size is a small configuration value
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("sweeping the operation log: %w", err))
	}
	stones, err := queries.DeleteAgedTombstones(ctx, sqlc.DeleteAgedTombstonesParams{
		Cutoff: at, Now: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		Batch: int32(batch), //nolint:gosec // G115: the batch size is a small configuration value
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("sweeping the tombstones: %w", err))
	}
	return int(ops + stones), nil
}

// CountExpired answers how many records are due, up to the ceiling for each of the two tables.
func (SyncLogSweeper) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	at := pgtype.Timestamptz{Time: cutoff, Valid: true}
	ops, err := queries.CountAgedSyncOps(ctx, sqlc.CountAgedSyncOpsParams{
		Cutoff:  at,
		Ceiling: int32(ceiling), //nolint:gosec // G115: the ceiling is a small configuration value
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the aged operations: %w", err))
	}
	stones, err := queries.CountAgedTombstones(ctx, sqlc.CountAgedTombstonesParams{
		Cutoff: at, Now: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		Ceiling: int32(ceiling), //nolint:gosec // G115: the ceiling is a small configuration value
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the aged tombstones: %w", err))
	}
	return int(ops + stones), nil
}

// EpochRepository is the workspace's synchronisation epoch (N-11), a column of the tenant row.
type EpochRepository struct{}

func NewEpochRepository() EpochRepository { return EpochRepository{} }

var _ repository.Epochs = EpochRepository{}

func (EpochRepository) Current(ctx context.Context) (int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	epoch, err := queries.CurrentSyncEpoch(ctx)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the synchronisation epoch: %w", err))
	}
	return epoch, nil
}

func (EpochRepository) Advance(ctx context.Context) (int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	epoch, err := queries.AdvanceSyncEpoch(ctx)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("advancing the synchronisation epoch: %w", err))
	}
	return epoch, nil
}
