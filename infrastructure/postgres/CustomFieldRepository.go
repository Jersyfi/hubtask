// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// CustomFieldRepository stores the definitions a workspace adds to its entries (C-07).
//
// The values are not here: they are a jsonb document on the entry, written through
// ItemRepository.SetCustomFields. This is the vocabulary, and keeping the two apart is what stops
// a read of an entry from reaching a statement written for the definitions.
//
// Nothing here names a tenant: the transaction the caller opened decided that, and row level
// security applies it to every statement below (ADR-0010).
type CustomFieldRepository struct{}

func NewCustomFieldRepository() CustomFieldRepository { return CustomFieldRepository{} }

var _ repository.CustomFields = CustomFieldRepository{}

// customFieldKeyIndex is the constraint that decides whether a key is free in one scope. Partial on
// the live rows, which is what lets a key be reused after its definition was deleted.
const customFieldKeyIndex = "cfd_key_uq"

// Insert writes a new definition.
func (r CustomFieldRepository) Insert(
	ctx context.Context, definition work.CustomFieldDefinition,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(definition.ID)
	if err != nil {
		return err
	}
	collectionID, err := optionalUUID(definition.CollectionID)
	if err != nil {
		return err
	}
	options, err := optionsOf(definition.Options)
	if err != nil {
		return err
	}

	err = queries.InsertCustomField(ctx, sqlc.InsertCustomFieldParams{
		ID: id, CollectionID: collectionID, Key: definition.Key,
		Kind: string(definition.Kind), Options: options, IsRequired: definition.IsRequired,
		AppliesTo: typesOf(definition.AppliesTo), CreatedAt: timestampOf(definition.CreatedAt),
	})
	if err != nil {
		return insertCustomFieldError(err, definition)
	}
	return nil
}

// insertCustomFieldError separates the one refusal a client can act on from everything else: the
// key is taken in this scope.
func insertCustomFieldError(err error, definition work.CustomFieldDefinition) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation &&
		pgErr.ConstraintName == customFieldKeyIndex {
		return shared.ErrConflict.
			WithDetail("fields.key_taken").
			WithParams(map[string]string{"key": definition.Key}).
			WithFields(shared.FieldError{Path: "/key", Code: "fields.key_taken"})
	}
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("writing the custom field definition: %w", err))
}

// Find returns the definition as it is stored, a deleted one included.
func (r CustomFieldRepository) Find(
	ctx context.Context, id shared.ID,
) (work.CustomFieldDefinition, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.CustomFieldDefinition{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return work.CustomFieldDefinition{}, err
	}

	row, err := queries.FindCustomField(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			// Also the answer when the row belongs to another tenant (multi-tenancy.md §2).
			return work.CustomFieldDefinition{}, shared.ErrNotFound.WithDetail("fields.not_found")
		}
		return work.CustomFieldDefinition{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the custom field definition: %w", err))
	}
	return customFieldFrom(row)
}

// FindInScope returns the live definition one collection sees under a key.
func (r CustomFieldRepository) FindInScope(
	ctx context.Context, collectionID shared.ID, key string,
) (work.CustomFieldDefinition, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.CustomFieldDefinition{}, err
	}
	scope, err := optionalUUID(collectionID)
	if err != nil {
		return work.CustomFieldDefinition{}, err
	}

	row, err := queries.FindCustomFieldInScope(ctx, sqlc.FindCustomFieldInScopeParams{
		Key: key, CollectionID: scope,
	})
	if err != nil {
		if IsNoRows(err) {
			return work.CustomFieldDefinition{}, shared.ErrNotFound.WithDetail("fields.not_found")
		}
		return work.CustomFieldDefinition{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the custom field definition by key: %w", err))
	}
	return customFieldFrom(sqlc.FindCustomFieldRow(row))
}

// ListInScope returns every live definition in force for one collection.
func (r CustomFieldRepository) ListInScope(
	ctx context.Context, collectionID shared.ID,
) ([]work.CustomFieldDefinition, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := optionalUUID(collectionID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListCustomFieldsInScope(ctx, scope)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the custom field definitions: %w", err))
	}

	definitions := make([]work.CustomFieldDefinition, 0, len(rows))
	for _, row := range rows {
		definition, err := customFieldFrom(sqlc.FindCustomFieldRow(row))
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

// SetAttributes writes what an edit may change.
func (r CustomFieldRepository) SetAttributes(
	ctx context.Context, definition work.CustomFieldDefinition, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(definition.ID)
	if err != nil {
		return err
	}
	options, err := optionsOf(definition.Options)
	if err != nil {
		return err
	}

	affected, err := queries.UpdateCustomField(ctx, sqlc.UpdateCustomFieldParams{
		Options: options, IsRequired: definition.IsRequired,
		AppliesTo: typesOf(definition.AppliesTo), UpdatedAt: timestampOf(definition.UpdatedAt),
		ID: id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the custom field definition: %w", err))
	}
	return versionConflictIfUntouched(affected, definition.ID, expectedVersion)
}

// SetDeleted marks the definition out of use.
func (r CustomFieldRepository) SetDeleted(
	ctx context.Context, definition work.CustomFieldDefinition, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(definition.ID)
	if err != nil {
		return err
	}
	if definition.DeletedAt == nil {
		// A deletion that names no moment. The domain stamps it, so reaching this is a defect
		// rather than input (security.md §9).
		return shared.ErrInternal.WithDetail("fields.deletion_stamp_missing")
	}

	affected, err := queries.SoftDeleteCustomField(ctx, sqlc.SoftDeleteCustomFieldParams{
		DeletedAt: timestampOf(*definition.DeletedAt), ID: id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("deleting the custom field definition: %w", err))
	}
	return versionConflictIfUntouched(affected, definition.ID, expectedVersion)
}

// optionsOf renders the option list for the column. An empty list is `[]` rather than SQL NULL,
// because the column is NOT NULL and "this kind offers no choices" is a document rather than the
// absence of one.
func optionsOf(options []string) ([]byte, error) {
	if len(options) == 0 {
		return []byte("[]"), nil
	}

	document, err := json.Marshal(options)
	if err != nil {
		// The options passed the domain's validation, so every one of them is a string with a JSON
		// spelling. Reaching this is a defect rather than input (security.md §9).
		return nil, shared.ErrInternal.WithDetail("fields.options_unserialisable").WithCause(err)
	}
	return document, nil
}

// typesOf maps the domain's item types onto the generated enum. A conversion rather than a lookup:
// both are the database's `item_type`, and a type the domain accepted is one the column has.
func typesOf(types []work.ItemType) []sqlc.ItemType {
	mapped := make([]sqlc.ItemType, 0, len(types))
	for _, itemType := range types {
		mapped = append(mapped, sqlc.ItemType(itemType))
	}
	return mapped
}

// customFieldFrom maps one stored row onto the aggregate.
func customFieldFrom(row sqlc.FindCustomFieldRow) (work.CustomFieldDefinition, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return work.CustomFieldDefinition{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return work.CustomFieldDefinition{}, err
	}
	collectionID, err := optionalID(row.CollectionID)
	if err != nil {
		return work.CustomFieldDefinition{}, err
	}
	options, err := optionsFrom(row.Options)
	if err != nil {
		return work.CustomFieldDefinition{}, err
	}

	appliesTo := make([]work.ItemType, 0, len(row.AppliesTo))
	for _, itemType := range row.AppliesTo {
		appliesTo = append(appliesTo, work.ItemType(itemType))
	}

	return work.CustomFieldDefinition{
		ID: id, TenantID: tenantID, CollectionID: collectionID,
		Key: row.Key, Kind: work.CustomFieldKind(row.Kind), Options: options,
		IsRequired: row.IsRequired, AppliesTo: appliesTo,
		CreatedAt: timeFrom(row.CreatedAt), UpdatedAt: timeFrom(row.UpdatedAt),
		DeletedAt: optionalTime(row.DeletedAt), Version: int(row.Version),
	}, nil
}

// optionsFrom reads the option list back. An absent or empty document is nil rather than an empty
// slice: the domain treats the two as the same thing.
func optionsFrom(document []byte) ([]string, error) {
	if len(document) == 0 {
		return nil, nil
	}

	var options []string
	if err := json.Unmarshal(document, &options); err != nil {
		return nil, shared.ErrInternal.WithDetail("fields.options_unreadable").WithCause(err)
	}
	if len(options) == 0 {
		return nil, nil
	}
	return options, nil
}
