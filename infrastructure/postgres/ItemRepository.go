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

// Neighbours returns the ranks either side of a position at one level.
func (r ItemRepository) Neighbours(
	ctx context.Context, level repository.Level, beforeID, movingID shared.ID,
) (string, string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", "", err
	}

	collection, err := uuidOf(level.CollectionID)
	if err != nil {
		return "", "", err
	}
	parent, err := optionalUUID(level.ParentID)
	if err != nil {
		return "", "", err
	}
	before, err := optionalUUID(beforeID)
	if err != nil {
		return "", "", err
	}
	moving, err := optionalUUID(movingID)
	if err != nil {
		return "", "", err
	}

	row, err := queries.OrderKeyNeighbours(ctx, sqlc.OrderKeyNeighboursParams{
		CollectionID: collection,
		ParentID:     parent,
		MovingID:     moving,
		BeforeID:     before,
	})
	if err != nil {
		return "", "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the neighbours in %s: %w", level.CollectionID, err))
	}
	return row.PreviousKey, row.NextKey, nil
}

// SetOrderKey writes a new rank for one item.
func (r ItemRepository) SetOrderKey(ctx context.Context, item work.WorkItem, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(item.ID)
	if err != nil {
		return err
	}

	affected, err := queries.SetWorkItemOrderKey(ctx, sqlc.SetWorkItemOrderKeyParams{
		OrderKey:  item.OrderKey,
		UpdatedAt: timestampOf(item.UpdatedAt),
		ID:        id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the rank of %s: %w", item.ID, err))
	}
	return versionConflictIfUntouched(affected, item.ID, expectedVersion)
}

// MoveSubtree rewrites the item's placement and its subtree's paths, and returns the subtree's size.
//
// Two statements in the order that matters. The placement carries the optimistic lock, so it runs first and a
// stale version stops the move before any path is rewritten. The subtree rewrite then cannot fail on a
// version, which is why it is safe to have no lock of its own: it is bounded by the path prefix of a row this
// transaction has already claimed.
//
// The moved item is written by the first statement and excluded from the second, so one move moves one version
// per row. Written the other way round - both statements matching the moved item - a single move would bump its
// version twice, which is an artefact of the split rather than anything a caller asked for.
func (r ItemRepository) MoveSubtree(ctx context.Context, move repository.Move) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	id, err := uuidOf(move.Item.ID)
	if err != nil {
		return 0, err
	}
	parent, err := optionalUUID(move.TargetParentID)
	if err != nil {
		return 0, err
	}
	collection, err := uuidOf(move.CollectionID)
	if err != nil {
		return 0, err
	}

	placed, err := queries.SetWorkItemPlacement(ctx, sqlc.SetWorkItemPlacementParams{
		ParentID:     parent,
		CollectionID: collection,
		Path:         move.NewPrefix,
		//nolint:gosec // G115: a depth is bounded by the profile's MaxDepth
		Depth:     int32(move.Item.Depth + move.DepthDelta),
		OrderKey:  move.OrderKey,
		UpdatedAt: timestampOf(move.UpdatedAt),
		ID:        id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(move.ExpectedVersion),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the placement of %s: %w", move.Item.ID, err))
	}
	if err := versionConflictIfUntouched(placed, move.Item.ID, move.ExpectedVersion); err != nil {
		return 0, err
	}

	touched, err := queries.MoveWorkItemSubtree(ctx, sqlc.MoveWorkItemSubtreeParams{
		CollectionID: collection,
		NewPrefix:    move.NewPrefix,
		OldPrefix:    move.OldPrefix,
		//nolint:gosec // G115: a depth delta is bounded by the profile's MaxDepth
		DepthDelta: int32(move.DepthDelta),
		UpdatedAt:  timestampOf(move.UpdatedAt),
		ItemID:     id,
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("rewriting the subtree of %s: %w", move.Item.ID, err))
	}
	// The count is the descendants, and the moved item is one more: the subtree statement excludes it, so
	// that one move moves one version per row. Zero descendants is an ordinary leaf and not a failure.
	return int(touched) + 1, nil
}

// versionConflictIfUntouched turns an update that matched nothing into the answer a caller can act on.
//
// Either the row is gone or somebody else moved it on, and the second is the interesting one: read it again
// and reapply. It is also the answer when the row belongs to another tenant, because row level security
// removed it from the update's reach - and a caller must not be able to tell that apart from a version that
// moved (multi-tenancy.md §2).
func versionConflictIfUntouched(affected int64, id shared.ID, expectedVersion int) error {
	if affected != 0 {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("items.version_conflict").
		WithParams(map[string]string{
			"item_id": id.String(), "expected_version": strconv.Itoa(expectedVersion),
		})
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
