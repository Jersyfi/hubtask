// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// LabelRepository stores a collection's vocabulary.
//
// Nothing here names a tenant: the transaction the caller opened decided that, and row level
// security applies it to every statement below (ADR-0010).
type LabelRepository struct{}

func NewLabelRepository() LabelRepository { return LabelRepository{} }

var _ repository.Labels = LabelRepository{}

// labelNameIndex is the constraint that decides whether a name is free in one collection.
const labelNameIndex = "label_name_uq"

// Find returns the label as it is stored, a deleted one included.
func (r LabelRepository) Find(ctx context.Context, id shared.ID) (work.Label, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.Label{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return work.Label{}, err
	}

	row, err := queries.FindLabel(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			// Also the answer when the row belongs to another tenant (multi-tenancy.md §2).
			return work.Label{}, shared.ErrNotFound.WithDetail("labels.not_found")
		}
		return work.Label{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the label: %w", err))
	}
	return labelFrom(row)
}

// List returns a collection's labels by name.
func (r LabelRepository) List(ctx context.Context, collectionID shared.ID) ([]work.Label, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListLabels(ctx, collection)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the labels: %w", err))
	}

	labels := make([]work.Label, 0, len(rows))
	for _, row := range rows {
		label, err := labelFrom(row)
		if err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, nil
}

// Insert writes the label. The name check is the unique index rather than a query before the
// insert, for the reason BucketRepository.Insert relies on it.
func (r LabelRepository) Insert(ctx context.Context, label work.Label) error {
	queries, id, err := structureWrite(ctx, label.ID)
	if err != nil {
		return err
	}
	collection, err := uuidOf(label.CollectionID)
	if err != nil {
		return err
	}

	err = queries.InsertLabel(ctx, sqlc.InsertLabelParams{
		ID:           id,
		CollectionID: collection,
		Name:         label.Name,
		ColorToken:   label.ColorToken,
		Description:  optionalText(label.Description),
	})
	if err == nil {
		return nil
	}
	return labelWriteError(err, label, "writing the label")
}

// SetAttributes writes a label's own fields.
func (r LabelRepository) SetAttributes(
	ctx context.Context, label work.Label, expectedVersion int,
) error {
	queries, id, err := structureWrite(ctx, label.ID)
	if err != nil {
		return err
	}

	affected, err := queries.SetLabelAttributes(ctx, sqlc.SetLabelAttributesParams{
		Name:        label.Name,
		ColorToken:  label.ColorToken,
		Description: optionalText(label.Description),
		ID:          id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return labelWriteError(err, label, "writing the label attributes")
	}
	return labelConflict(affected, label, expectedVersion)
}

// SetDeleted writes the label's deletion stamp.
func (r LabelRepository) SetDeleted(
	ctx context.Context, label work.Label, expectedVersion int,
) error {
	queries, id, err := structureWrite(ctx, label.ID)
	if err != nil {
		return err
	}

	if label.DeletedAt == nil {
		// The domain has already set the stamp; the zero time would write "deleted in the year one".
		return shared.ErrInternal.WithDetail("labels.identity_incomplete")
	}

	affected, err := queries.SetLabelDeleted(ctx, sqlc.SetLabelDeletedParams{
		DeletedAt: timestampOf(*label.DeletedAt),
		ID:        id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return labelWriteError(err, label, "writing the label deletion")
	}
	return labelConflict(affected, label, expectedVersion)
}

// labelWriteError translates what the driver reports. No name in the message: the error text
// reaches the log, and a label's name is user content (rule 10, ADR-0017).
func labelWriteError(err error, label work.Label, what string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == labelNameIndex {
		return shared.ErrConflict.
			WithDetail("labels.name_taken").
			WithParams(map[string]string{"name": label.Name})
	}
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("%s of %s: %w", what, label.ID, err))
}

// labelConflict turns "no row matched" into the version conflict it is.
func labelConflict(affected int64, label work.Label, expectedVersion int) error {
	if affected != 0 {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("labels.version_conflict").
		WithParams(map[string]string{
			"label_id": label.ID.String(), "expected_version": strconv.Itoa(expectedVersion),
		})
}

func labelFrom(row sqlc.Label) (work.Label, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return work.Label{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return work.Label{}, err
	}
	collectionID, err := idFrom(row.CollectionID)
	if err != nil {
		return work.Label{}, err
	}

	return work.Label{
		ID:           id,
		TenantID:     tenantID,
		CollectionID: collectionID,
		Name:         row.Name,
		ColorToken:   row.ColorToken,
		Description:  textFrom(row.Description),
		DeletedAt:    optionalTime(row.DeletedAt),
		Version:      int(row.Version),
	}, nil
}

// ItemLabelRepository stores which items carry which labels, and the tags that decide it after an
// offline merge.
//
// Two tables behind one type. `item_label` is the membership every read goes through, and
// `set_element` is the OR-set tag that survives a merge (offline-sync.md §4.2). They are written
// together, in one method each, because writing one without the other is the failure: membership
// with no tag merges as last writer wins and loses a concurrent change, a tag with no membership is
// invisible to every read.
type ItemLabelRepository struct{}

func NewItemLabelRepository() ItemLabelRepository { return ItemLabelRepository{} }

var _ repository.ItemLabels = ItemLabelRepository{}

// List returns the labels an item carries, deleted labels left out.
func (r ItemLabelRepository) List(ctx context.Context, itemID shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListItemLabels(ctx, item)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the labels of %s: %w", itemID, err))
	}

	labels := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		labels = append(labels, id)
	}
	return labels, nil
}

// Add puts a label on an item and records the addition's tag.
func (r ItemLabelRepository) Add(
	ctx context.Context, itemID, labelID shared.ID, tag shared.HLC,
) error {
	queries, item, label, err := itemLabelWrite(ctx, itemID, labelID)
	if err != nil {
		return err
	}

	if err := queries.AddItemLabel(ctx, sqlc.AddItemLabelParams{
		ItemID:  item,
		LabelID: label,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("adding a label to %s: %w", itemID, err))
	}

	// The tag second, in the same transaction: a membership without one merges as last writer wins
	// over the whole set, which is the loss the OR-set exists to prevent.
	if err := queries.RecordSetElementAdded(ctx, sqlc.RecordSetElementAddedParams{
		ItemID:    item,
		SetName:   string(work.SetLabels),
		ElementID: label,
		Tag:       optionalText(tag.String()),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the label tag of %s: %w", itemID, err))
	}
	return nil
}

// Remove takes a label off an item, records the removal's tag, and reports whether the item carried
// it at all.
func (r ItemLabelRepository) Remove(
	ctx context.Context, itemID, labelID shared.ID, tag shared.HLC,
) (bool, error) {
	queries, item, label, err := itemLabelWrite(ctx, itemID, labelID)
	if err != nil {
		return false, err
	}

	affected, err := queries.RemoveItemLabel(ctx, sqlc.RemoveItemLabelParams{
		ItemID:  item,
		LabelID: label,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing a label from %s: %w", itemID, err))
	}

	// The tag is written whether or not a row was there. A device that removes something this
	// replica never saw added has still made a decision another replica has to merge against, and
	// a removal nobody recorded is one the re-add would silently win.
	if err := queries.RecordSetElementRemoved(ctx, sqlc.RecordSetElementRemovedParams{
		ItemID:    item,
		SetName:   string(work.SetLabels),
		ElementID: label,
		Tag:       optionalText(tag.String()),
	}); err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the label tag of %s: %w", itemID, err))
	}
	return affected != 0, nil
}

// ListFor returns the labels a page of entries carries, keyed by entry.
func (r ItemLabelRepository) ListFor(
	ctx context.Context, itemIDs []shared.ID,
) (map[shared.ID][]shared.ID, error) {
	carried := map[shared.ID][]shared.ID{}
	if len(itemIDs) == 0 {
		// Nothing to ask about. Answered here rather than by the query, so that an empty page costs
		// no round trip at all.
		return carried, nil
	}

	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	keys := make([]pgtype.UUID, 0, len(itemIDs))
	for _, id := range itemIDs {
		key, err := uuidOf(id)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	rows, err := queries.ListLabelsOfItems(ctx, keys)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the labels of %d entries: %w", len(itemIDs), err))
	}

	for _, row := range rows {
		itemID, labelID, err := idPair(row.ItemID, row.LabelID)
		if err != nil {
			return nil, err
		}
		carried[itemID] = append(carried[itemID], labelID)
	}
	return carried, nil
}

// Elements returns every tag of one item's label set.
func (r ItemLabelRepository) Elements(
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
		ItemID:  item,
		SetName: string(work.SetLabels),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the label tags of %s: %w", itemID, err))
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

// itemLabelWrite is the preamble both writes share.
func itemLabelWrite(
	ctx context.Context, itemID, labelID shared.ID,
) (*sqlc.Queries, pgtype.UUID, pgtype.UUID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, pgtype.UUID{}, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, pgtype.UUID{}, pgtype.UUID{}, err
	}
	label, err := uuidOf(labelID)
	if err != nil {
		return nil, pgtype.UUID{}, pgtype.UUID{}, err
	}
	return queries, item, label, nil
}

// tagFrom reads a stored clock reading back. An absent one is the zero reading, which the merge
// rule treats as "this replica never saw the event" rather than as the beginning of time.
//
// A stored value that cannot be parsed is a defect rather than input: only this system writes the
// column, and reading it as absent would silently resurrect an element somebody removed.
func tagFrom(value *string) (shared.HLC, error) {
	if value == nil || *value == "" {
		return shared.HLC{}, nil
	}
	hlc, err := shared.ParseHLC(*value)
	if err != nil {
		return shared.HLC{}, shared.ErrInternal.
			WithDetail("sync.hlc_malformed").
			WithCause(fmt.Errorf("reading a set element tag: %w", err))
	}
	return hlc, nil
}
