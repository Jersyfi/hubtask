// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	realtimerepo "github.com/Jersyfi/hubtask/core/application/repository/realtime"
	"github.com/Jersyfi/hubtask/core/domain/model/realtime"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// RealtimeRepository stores live documents (migration 0230): realtime.Documents and, for the
// retention sweep, realtime.History.
type RealtimeRepository struct{}

// NewRealtimeRepository answers the store.
func NewRealtimeRepository() RealtimeRepository { return RealtimeRepository{} }

// Append takes the document's next place and writes the update there.
func (RealtimeRepository) Append(
	ctx context.Context, ref realtime.Ref, data []byte, author shared.ID, at time.Time,
) (int64, error) {
	queries, id, err := realtimeOpen(ctx, ref)
	if err != nil {
		return 0, err
	}
	stamp := pgtype.Timestamptz{Time: at, Valid: true}
	if err := queries.OpenRealtimeDocument(ctx, sqlc.OpenRealtimeDocumentParams{
		Kind: string(ref.Kind), DocumentID: id, At: stamp,
	}); err != nil {
		return 0, realtimeFailed("opening the document", err)
	}
	seq, err := queries.AdvanceRealtimeHead(ctx, sqlc.AdvanceRealtimeHeadParams{
		At: stamp, Kind: string(ref.Kind), DocumentID: id,
	})
	if err != nil {
		return 0, realtimeFailed("taking the next place", err)
	}
	authorID, err := optionalUUID(author)
	if err != nil {
		return 0, err
	}
	if err := queries.InsertRealtimeUpdate(ctx, sqlc.InsertRealtimeUpdateParams{
		Kind: string(ref.Kind), DocumentID: id, Seq: seq, Data: data, AuthorID: authorID, At: stamp,
	}); err != nil {
		return 0, realtimeFailed("writing the update", err)
	}
	return seq, nil
}

// Head answers where the log stands; a document never written stands at zero.
func (RealtimeRepository) Head(ctx context.Context, ref realtime.Ref) (realtime.Head, error) {
	queries, id, err := realtimeOpen(ctx, ref)
	if err != nil {
		return realtime.Head{}, err
	}
	row, err := queries.RealtimeHead(ctx, sqlc.RealtimeHeadParams{Kind: string(ref.Kind), DocumentID: id})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return realtime.Head{}, nil
	case err != nil:
		return realtime.Head{}, realtimeFailed("reading the head", err)
	}
	return realtime.Head{Seq: row.HeadSeq, SnapshotSeq: row.SnapshotSeq}, nil
}

// Snapshot answers the latest snapshot.
func (RealtimeRepository) Snapshot(ctx context.Context, ref realtime.Ref) (realtime.Snapshot, bool, error) {
	queries, id, err := realtimeOpen(ctx, ref)
	if err != nil {
		return realtime.Snapshot{}, false, err
	}
	row, err := queries.LatestRealtimeSnapshot(ctx, sqlc.LatestRealtimeSnapshotParams{Kind: string(ref.Kind), DocumentID: id})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return realtime.Snapshot{}, false, nil
	case err != nil:
		return realtime.Snapshot{}, false, realtimeFailed("reading the snapshot", err)
	}
	return realtime.Snapshot{UpToSeq: row.UpToSeq, State: row.State, At: timeFrom(row.CreatedAt)}, true, nil
}

// Since answers the updates after a place, oldest first.
func (RealtimeRepository) Since(ctx context.Context, ref realtime.Ref, after int64, limit int) ([]realtime.Update, error) {
	queries, id, err := realtimeOpen(ctx, ref)
	if err != nil {
		return nil, err
	}
	rows, err := queries.RealtimeUpdatesSince(ctx, sqlc.RealtimeUpdatesSinceParams{
		Kind: string(ref.Kind), DocumentID: id, After: after,
		Batch: int32(limit), //nolint:gosec // bounded by realtime.MaxOpenUpdates
	})
	if err != nil {
		return nil, realtimeFailed("reading the updates", err)
	}
	return mapRows(rows, func(row sqlc.RealtimeUpdatesSinceRow) (realtime.Update, error) {
		author, err := optionalID(row.AuthorID)
		if err != nil {
			return realtime.Update{}, err
		}
		return realtime.Update{Seq: row.Seq, Data: row.Data, AuthorID: author, At: timeFrom(row.CreatedAt)}, nil
	})
}

// StoreSnapshot writes the snapshot and marks what it covers and what it replaces.
func (RealtimeRepository) StoreSnapshot(ctx context.Context, ref realtime.Ref, snapshot realtime.Snapshot) (bool, error) {
	queries, id, err := realtimeOpen(ctx, ref)
	if err != nil {
		return false, err
	}
	stamp := pgtype.Timestamptz{Time: snapshot.At, Valid: true}
	moved, err := queries.AdvanceRealtimeSnapshot(ctx, sqlc.AdvanceRealtimeSnapshotParams{
		UpToSeq: snapshot.UpToSeq, At: stamp, Kind: string(ref.Kind), DocumentID: id,
	})
	if err != nil {
		return false, realtimeFailed("moving the snapshot mark", err)
	}
	if moved == 0 {
		return false, nil
	}
	if err := queries.InsertRealtimeSnapshot(ctx, sqlc.InsertRealtimeSnapshotParams{
		Kind: string(ref.Kind), DocumentID: id, UpToSeq: snapshot.UpToSeq, State: snapshot.State, At: stamp,
	}); err != nil {
		return false, realtimeFailed("writing the snapshot", err)
	}
	if err := queries.SupersedeRealtimeSnapshots(ctx, sqlc.SupersedeRealtimeSnapshotsParams{
		At: stamp, Kind: string(ref.Kind), DocumentID: id, UpToSeq: snapshot.UpToSeq,
	}); err != nil {
		return false, realtimeFailed("superseding the older snapshot", err)
	}
	if err := queries.CompactRealtimeUpdates(ctx, sqlc.CompactRealtimeUpdatesParams{
		At: stamp, Kind: string(ref.Kind), DocumentID: id, UpToSeq: snapshot.UpToSeq,
	}); err != nil {
		return false, realtimeFailed("marking the compacted updates", err)
	}
	return true, nil
}

// Erase removes the document whole.
func (RealtimeRepository) Erase(ctx context.Context, ref realtime.Ref) error {
	queries, id, err := realtimeOpen(ctx, ref)
	if err != nil {
		return err
	}
	if err := queries.EraseRealtimeDocument(ctx, sqlc.EraseRealtimeDocumentParams{Kind: string(ref.Kind), DocumentID: id}); err != nil {
		return realtimeFailed("erasing the document", err)
	}
	return nil
}

// CountExpired counts the compacted updates and superseded snapshots older than the cutoff.
func (RealtimeRepository) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	stamp := pgtype.Timestamptz{Time: cutoff, Valid: true}
	bound := int32(ceiling) //nolint:gosec // the sweep's batch size
	updates, err := queries.CountExpiredRealtimeUpdates(ctx, sqlc.CountExpiredRealtimeUpdatesParams{Cutoff: stamp, Ceiling: bound})
	if err != nil {
		return 0, realtimeFailed("counting the expired updates", err)
	}
	snapshots, err := queries.CountExpiredRealtimeSnapshots(ctx, sqlc.CountExpiredRealtimeSnapshotsParams{Cutoff: stamp, Ceiling: bound})
	if err != nil {
		return 0, realtimeFailed("counting the expired snapshots", err)
	}
	return min(int(updates)+int(snapshots), ceiling), nil
}

// DeleteExpired removes up to a batch of each.
func (RealtimeRepository) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	stamp := pgtype.Timestamptz{Time: cutoff, Valid: true}
	bound := int32(batch) //nolint:gosec // the sweep's batch size
	updates, err := queries.DeleteExpiredRealtimeUpdates(ctx, sqlc.DeleteExpiredRealtimeUpdatesParams{Cutoff: stamp, Batch: bound})
	if err != nil {
		return 0, realtimeFailed("removing the expired updates", err)
	}
	snapshots, err := queries.DeleteExpiredRealtimeSnapshots(ctx, sqlc.DeleteExpiredRealtimeSnapshotsParams{Cutoff: stamp, Batch: bound})
	if err != nil {
		return 0, realtimeFailed("removing the expired snapshots", err)
	}
	return int(updates + snapshots), nil
}

func realtimeOpen(ctx context.Context, ref realtime.Ref) (*sqlc.Queries, pgtype.UUID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	id, err := uuidOf(ref.ID)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	return queries, id, nil
}

func realtimeFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

var (
	_ realtimerepo.Documents = RealtimeRepository{}
	_ realtimerepo.History   = RealtimeRepository{}
)
