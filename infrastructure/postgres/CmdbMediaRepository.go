// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

var _ mediarepo.CiFiles = MediaRepository{}

// AttachToCi links the object to the inventory object; false where it already held it.
func (r MediaRepository) AttachToCi(ctx context.Context, ciID, mediaID shared.ID) (bool, error) {
	return r.ciLink(ctx, ciID, mediaID, true)
}

// DetachFromCi unlinks; false where there was no link.
func (r MediaRepository) DetachFromCi(ctx context.Context, ciID, mediaID shared.ID) (bool, error) {
	return r.ciLink(ctx, ciID, mediaID, false)
}

func (r MediaRepository) ciLink(ctx context.Context, ciID, mediaID shared.ID, attach bool) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	ci, err := uuidOf(ciID)
	if err != nil {
		return false, err
	}
	object, err := uuidOf(mediaID)
	if err != nil {
		return false, err
	}
	var moved int64
	if attach {
		moved, err = queries.InsertCiMedia(ctx, sqlc.InsertCiMediaParams{CiID: ci, MediaID: object})
	} else {
		moved, err = queries.DeleteCiMedia(ctx, sqlc.DeleteCiMediaParams{CiID: ci, MediaID: object})
	}
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the files of %s: %w", ciID, err))
	}
	return moved > 0, nil
}

// CiMediaIDs returns the identifiers an inventory object holds, stably ordered.
func (r MediaRepository) CiMediaIDs(ctx context.Context, ciID shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	ci, err := uuidOf(ciID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListCiMediaIDs(ctx, ci)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the file ids of %s: %w", ciID, err))
	}
	return idsFrom(rows)
}

// CountCiMedia counts the files an inventory object holds.
func (r MediaRepository) CountCiMedia(ctx context.Context, ciID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	ci, err := uuidOf(ciID)
	if err != nil {
		return 0, err
	}
	n, err := queries.CountCiMedia(ctx, ci)
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the files of %s: %w", ciID, err))
	}
	return int(n), nil
}

// ListForCi returns one page of an inventory object's files, oldest upload first.
func (r MediaRepository) ListForCi(ctx context.Context, ciID shared.ID, page repository.Page) (mediarepo.ObjectPage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return mediarepo.ObjectPage{}, err
	}
	ci, err := uuidOf(ciID)
	if err != nil {
		return mediarepo.ObjectPage{}, err
	}
	boundary, err := mediaCursor(r.cursors, page.Cursor)
	if err != nil {
		return mediarepo.ObjectPage{}, err
	}
	rows, err := queries.ListCiMedia(ctx, sqlc.ListCiMediaParams{
		CiID: ci, CursorCreatedAt: boundary.createdAt, CursorID: boundary.id, PageSize: pageProbe(page.Size),
	})
	if err != nil {
		return mediarepo.ObjectPage{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the files of %s: %w", ciID, err))
	}
	objects := make([]media.Object, 0, len(rows))
	for _, row := range rows {
		object, err := mediaObjectFrom(
			row.ID, row.TenantID, row.StorageKey, row.FileName, row.MimeType, row.ByteSize,
			row.Checksum, row.Usage, row.Status, row.RefCount, row.CreatedBy, row.CreatedAt, row.DeletedAt,
		)
		if err != nil {
			return mediarepo.ObjectPage{}, err
		}
		objects = append(objects, object)
	}
	kept, info := pageOf(objects, page.Size, r.cursors, func(object media.Object) security.Position {
		return security.At(object.CreatedAt.UTC().Format(time.RFC3339Nano), object.ID)
	})
	return mediarepo.ObjectPage{Objects: kept, Info: repository.PageInfo{NextCursor: info.NextCursor, HasMore: info.HasMore}}, nil
}

// ReferencingCis returns the hubs of the inventory objects that hold the object.
func (r MediaRepository) ReferencingCis(ctx context.Context, mediaID shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuidOf(mediaID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListMediaReferencingCis(ctx, id)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the inventory objects of %s: %w", mediaID, err))
	}
	return idsFrom(rows)
}
