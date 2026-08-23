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

	restrictTo, err := uuidsOf(query.RestrictTo)
	if err != nil {
		return repository.ItemPage{}, err
	}

	rows, err := queries.ListWorkItems(ctx, sqlc.ListWorkItemsParams{
		CollectionID:    collection,
		ParentID:        parent,
		IncludeArchived: query.IncludeArchived,
		RestrictTo:      restrictTo,
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
			return security.At(last.OrderKey, last.ID)
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

// SetAttributes writes an item's own fields: the title, and the notes where the type carries them.
//
// One statement writing both columns, whichever of them moved. What the row should say was decided by the
// use case, and re-deciding it here from a list of changed fields would put that rule in the layer that is
// not allowed to hold one (ADR-0005).
func (r ItemRepository) SetAttributes(ctx context.Context, item work.WorkItem, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(item.ID)
	if err != nil {
		return err
	}

	bucket, err := optionalUUID(item.BucketID)
	if err != nil {
		return err
	}

	affected, err := queries.SetWorkItemAttributes(ctx, sqlc.SetWorkItemAttributesParams{
		Title:     item.Title,
		Notes:     optionalText(item.Notes),
		BucketID:  bucket,
		UpdatedAt: timestampOf(item.UpdatedAt),
		ID:        id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		// No title and no notes in the message: the error text reaches the log, and user content does not
		// go there (rule 10, ADR-0017).
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the attributes of %s: %w", item.ID, err))
	}
	if affected == 0 {
		// Either it is gone or somebody else moved it on, and a row belonging to another tenant is the same
		// answer: row level security removed it from the update's reach, and a caller must not be able to
		// tell that apart from a version that moved (multi-tenancy.md §2).
		return shared.ErrVersionConflict.
			WithDetail("items.version_conflict").
			WithParams(map[string]string{
				"item_id": item.ID.String(), "expected_version": strconv.Itoa(expectedVersion),
			})
	}
	return nil
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

// SetAssignee writes the one person the entry is on, set or cleared, or reports a version conflict.
//
// The whole item is passed and the identifier read off it, as everywhere else in this adapter: the
// decision about what the row should say has already been taken, and an adapter handed the account
// alone would have to be trusted to put it on the right row.
func (r ItemRepository) SetAssignee(ctx context.Context, item work.WorkItem, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(item.ID)
	if err != nil {
		return err
	}
	// The zero identifier is nobody, and reaches the column as NULL rather than as a zero UUID:
	// `assignee_id` is a nullable foreign key, and a row of zeroes would be a reference to an
	// account that cannot exist.
	assignee, err := optionalUUID(item.AssigneeID)
	if err != nil {
		return err
	}

	affected, err := queries.SetWorkItemAssignee(ctx, sqlc.SetWorkItemAssigneeParams{
		AssigneeID: assignee,
		UpdatedAt:  timestampOf(item.UpdatedAt),
		ID:         id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the assignee of %s: %w", item.ID, err))
	}
	return versionConflictIfUntouched(affected, item.ID, expectedVersion)
}

// CountOpenByAssignee counts the open entries each of the given accounts carries, tenant-wide.
// What "open" means - and why an ancestor's archive is not consulted - is stated at the query.
func (r ItemRepository) CountOpenByAssignee(
	ctx context.Context, accounts []shared.ID,
) (map[shared.ID]int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]pgtype.UUID, 0, len(accounts))
	for _, account := range accounts {
		id, err := uuidOf(account)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	rows, err := queries.CountOpenItemsByAssignee(ctx, ids)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting open entries per assignee: %w", err))
	}

	load := make(map[shared.ID]int, len(rows))
	for _, row := range rows {
		account, err := idFrom(row.AssigneeID)
		if err != nil {
			return nil, err
		}
		load[account] = int(row.OpenItems)
	}
	return load, nil
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
func (r ItemRepository) MoveSubtree(
	ctx context.Context, move repository.Move,
) (int, []work.DroppedReference, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, nil, err
	}

	id, err := uuidOf(move.Item.ID)
	if err != nil {
		return 0, nil, err
	}
	parent, err := optionalUUID(move.TargetParentID)
	if err != nil {
		return 0, nil, err
	}
	collection, err := uuidOf(move.CollectionID)
	if err != nil {
		return 0, nil, err
	}

	// The moved item keeps its column only where the board did not change under it. A move to
	// another collection takes the item away from the board it was on, and a reference to a column
	// of the collection it left would be one nothing renders (I-W6).
	bucket, err := optionalUUID(move.BucketID)
	if err != nil {
		return 0, nil, err
	}

	// The references the destination cannot resolve, dropped and reported before anything moves
	// (I-W6). Before, because the subtree is found by the path it still has - and because the
	// placement statement below writes the moved item's own column, so a clear that ran after it
	// would have nothing left to report about the row a person actually dragged.
	dropped, err := r.clearForeignReferences(ctx, queries, move)
	if err != nil {
		return 0, nil, err
	}

	placed, err := queries.SetWorkItemPlacement(ctx, sqlc.SetWorkItemPlacementParams{
		ParentID:     parent,
		CollectionID: collection,
		BucketID:     bucket,
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
		return 0, nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the placement of %s: %w", move.Item.ID, err))
	}
	if err := versionConflictIfUntouched(placed, move.Item.ID, move.ExpectedVersion); err != nil {
		return 0, nil, err
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
		return 0, nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("rewriting the subtree of %s: %w", move.Item.ID, err))
	}
	// The count is the descendants, and the moved item is one more: the subtree statement excludes it, so
	// that one move moves one version per row. Zero descendants is an ordinary leaf and not a failure.
	return int(touched) + 1, dropped, nil
}

// clearForeignReferences drops what the destination collection does not define, and names it.
//
// Two statements rather than one, because they touch two tables - but neither is separable from the
// move: an entry that kept a label of the collection it left is a reference nothing renders, and one
// that kept a column is on a board it is not on (I-W6).
//
// The subtree is found by the path it still has, because this runs before the paths move. Neither
// statement moves a version: the move's own two statements bump every row in the subtree, and a
// second bump would make one move look like two.
func (r ItemRepository) clearForeignReferences(
	ctx context.Context, queries *sqlc.Queries, move repository.Move,
) ([]work.DroppedReference, error) {
	collection, err := uuidOf(move.CollectionID)
	if err != nil {
		return nil, err
	}

	labels, err := queries.ClearForeignSubtreeLabels(ctx, sqlc.ClearForeignSubtreeLabelsParams{
		PathPrefix:   move.OldPrefix,
		CollectionID: collection,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing the labels below %s: %w", move.Item.ID, err))
	}

	buckets, err := queries.ClearForeignSubtreeBuckets(ctx, sqlc.ClearForeignSubtreeBucketsParams{
		PathPrefix:   move.OldPrefix,
		CollectionID: collection,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing the buckets below %s: %w", move.Item.ID, err))
	}

	dropped := make([]work.DroppedReference, 0, len(labels)+len(buckets))
	for _, row := range labels {
		itemID, labelID, err := idPair(row.ItemID, row.LabelID)
		if err != nil {
			return nil, err
		}
		dropped = append(dropped, work.DroppedLabel(itemID, labelID))
	}
	for _, row := range buckets {
		itemID, bucketID, err := idPair(row.ID, row.BucketID)
		if err != nil {
			return nil, err
		}
		dropped = append(dropped, work.DroppedBucket(itemID, bucketID))
	}
	return dropped, nil
}

// idPair reads two identifiers of one row, so that the two loops above do not each carry four lines
// of error handling.
func idPair(left, right pgtype.UUID) (shared.ID, shared.ID, error) {
	first, err := idFrom(left)
	if err != nil {
		return "", "", err
	}
	second, err := idFrom(right)
	if err != nil {
		return "", "", err
	}
	return first, second, nil
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

	bucket, err := optionalUUID(item.BucketID)
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
		BucketID:     bucket,
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
	bucketID, err := optionalID(row.BucketID)
	if err != nil {
		return work.WorkItem{}, err
	}
	assigneeID, err := optionalID(row.AssigneeID)
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
		BucketID:     bucketID,
		OrderKey:     row.OrderKey,
		AssigneeID:   assigneeID,
		ArchivedAt:   optionalTime(row.ArchivedAt),
		DeletedAt:    optionalTime(row.DeletedAt),
		TrashBatchID: trashBatchID,
		CreatedBy:    createdBy,
		CreatedAt:    timeFrom(row.CreatedAt),
		UpdatedAt:    timeFrom(row.UpdatedAt),
		Version:      int(row.Version),
	}, nil
}
