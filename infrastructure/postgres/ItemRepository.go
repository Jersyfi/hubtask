// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// ItemRepository stores tasks, work packages and activities - one table for all three, because
// they are one aggregate (ADR-0006).
//
// Nothing here names a tenant. The transaction the caller opened decided that, and row level
// security applies it to every statement below (ADR-0010) - which is why the cross-tenant tests
// in test/integration are what prove this file correct, not a unit test with a fake.
type ItemRepository struct {
	cursors security.CursorCodec
}

func NewItemRepository(cursors security.CursorCodec) ItemRepository {
	return ItemRepository{cursors: cursors}
}

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

// List returns one page of one level of one collection, in the items' manual order.
//
// Unlike Find, this filters: a trashed item is not part of a level, and an archived one only when
// the caller says so. Find answers "what is this item", where the lifecycle state is the answer;
// this answers "what is in here", where it is not.
func (r ItemRepository) List(ctx context.Context, query repository.ItemQuery) (repository.ItemPage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.ItemPage{}, err
	}

	collection, err := uuidOf(query.CollectionID)
	if err != nil {
		return repository.ItemPage{}, err
	}
	parent, err := optionalUUID(query.ParentID)
	if err != nil {
		return repository.ItemPage{}, err
	}
	from, err := cursorAfter(r.cursors, query.Page.Cursor)
	if err != nil {
		return repository.ItemPage{}, err
	}

	rows, err := queries.ListWorkItems(ctx, sqlc.ListWorkItemsParams{
		CollectionID:    collection,
		ParentID:        parent,
		IncludeArchived: query.IncludeArchived,
		CursorOrderKey:  from.sortKey,
		CursorID:        from.id,
		PageSize:        pageProbe(query.Page.Size),
	})
	if err != nil {
		return repository.ItemPage{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the work items: %w", err))
	}

	page := repository.ItemPage{Items: make([]work.WorkItem, 0, len(rows))}
	for _, row := range rows {
		item, err := itemFrom(sqlc.FindWorkItemRow(row))
		if err != nil {
			return repository.ItemPage{}, err
		}
		page.Items = append(page.Items, item)
	}

	page.Items, page.Info = pageOf(page.Items, query.Page.Size, r.cursors,
		func(last work.WorkItem) security.Position {
			return security.Position{SortKey: last.OrderKey, ID: last.ID}
		})
	return page, nil
}

// ChildCompletion counts one item's children and how many are done.
func (r ItemRepository) ChildCompletion(ctx context.Context, parentID shared.ID) (work.ChildCompletion, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.ChildCompletion{}, err
	}
	parent, err := uuidOf(parentID)
	if err != nil {
		return work.ChildCompletion{}, err
	}

	// No IsNoRows branch: an aggregate over no rows is still one row, with two zeroes in it. An item
	// with no children is therefore the same answer as an item whose children were all trashed, which is
	// what the roll-up wants - it concludes nothing from either.
	row, err := queries.ChildCompletion(ctx, parent)
	if err != nil {
		return work.ChildCompletion{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the children of %s: %w", parentID, err))
	}
	return work.ChildCompletion{Total: int(row.Total), Completed: int(row.Completed)}, nil
}

// SetCompletion writes the completion, against the version the caller decided on.
func (r ItemRepository) SetCompletion(ctx context.Context, item work.WorkItem, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(item.ID)
	if err != nil {
		return err
	}
	completedBy, err := optionalUUID(item.Completion.CompletedBy)
	if err != nil {
		return err
	}

	var completedAt pgtype.Timestamptz
	if item.Completion.CompletedAt != nil {
		completedAt = timestampOf(*item.Completion.CompletedAt)
	}

	affected, err := queries.SetWorkItemCompletion(ctx, sqlc.SetWorkItemCompletionParams{
		IsCompleted: item.Completion.IsCompleted,
		CompletedAt: completedAt,
		CompletedBy: completedBy,
		UpdatedAt:   timestampOf(item.UpdatedAt),
		ID:          id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the completion of %s: %w", item.ID, err))
	}
	if affected == 0 {
		// Either it is gone or somebody else moved it on. The second is the interesting one and the one a
		// client can act on: read it again and reapply. It is also the answer when the row belongs to
		// another tenant, because row level security removed it from the update's reach - and a caller
		// must not be able to tell that apart from a version that moved (multi-tenancy.md §2).
		return shared.ErrVersionConflict.
			WithDetail("items.version_conflict").
			WithParams(map[string]string{
				"item_id": item.ID.String(), "expected_version": strconv.Itoa(expectedVersion),
			})
	}
	return nil
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
