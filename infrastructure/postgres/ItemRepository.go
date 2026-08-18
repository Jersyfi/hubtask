// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"math"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// ItemRepository stores tasks, work packages and activities - one table for all three, because
// they are one aggregate (ADR-0006).
//
// Nothing here names a tenant. The transaction the caller opened decided that, and row level
// security applies it to every statement below (ADR-0010) - which is why the cross-tenant tests
// in test/integration are what prove this file correct, not a unit test with a fake.
type ItemRepository struct{}

func NewItemRepository() ItemRepository { return ItemRepository{} }

var _ repository.Items = ItemRepository{}

// Find returns the item as it is stored, including a trashed or an archived one (I-W4).
func (r ItemRepository) Find(ctx context.Context, id shared.ID) (work.WorkItem, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.WorkItem{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return work.WorkItem{}, err
	}

	row, err := queries.FindWorkItem(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			// Also the answer when the row belongs to another tenant: row level security removed
			// it from the result, and the caller must not be able to tell the two apart
			// (multi-tenancy.md §2).
			return work.WorkItem{}, shared.ErrNotFound.WithDetail("items.not_found")
		}
		return work.WorkItem{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the work item: %w", err))
	}
	return itemFrom(row)
}

// LastOrderKey returns the highest rank among a new item's siblings, or the empty string when it
// would be the first.
func (r ItemRepository) LastOrderKey(ctx context.Context, collectionID, parentID shared.ID) (string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}

	collection, err := uuidOf(collectionID)
	if err != nil {
		return "", err
	}
	parent, err := optionalUUID(parentID)
	if err != nil {
		return "", err
	}

	key, err := queries.LastWorkItemOrderKey(ctx, sqlc.LastWorkItemOrderKeyParams{
		CollectionID: collection,
		ParentID:     parent,
	})
	if err != nil {
		if IsNoRows(err) {
			// An empty level, not a failure: the first item under a parent has nothing to sort
			// after.
			return "", nil
		}
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the last item order key: %w", err))
	}
	return key, nil
}

// Insert writes the item.
//
// There is no uniqueness to translate here, and that is deliberate rather than an omission: two
// items in one collection may share a title. A shopping list with "milk" on it twice is a list
// somebody wrote that way, and a container's name is the thing that has to be unique because it
// is how a person navigates.
func (r ItemRepository) Insert(ctx context.Context, item work.WorkItem) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(item.ID)
	if err != nil {
		return err
	}
	collection, err := uuidOf(item.CollectionID)
	if err != nil {
		return err
	}
	parent, err := optionalUUID(item.ParentID)
	if err != nil {
		return err
	}
	createdBy, err := uuidOf(item.CreatedBy)
	if err != nil {
		return err
	}

	depth, err := columnDepth(item.Depth)
	if err != nil {
		return err
	}

	err = queries.InsertWorkItem(ctx, sqlc.InsertWorkItemParams{
		ID:           id,
		CollectionID: collection,
		Type:         sqlc.ItemType(item.Type),
		ParentID:     parent,
		Path:         item.Path,
		Depth:        depth,
		Title:        item.Title,
		Notes:        optionalText(item.Notes),
		OrderKey:     item.OrderKey,
		CreatedBy:    createdBy,
		CreatedAt:    timestampOf(item.CreatedAt),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the work item: %w", err))
	}
	return nil
}

// columnDepth narrows the depth to the column's width.
//
// Refused rather than clamped, unlike the pool and batch sizes elsewhere in this package. Those
// are settings, where the nearest permitted value is the helpful answer; this is a derived value
// bounded by the capability profiles, so one that does not fit means the derivation is broken -
// and a clamped depth would write a row whose position is a lie.
func columnDepth(depth int) (int32, error) {
	if depth < 1 || depth > math.MaxInt32 {
		return 0, shared.ErrInternal.
			WithDetail("items.depth_inconsistent").
			WithParams(map[string]string{"depth": strconv.Itoa(depth)})
	}
	return int32(depth), nil
}

func itemFrom(row sqlc.FindWorkItemRow) (work.WorkItem, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return work.WorkItem{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return work.WorkItem{}, err
	}
	collectionID, err := idFrom(row.CollectionID)
	if err != nil {
		return work.WorkItem{}, err
	}
	createdBy, err := idFrom(row.CreatedBy)
	if err != nil {
		return work.WorkItem{}, err
	}
	parentID, err := optionalID(row.ParentID)
	if err != nil {
		return work.WorkItem{}, err
	}
	completedBy, err := optionalID(row.CompletedBy)
	if err != nil {
		return work.WorkItem{}, err
	}
	trashBatchID, err := optionalID(row.TrashBatchID)
	if err != nil {
		return work.WorkItem{}, err
	}

	return work.WorkItem{
		ID:           id,
		TenantID:     tenantID,
		CollectionID: collectionID,
		Type:         work.ItemType(row.Type),
		ParentID:     parentID,
		Path:         row.Path,
		Depth:        int(row.Depth),
		Title:        row.Title,
		Notes:        stringFrom(row.Notes),
		Completion: work.Completion{
			IsCompleted: row.IsCompleted,
			CompletedAt: optionalTime(row.CompletedAt),
			CompletedBy: completedBy,
		},
		OrderKey:     row.OrderKey,
		ArchivedAt:   optionalTime(row.ArchivedAt),
		DeletedAt:    optionalTime(row.DeletedAt),
		TrashBatchID: trashBatchID,
		CreatedBy:    createdBy,
		CreatedAt:    timeFrom(row.CreatedAt),
		UpdatedAt:    timeFrom(row.UpdatedAt),
		Version:      int(row.Version),
	}, nil
}
