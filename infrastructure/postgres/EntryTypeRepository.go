// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// entryTypeKeyIndex is the unique constraint a taken key violates (migration 0209).
const entryTypeKeyIndex = "entry_type_key_uq"

// EntryTypeRepository stores a workspace's type registry (UC-FLW-01), through the transaction the
// unit of work opened, so row level security decides whose rows it sees (rule 3).
type EntryTypeRepository struct{}

func NewEntryTypeRepository() EntryTypeRepository { return EntryTypeRepository{} }

// List returns the workspace's rows by level, position and key.
func (r EntryTypeRepository) List(ctx context.Context) ([]work.EntryType, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListEntryTypes(ctx)
	if err != nil {
		return nil, entryTypeReadError(err, "listing the entry types")
	}
	types := make([]work.EntryType, 0, len(rows))
	for _, row := range rows {
		entryType, err := entryTypeFrom(row)
		if err != nil {
			return nil, err
		}
		types = append(types, entryType)
	}
	return types, nil
}

// Find returns the row of a key.
func (r EntryTypeRepository) Find(ctx context.Context, key string) (work.EntryType, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.EntryType{}, err
	}
	row, err := queries.FindEntryType(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			// Also the answer when the row belongs to another tenant (multi-tenancy.md §2).
			return work.EntryType{}, shared.ErrNotFound.WithDetail("entry_types.not_found")
		}
		return work.EntryType{}, entryTypeReadError(err, "reading the entry type")
	}
	return entryTypeFrom(row)
}

// Insert writes a new row.
func (r EntryTypeRepository) Insert(ctx context.Context, entryType work.EntryType) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(entryType.ID)
	if err != nil {
		return err
	}
	err = queries.InsertEntryType(ctx, sqlc.InsertEntryTypeParams{
		ID: id, Key: entryType.Key, Level: string(entryType.Level), System: entryType.System,
		Name: entryType.Name, Icon: entryType.Icon, ColorToken: entryType.ColorToken,
		Completable: entryType.Behaviours.Completable, Workflow: entryType.Behaviours.Workflow,
		Record: entryType.Behaviours.Record, Schedulable: entryType.Behaviours.Schedulable,
		Estimable: entryType.Behaviours.Estimable, AllowedChildren: nonNil(entryType.AllowedChildren),
		//nolint:gosec // G115: a position is a small rank among a workspace's types
		Position:   int32(entryType.Position),
		ArchivedAt: optionalTimestamp(entryType.ArchivedAt),
		CreatedAt:  timestampOf(entryType.CreatedAt),
	})
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == entryTypeKeyIndex {
		return shared.ErrConflict.WithDetail("entry_types.key_taken").
			WithParams(map[string]string{"key": entryType.Key})
	}
	return entryTypeReadError(err, "writing the entry type")
}

// Update writes a row whole under the optimistic lock.
func (r EntryTypeRepository) Update(ctx context.Context, entryType work.EntryType, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	affected, err := queries.UpdateEntryType(ctx, sqlc.UpdateEntryTypeParams{
		Name: entryType.Name, Icon: entryType.Icon, ColorToken: entryType.ColorToken,
		Completable: entryType.Behaviours.Completable, Workflow: entryType.Behaviours.Workflow,
		Record: entryType.Behaviours.Record, Schedulable: entryType.Behaviours.Schedulable,
		Estimable: entryType.Behaviours.Estimable, AllowedChildren: nonNil(entryType.AllowedChildren),
		//nolint:gosec // G115: as Insert
		Position:   int32(entryType.Position),
		ArchivedAt: optionalTimestamp(entryType.ArchivedAt),
		UpdatedAt:  timestampOf(entryType.UpdatedAt),
		Key:        entryType.Key,
		//nolint:gosec // G115: a version is a row counter
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return entryTypeReadError(err, "updating the entry type")
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("entry_types.version_conflict").
			WithParams(map[string]string{"key": entryType.Key, "expected_version": strconv.Itoa(expectedVersion)})
	}
	return nil
}

// Delete removes a type no entry carries.
func (r EntryTypeRepository) Delete(ctx context.Context, key string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteEntryType(ctx, key)
	if err != nil {
		if isForeignKeyViolation(err) {
			return shared.ErrConflict.WithDetail("entry_types.in_use")
		}
		return entryTypeReadError(err, "deleting the entry type")
	}
	if affected > 0 {
		return nil
	}
	// Nothing matched: missing, a system row, or carried by an entry - the in-use question tells
	// the last apart, the application has already refused a system type.
	inUse, err := r.InUse(ctx, key)
	if err != nil {
		return err
	}
	if inUse {
		return shared.ErrConflict.WithDetail("entry_types.in_use")
	}
	return shared.ErrNotFound.WithDetail("entry_types.not_found")
}

// InUse reports whether any entry carries the type.
func (r EntryTypeRepository) InUse(ctx context.Context, key string) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	inUse, err := queries.EntryTypeInUse(ctx, key)
	if err != nil {
		return false, entryTypeReadError(err, "asking whether the entry type is in use")
	}
	return inUse, nil
}

// SetOf writes an entry's type.
func (r EntryTypeRepository) SetOf(ctx context.Context, item work.WorkItem, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(item.ID)
	if err != nil {
		return err
	}
	affected, err := queries.SetWorkItemEntryType(ctx, sqlc.SetWorkItemEntryTypeParams{
		EntryType: optionalText(item.EntryType),
		UpdatedAt: timestampOf(item.UpdatedAt),
		ID:        id,
		//nolint:gosec // G115: a version is a row counter
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return shared.ErrValidation.WithDetail("entry_types.unknown")
		}
		return entryTypeReadError(err, "writing the entry's type")
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("items.version_conflict").
			WithParams(map[string]string{"item_id": item.ID.String(), "expected_version": strconv.Itoa(expectedVersion)})
	}
	return nil
}

func entryTypeFrom(row sqlc.EntryType) (work.EntryType, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return work.EntryType{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return work.EntryType{}, err
	}
	return work.EntryType{
		ID: id, TenantID: tenantID, Key: row.Key, Level: work.ItemType(row.Level), System: row.System,
		Name: row.Name, Icon: row.Icon, ColorToken: row.ColorToken,
		Behaviours: work.EntryBehaviours{
			Completable: row.Completable, Workflow: row.Workflow, Record: row.Record,
			Schedulable: row.Schedulable, Estimable: row.Estimable,
		},
		AllowedChildren: row.AllowedChildren, Position: int(row.Position),
		ArchivedAt: optionalTime(row.ArchivedAt),
		CreatedAt:  timeFrom(row.CreatedAt), UpdatedAt: timeFrom(row.UpdatedAt),
		Version: int(row.Version),
	}, nil
}

func entryTypeReadError(err error, what string) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}
