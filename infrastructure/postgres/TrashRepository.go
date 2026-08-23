// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The lifecycle side of both aggregates, in one file: the archive stamp, the way into the trash and
// out of it, and the hard delete at the end.
//
// It sits beside the two repositories rather than inside them because the invariant it implements
// is one that spans them - a container's deletion takes items with it, and both are keyed on the
// same batch (I-C2) - and because a reader looking for "where does something actually get deleted"
// should find one file rather than three.
//
// Nothing here names a tenant. The transaction the caller opened decided that, and row level
// security applies it to every statement below (ADR-0010), which is why the cross-tenant tests in
// test/integration are what prove this file correct.

// TrashRepository reads what is in the trash and finally removes it.
type TrashRepository struct {
	cursors security.CursorCodec
}

func NewTrashRepository(cursors security.CursorCodec) TrashRepository {
	return TrashRepository{cursors: cursors}
}

var _ repository.Trash = TrashRepository{}

// SetArchived writes an item's archive stamp, set or cleared.
func (r ItemRepository) SetArchived(
	ctx context.Context, item work.WorkItem, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(item.ID)
	if err != nil {
		return err
	}

	// A nil stamp is SQL NULL rather than the zero time: unarchiving clears the column, and a zero
	// timestamp in it would read back as "archived in the year one".
	archivedAt := pgtype.Timestamptz{}
	if item.ArchivedAt != nil {
		archivedAt = timestampOf(*item.ArchivedAt)
	}

	affected, err := queries.SetWorkItemArchived(ctx, sqlc.SetWorkItemArchivedParams{
		ArchivedAt:      archivedAt,
		UpdatedAt:       timestampOf(item.UpdatedAt),
		ID:              id,
		ExpectedVersion: versionColumn(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the archive stamp of %s: %w", item.ID, err))
	}
	return versionConflictIfUntouched(affected, item.ID, expectedVersion)
}

// TrashSubtree puts an item and everything under it into the trash under one batch.
//
// Two statements and not one: the item's own row carries the optimistic lock the caller read
// against, and its descendants do not - they are not rows anybody read, and refusing the deletion
// because a child changed while the request was in flight would make deleting a busy subtree a
// matter of luck. The item is excluded from the subtree statement so that one deletion moves its
// version once.
func (r ItemRepository) TrashSubtree(ctx context.Context, trash repository.ItemTrash) (int, error) {
	return r.moveThroughTheTrash(ctx, trash, true)
}

// RestoreBatch takes one deletion back out of the trash, whole.
//
// The batch rather than the subtree: a younger deletion inside the same subtree is a decision
// somebody else made and stays in the trash. It reaches the items of a trashed collection too -
// they carry the container's batch, so a container's restore ends here for its entries.
func (r ItemRepository) RestoreBatch(ctx context.Context, restore repository.ItemTrash) (int, error) {
	return r.moveThroughTheTrash(ctx, restore, false)
}

// moveThroughTheTrash is the two directions in one implementation. They differ in the stamp being
// written and in what the second statement selects on - a path on the way in, a batch on the way
// out - and in nothing else, and two copies of the version handling is two places for it to drift.
func (r ItemRepository) moveThroughTheTrash(
	ctx context.Context, trash repository.ItemTrash, entering bool,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	id, err := uuidOf(trash.Item.ID)
	if err != nil {
		return 0, err
	}
	batch, err := uuidOf(trash.BatchID)
	if err != nil {
		return 0, err
	}

	deletedAt := pgtype.Timestamptz{}
	batchColumn := pgtype.UUID{}
	if entering {
		if trash.Item.DeletedAt == nil {
			// The domain stamps the row before it gets here. A row without the stamp would clear
			// the column instead of setting it, which is a restore wearing a deletion's name.
			return 0, shared.ErrInternal.WithDetail("items.trash_stamp_missing")
		}
		deletedAt = timestampOf(*trash.Item.DeletedAt)
		batchColumn = batch
	}

	affected, err := queries.SetWorkItemTrashed(ctx, sqlc.SetWorkItemTrashedParams{
		DeletedAt:       deletedAt,
		TrashBatchID:    batchColumn,
		UpdatedAt:       timestampOf(trash.Item.UpdatedAt),
		ID:              id,
		ExpectedVersion: versionColumn(trash.ExpectedVersion),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the deletion stamp of %s: %w", trash.Item.ID, err))
	}
	if err := versionConflictIfUntouched(affected, trash.Item.ID, trash.ExpectedVersion); err != nil {
		return 0, err
	}

	var rest int64
	if entering {
		rest, err = queries.TrashWorkItemDescendants(ctx, sqlc.TrashWorkItemDescendantsParams{
			DeletedAt:    deletedAt,
			TrashBatchID: batch,
			UpdatedAt:    timestampOf(trash.Item.UpdatedAt),
			Prefix:       trash.Prefix,
			ItemID:       id,
		})
	} else {
		rest, err = queries.RestoreWorkItemBatch(ctx, sqlc.RestoreWorkItemBatchParams{
			UpdatedAt:    timestampOf(trash.Item.UpdatedAt),
			TrashBatchID: batch,
			ItemID:       id,
		})
	}
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("moving the subtree of %s through the trash: %w", trash.Item.ID, err))
	}
	return int(rest) + 1, nil
}

// TrashSubtree puts a hub or a collection and everything under it into the trash under one batch.
//
// Three statements for a hub and two for a collection, in one method because they must not be
// separable: a hub in the trash whose collections are still live is a tree that does not describe
// itself, and a collection in the trash whose items are not is a deletion no restore could reverse.
func (r ContainerRepository) TrashSubtree(
	ctx context.Context, trash repository.ContainerTrash,
) (repository.Cascade, error) {
	return r.moveThroughTheTrash(ctx, trash, true)
}

// RestoreBatch takes one container deletion back out of the trash, whole.
func (r ContainerRepository) RestoreBatch(
	ctx context.Context, restore repository.ContainerTrash,
) (repository.Cascade, error) {
	return r.moveThroughTheTrash(ctx, restore, false)
}

func (r ContainerRepository) moveThroughTheTrash(
	ctx context.Context, trash repository.ContainerTrash, entering bool,
) (repository.Cascade, error) {
	queries, id, err := containerWrite(ctx, trash.Container.ID)
	if err != nil {
		return repository.Cascade{}, err
	}
	batch, err := uuidOf(trash.BatchID)
	if err != nil {
		return repository.Cascade{}, err
	}

	deletedAt := pgtype.Timestamptz{}
	batchColumn := pgtype.UUID{}
	if entering {
		if trash.Container.DeletedAt == nil {
			return repository.Cascade{}, shared.ErrInternal.
				WithDetail("containers.trash_stamp_missing")
		}
		deletedAt = timestampOf(*trash.Container.DeletedAt)
		batchColumn = batch
	}
	updatedAt := timestampOf(trash.Container.UpdatedAt)

	affected, err := queries.SetContainerTrashed(ctx, sqlc.SetContainerTrashedParams{
		DeletedAt:       deletedAt,
		TrashBatchID:    batchColumn,
		UpdatedAt:       updatedAt,
		ID:              id,
		ExpectedVersion: versionColumn(trash.ExpectedVersion),
	})
	if err != nil {
		return repository.Cascade{}, containerWriteError(err, trash.Container, "writing the deletion stamp")
	}
	if err := containerConflict(affected, trash.Container, trash.ExpectedVersion); err != nil {
		return repository.Cascade{}, err
	}

	// Which collections the deletion covers. A hub's are read from the statement that stamps them;
	// a collection covers itself, and its own row has just been written.
	var covered []pgtype.UUID
	switch {
	case trash.Container.Type == work.ContainerCollection:
		covered = []pgtype.UUID{id}
	case entering:
		covered, err = queries.TrashCollectionsOfHub(ctx, sqlc.TrashCollectionsOfHubParams{
			DeletedAt: deletedAt, TrashBatchID: batch, UpdatedAt: updatedAt, HubID: id,
		})
	default:
		covered, err = queries.RestoreContainerBatch(ctx, sqlc.RestoreContainerBatchParams{
			UpdatedAt: updatedAt, TrashBatchID: batch, ContainerID: id,
		})
	}
	if err != nil {
		return repository.Cascade{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("moving the collections of %s through the trash: %w", trash.Container.ID, err))
	}

	var items int64
	if entering {
		items, err = queries.TrashItemsOfCollections(ctx, sqlc.TrashItemsOfCollectionsParams{
			DeletedAt: deletedAt, TrashBatchID: batch, UpdatedAt: updatedAt, CollectionIds: covered,
		})
	} else {
		// On the way out the batch is the selector rather than the collections: an item trashed
		// with the container carries the container's batch, and reading it back off the collections
		// would also take items that were in the trash before this deletion and never part of it.
		items, err = queries.RestoreWorkItemBatch(ctx, sqlc.RestoreWorkItemBatchParams{
			UpdatedAt: updatedAt, TrashBatchID: batch, ItemID: id,
		})
	}
	if err != nil {
		return repository.Cascade{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("moving the entries of %s through the trash: %w", trash.Container.ID, err))
	}

	collections, err := idsFrom(covered)
	if err != nil {
		return repository.Cascade{}, err
	}
	if trash.Container.Type == work.ContainerCollection {
		// The container's own row is not part of what came along with it.
		collections = nil
	}
	return repository.Cascade{Collections: collections, Items: int(items)}, nil
}

// List returns one page of the trash, newest deletion first.
func (r TrashRepository) List(
	ctx context.Context, page repository.Page,
) (repository.TrashPage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.TrashPage{}, err
	}
	boundary, err := trashCursor(r.cursors, page.Cursor)
	if err != nil {
		return repository.TrashPage{}, err
	}

	rows, err := queries.ListTrash(ctx, sqlc.ListTrashParams{
		CursorDeletedAt: boundary.deletedAt,
		CursorID:        boundary.id,
		PageSize:        pageProbe(page.Size),
	})
	if err != nil {
		return repository.TrashPage{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the trash: %w", err))
	}

	entries := make([]work.TrashEntry, 0, len(rows))
	for _, row := range rows {
		entry, err := trashEntryFrom(row)
		if err != nil {
			return repository.TrashPage{}, err
		}
		entries = append(entries, entry)
	}

	kept, info := pageOf(entries, page.Size, r.cursors, func(entry work.TrashEntry) security.Position {
		return security.At(entry.DeletedAt.UTC().Format(time.RFC3339Nano), entry.ID)
	})
	return repository.TrashPage{Entries: kept, Info: info}, nil
}

// SubtreeIDs returns every identifier in one item's subtree, the item included.
func (r TrashRepository) SubtreeIDs(ctx context.Context, prefix string) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.WorkItemSubtreeIDs(ctx, prefix)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading a subtree before purging it: %w", err))
	}
	return idsFrom(rows)
}

// PurgeItems removes items for good, by identifier.
func (r TrashRepository) PurgeItems(ctx context.Context, ids []shared.ID) (int, error) {
	return purge(ctx, ids, func(queries *sqlc.Queries, keys []pgtype.UUID) (int64, error) {
		return queries.PurgeWorkItems(ctx, keys)
	})
}

// PurgeContainers removes hubs and collections for good, by identifier. Collections before the hubs
// that hold them - `container.parent_id` is ON DELETE RESTRICT, and a hub whose collections are
// still there refuses to go.
func (r TrashRepository) PurgeContainers(ctx context.Context, ids []shared.ID) (int, error) {
	return purge(ctx, ids, func(queries *sqlc.Queries, keys []pgtype.UUID) (int64, error) {
		return queries.PurgeContainers(ctx, keys)
	})
}

// purge is the shape both hard deletes share: nothing to do for an empty list, and a count back.
//
// The empty case is checked rather than sent, because `= ANY('{}')` is a statement that matches
// nothing and still takes a round trip - and a retention run over a tenant with nothing to delete
// is the commonest run there is.
func purge(
	ctx context.Context, ids []shared.ID,
	remove func(*sqlc.Queries, []pgtype.UUID) (int64, error),
) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	keys, err := uuidsOf(ids)
	if err != nil {
		return 0, err
	}

	affected, err := remove(queries, keys)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("purging %d rows: %w", len(ids), err))
	}
	return int(affected), nil
}

// trashBoundary is a decoded trash cursor: the deletion time and the identifier the page continues
// after. Both absent for the first page, which is what makes the statement's
// `cursor_deleted_at IS NULL` mean "start at the newest".
type trashBoundary struct {
	deletedAt pgtype.Timestamptz
	id        pgtype.UUID
}

// trashCursor decodes the boundary. The sort key is a timestamp rather than a text column here, so
// it is parsed rather than passed through - a malformed one is a cursor this server did not
// produce, which the signature should already have caught, and it is refused the same way.
func trashCursor(cursors security.CursorCodec, cursor string) (trashBoundary, error) {
	if cursor == "" {
		return trashBoundary{}, nil
	}

	position, err := cursors.Decode(cursor)
	if err != nil {
		return trashBoundary{}, err
	}
	deletedAt, err := time.Parse(time.RFC3339Nano, position.SortKey())
	if err != nil {
		return trashBoundary{}, shared.ErrValidation.WithDetail("shared.cursor_invalid").WithCause(err)
	}
	id, err := uuidOf(position.ID)
	if err != nil {
		return trashBoundary{}, err
	}
	return trashBoundary{deletedAt: timestampOf(deletedAt), id: id}, nil
}

// trashEntryFrom maps one row of the union onto the projection.
func trashEntryFrom(row sqlc.ListTrashRow) (work.TrashEntry, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return work.TrashEntry{}, err
	}
	batch, err := idFrom(row.TrashBatchID)
	if err != nil {
		return work.TrashEntry{}, err
	}
	// These are legitimately absent: a hub sits under nothing and is its own level, and a task's
	// parent is its collection rather than another item. optionalID is what keeps that an empty
	// identifier rather than an error about a NULL.
	hub, err := optionalID(row.HubID)
	if err != nil {
		return work.TrashEntry{}, err
	}
	collection, err := optionalID(row.CollectionID)
	if err != nil {
		return work.TrashEntry{}, err
	}
	parent, err := optionalID(row.ParentID)
	if err != nil {
		return work.TrashEntry{}, err
	}
	if !row.DeletedAt.Valid {
		// The statement selects on `deleted_at IS NOT NULL`, so this is unreachable by construction
		// - and unreachable is exactly the state worth refusing rather than defaulting, because the
		// zero time it would default to is a deletion date forty-six years in the past.
		return work.TrashEntry{}, shared.ErrInternal.WithDetail("postgres.row_incoherent")
	}

	return work.TrashEntry{
		Kind:         work.TrashKind(row.Kind),
		ID:           id,
		BatchID:      batch,
		DeletedAt:    row.DeletedAt.Time,
		Title:        row.Title,
		Subtype:      row.Subtype,
		HubID:        hub,
		CollectionID: collection,
		ParentID:     parent,
		Version:      int(row.Version),
	}, nil
}

// idsFrom maps a column of identifiers.
func idsFrom(values []pgtype.UUID) ([]shared.ID, error) {
	ids := make([]shared.ID, 0, len(values))
	for _, value := range values {
		id, err := idFrom(value)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// uuidsOf maps identifiers into the array a statement takes.
//
// Nil for an empty set rather than an empty array, because a statement that reads the array as an
// optional restriction reads null as "no restriction" and an empty array as "restrict to nothing"
// (db/queries/Work.sql, ListWorkItems). The callers that always pass a non-empty set are unaffected.
func uuidsOf(ids []shared.ID) ([]pgtype.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	keys := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		key, err := uuidOf(id)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// versionColumn narrows a version for the statement that locks on it.
//
//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
func versionColumn(version int) int32 { return int32(version) }
