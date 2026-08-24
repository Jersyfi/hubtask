// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// MediaRepository stores the media records and the attachment links (C-06).
type MediaRepository struct {
	cursors security.CursorCodec
}

func NewMediaRepository(cursors security.CursorCodec) MediaRepository {
	return MediaRepository{cursors: cursors}
}

var (
	_ mediarepo.Objects     = MediaRepository{}
	_ mediarepo.Attachments = MediaRepository{}
)

// Insert stages a new object.
func (r MediaRepository) Insert(ctx context.Context, object media.Object) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(object.ID)
	if err != nil {
		return err
	}
	createdBy, err := uuidOf(object.CreatedBy)
	if err != nil {
		return err
	}

	err = queries.InsertMediaObject(ctx, sqlc.InsertMediaObjectParams{
		ID:         id,
		StorageKey: object.StorageKey,
		FileName:   optionalText(object.FileName),
		MimeType:   object.ContentType,
		ByteSize:   object.ByteSize,
		Usage:      string(object.Usage),
		Status:     string(object.Status),
		CreatedBy:  createdBy,
		CreatedAt:  timestampOf(object.CreatedAt),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("staging the media object %s: %w", object.ID, err))
	}
	return nil
}

// Find returns the object, marked or not.
func (r MediaRepository) Find(ctx context.Context, id shared.ID) (media.Object, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return media.Object{}, err
	}
	mediaID, err := uuidOf(id)
	if err != nil {
		return media.Object{}, err
	}

	row, err := queries.FindMediaObject(ctx, mediaID)
	if err != nil {
		if IsNoRows(err) {
			return media.Object{}, shared.ErrNotFound
		}
		return media.Object{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the media object %s: %w", id, err))
	}
	return mediaObjectFrom(
		row.ID, row.TenantID, row.StorageKey, row.FileName, row.MimeType, row.ByteSize,
		row.Checksum, row.Usage, row.Status, row.RefCount, row.CreatedBy, row.CreatedAt,
		row.DeletedAt,
	)
}

// Seal writes the confirmation.
func (r MediaRepository) Seal(ctx context.Context, object media.Object) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(object.ID)
	if err != nil {
		return err
	}

	affected, err := queries.SealMediaObject(ctx, sqlc.SealMediaObjectParams{
		MimeType: object.ContentType,
		ByteSize: object.ByteSize,
		ID:       id,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("sealing the media object %s: %w", object.ID, err))
	}
	if affected == 0 {
		return shared.ErrConflict.WithDetail("media.already_confirmed")
	}
	return nil
}

// AdjustRefCount moves the counter, floored at zero.
func (r MediaRepository) AdjustRefCount(ctx context.Context, id shared.ID, delta int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	mediaID, err := uuidOf(id)
	if err != nil {
		return err
	}

	//nolint:gosec // G115: a reference delta is ±1 by construction
	if _, err := queries.AdjustMediaRefCount(ctx, sqlc.AdjustMediaRefCountParams{
		Delta: int32(delta), ID: mediaID,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("moving the reference count of %s: %w", id, err))
	}
	return nil
}

// MarkDeleted marks an unreferenced, live object.
func (r MediaRepository) MarkDeleted(ctx context.Context, id shared.ID, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	mediaID, err := uuidOf(id)
	if err != nil {
		return false, err
	}

	affected, err := queries.DeleteMediaObjectRow(ctx, sqlc.DeleteMediaObjectRowParams{
		DeletedAt: timestampOf(at), ID: mediaID,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("marking the media object %s: %w", id, err))
	}
	return affected != 0, nil
}

// Recount makes every live counter what the references say.
func (r MediaRepository) Recount(ctx context.Context) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if err := queries.RecountMediaReferences(ctx); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recounting the media references: %w", err))
	}
	return nil
}

// MarkOrphans marks what nothing references.
func (r MediaRepository) MarkOrphans(ctx context.Context, now, pendingBefore time.Time) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	marked, err := queries.MarkMediaOrphans(ctx, sqlc.MarkMediaOrphansParams{
		Now: timestampOf(now), PendingBefore: timestampOf(pendingBefore),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("marking the media orphans: %w", err))
	}
	return int(marked), nil
}

// TakeOrphans returns marked rows whose grace ended.
func (r MediaRepository) TakeOrphans(
	ctx context.Context, markedBefore time.Time, batch int,
) ([]mediarepo.Orphan, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.TakeMediaOrphans(ctx, sqlc.TakeMediaOrphansParams{
		MarkedBefore: timestampOf(markedBefore),
		//nolint:gosec // G115: a batch size is a small positive configuration value
		Batch: int32(batch),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("collecting the media orphans: %w", err))
	}

	orphans := make([]mediarepo.Orphan, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		orphans = append(orphans, mediarepo.Orphan{ID: id, StorageKey: row.StorageKey})
	}
	return orphans, nil
}

// RemoveRows deletes the records for good.
func (r MediaRepository) RemoveRows(ctx context.Context, ids []shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	uuids := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		uuid, err := uuidOf(id)
		if err != nil {
			return 0, err
		}
		uuids = append(uuids, uuid)
	}

	removed, err := queries.RemoveMediaObjectRows(ctx, uuids)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing the media records: %w", err))
	}
	return int(removed), nil
}

// ReferencingItems returns the items the object serves.
func (r MediaRepository) ReferencingItems(
	ctx context.Context, mediaID shared.ID,
) ([]mediarepo.ItemRef, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuidOf(mediaID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListMediaReferencingItems(ctx, id)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the references of %s: %w", mediaID, err))
	}

	refs := make([]mediarepo.ItemRef, 0, len(rows))
	for _, row := range rows {
		itemID, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		collectionID, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		refs = append(refs, mediarepo.ItemRef{ItemID: itemID, CollectionID: collectionID})
	}
	return refs, nil
}

// ListForItem returns one page of an item's attachments as media objects.
func (r MediaRepository) ListForItem(
	ctx context.Context, itemID shared.ID, page repository.Page,
) (mediarepo.ObjectPage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return mediarepo.ObjectPage{}, err
	}
	id, err := uuidOf(itemID)
	if err != nil {
		return mediarepo.ObjectPage{}, err
	}
	boundary, err := mediaCursor(r.cursors, page.Cursor)
	if err != nil {
		return mediarepo.ObjectPage{}, err
	}

	rows, err := queries.ListItemAttachments(ctx, sqlc.ListItemAttachmentsParams{
		ItemID:          id,
		CursorCreatedAt: boundary.createdAt,
		CursorID:        boundary.id,
		PageSize:        pageProbe(page.Size),
	})
	if err != nil {
		return mediarepo.ObjectPage{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the attachments of %s: %w", itemID, err))
	}

	objects := make([]media.Object, 0, len(rows))
	for _, row := range rows {
		object, err := mediaObjectFrom(
			row.ID, row.TenantID, row.StorageKey, row.FileName, row.MimeType, row.ByteSize,
			row.Checksum, row.Usage, row.Status, row.RefCount, row.CreatedBy, row.CreatedAt,
			row.DeletedAt,
		)
		if err != nil {
			return mediarepo.ObjectPage{}, err
		}
		objects = append(objects, object)
	}

	kept, info := pageOf(objects, page.Size, r.cursors, func(object media.Object) security.Position {
		return security.At(object.CreatedAt.UTC().Format(time.RFC3339Nano), object.ID)
	})
	return mediarepo.ObjectPage{Objects: kept, Info: repository.PageInfo(info)}, nil
}

// Add links the object to the item and records the addition's tag.
func (r MediaRepository) Add(
	ctx context.Context, itemID, mediaID shared.ID, tag shared.HLC,
) (bool, error) {
	queries, item, object, err := attachmentWrite(ctx, itemID, mediaID)
	if err != nil {
		return false, err
	}

	affected, err := queries.InsertItemAttachment(ctx, sqlc.InsertItemAttachmentParams{
		ItemID: item, MediaID: object,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("attaching %s to %s: %w", mediaID, itemID, err))
	}

	// The tag second, in the same transaction: a link without one merges as last writer wins over
	// the whole set, which is the loss the OR-set exists to prevent (offline-sync.md §4.2). It is
	// written even when the link was already there - a device that decided this has made a
	// decision another replica has to merge against.
	if err := queries.RecordSetElementAdded(ctx, sqlc.RecordSetElementAddedParams{
		ItemID:    item,
		SetName:   string(work.SetAttachments),
		ElementID: object,
		Tag:       optionalText(tag.String()),
	}); err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the attachment tag of %s: %w", itemID, err))
	}
	return affected != 0, nil
}

// Remove unlinks and records the removal's tag.
func (r MediaRepository) Remove(
	ctx context.Context, itemID, mediaID shared.ID, tag shared.HLC,
) (bool, error) {
	queries, item, object, err := attachmentWrite(ctx, itemID, mediaID)
	if err != nil {
		return false, err
	}

	affected, err := queries.DeleteItemAttachment(ctx, sqlc.DeleteItemAttachmentParams{
		ItemID: item, MediaID: object,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("detaching %s from %s: %w", mediaID, itemID, err))
	}

	if err := queries.RecordSetElementRemoved(ctx, sqlc.RecordSetElementRemovedParams{
		ItemID:    item,
		SetName:   string(work.SetAttachments),
		ElementID: object,
		Tag:       optionalText(tag.String()),
	}); err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the detachment tag of %s: %w", itemID, err))
	}
	return affected != 0, nil
}

// Elements returns every tag of one item's attachment set.
func (r MediaRepository) Elements(
	ctx context.Context, itemID shared.ID,
) ([]work.SetElement, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListSetElements(ctx, sqlc.ListSetElementsParams{
		ItemID: item, SetName: string(work.SetAttachments),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the attachment tags of %s: %w", itemID, err))
	}

	elements := make([]work.SetElement, 0, len(rows))
	for _, row := range rows {
		elementID, err := idFrom(row.ElementID)
		if err != nil {
			return nil, err
		}
		added, err := tagFrom(row.AddTag)
		if err != nil {
			return nil, err
		}
		removed, err := tagFrom(row.RemoveTag)
		if err != nil {
			return nil, err
		}
		elements = append(elements, work.SetElement{
			ElementID: elementID, AddedAt: added, RemovedAt: removed,
		})
	}
	return elements, nil
}

// attachmentWrite is the preamble both writes share.
func attachmentWrite(
	ctx context.Context, itemID, mediaID shared.ID,
) (*sqlc.Queries, pgtype.UUID, pgtype.UUID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, pgtype.UUID{}, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, pgtype.UUID{}, pgtype.UUID{}, err
	}
	object, err := uuidOf(mediaID)
	if err != nil {
		return nil, pgtype.UUID{}, pgtype.UUID{}, err
	}
	return queries, item, object, nil
}

// MediaIDs returns the identifiers an item carries.
func (r MediaRepository) MediaIDs(ctx context.Context, itemID shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListItemAttachmentIDs(ctx, item)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the attachment ids of %s: %w", itemID, err))
	}

	ids := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// mediaCursor decodes the attachments page boundary, both fields absent for the first page.
type mediaBoundary struct {
	createdAt pgtype.Timestamptz
	id        pgtype.UUID
}

func mediaCursor(cursors security.CursorCodec, cursor string) (mediaBoundary, error) {
	if cursor == "" {
		return mediaBoundary{}, nil
	}
	position, err := cursors.Decode(cursor)
	if err != nil {
		return mediaBoundary{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, position.SortKey())
	if err != nil {
		return mediaBoundary{}, shared.ErrValidation.
			WithDetail("shared.cursor_invalid").WithCause(err)
	}
	id, err := uuidOf(position.ID)
	if err != nil {
		return mediaBoundary{}, err
	}
	return mediaBoundary{createdAt: timestampOf(createdAt), id: id}, nil
}

// mediaObjectFrom maps a stored row onto the domain's object. One mapper for both selects.
func mediaObjectFrom(
	id, tenantID pgtype.UUID, storageKey string, fileName *string, mimeType string,
	byteSize int64, checksum *string, usage, status string, refCount int32,
	createdBy pgtype.UUID, createdAt, deletedAt pgtype.Timestamptz,
) (media.Object, error) {
	objectID, err := idFrom(id)
	if err != nil {
		return media.Object{}, err
	}
	tenant, err := idFrom(tenantID)
	if err != nil {
		return media.Object{}, err
	}
	creator, err := idFrom(createdBy)
	if err != nil {
		return media.Object{}, err
	}
	if !createdAt.Valid {
		return media.Object{}, shared.ErrInternal.WithDetail("postgres.row_incoherent")
	}

	return media.Object{
		ID:          objectID,
		TenantID:    tenant,
		StorageKey:  storageKey,
		FileName:    stringFrom(fileName),
		ContentType: mimeType,
		ByteSize:    byteSize,
		Checksum:    stringFrom(checksum),
		Usage:       media.Usage(usage),
		Status:      media.Status(status),
		RefCount:    int(refCount),
		CreatedBy:   creator,
		CreatedAt:   timeFrom(createdAt),
		DeletedAt:   optionalTime(deletedAt),
	}, nil
}
