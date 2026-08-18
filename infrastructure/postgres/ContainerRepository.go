// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// ContainerRepository stores hubs and collections.
//
// Nothing here names a tenant. The transaction the caller opened decided that, and row level
// security applies it to every statement below (ADR-0010) - which is why the cross-tenant tests
// in test/integration are the ones that prove this file correct, not a unit test with a fake.
type ContainerRepository struct{}

func NewContainerRepository() ContainerRepository { return ContainerRepository{} }

var _ repository.Containers = ContainerRepository{}

// uniqueViolation is PostgreSQL's SQLSTATE for a broken unique constraint.
const uniqueViolation = "23505"

// containerNameIndex is the constraint that decides whether a name is free at one level. Named
// explicitly, so that a different unique violation - one added by a later migration - is not
// reported to the client as a duplicate name.
const containerNameIndex = "container_name_uq"

// Find returns the container as it is stored, including a trashed or an archived one. What that
// state means is the domain's decision (I-C2, I-C3); a query that filtered here would turn "it is
// in the trash" into "it does not exist".
func (r ContainerRepository) Find(ctx context.Context, id shared.ID) (work.Container, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.Container{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return work.Container{}, err
	}

	row, err := queries.FindContainer(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			// Also the answer when the row belongs to another tenant: row level security removed
			// it from the result, and the caller must not be able to tell the two apart
			// (multi-tenancy.md §2).
			return work.Container{}, shared.ErrNotFound.WithDetail("containers.not_found")
		}
		return work.Container{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the container: %w", err))
	}
	return containerFrom(row)
}

// LastOrderKey returns the highest rank at one level, or the empty string when the level is empty.
func (r ContainerRepository) LastOrderKey(ctx context.Context, parentID shared.ID) (string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}

	parent, err := optionalUUID(parentID)
	if err != nil {
		return "", err
	}

	key, err := queries.LastContainerOrderKey(ctx, parent)
	if err != nil {
		if IsNoRows(err) {
			// An empty level, not a failure: the first container at a level has nothing to sort
			// after.
			return "", nil
		}
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the last order key: %w", err))
	}
	return key, nil
}

// Insert writes the container.
//
// The name check is the unique index rather than a query before the insert. A check followed by
// an insert is two statements with a gap, and two requests arriving inside that gap both pass the
// check - which is how duplicates appear on exactly the busy levels where they matter.
func (r ContainerRepository) Insert(ctx context.Context, container work.Container) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(container.ID)
	if err != nil {
		return err
	}
	parent, err := optionalUUID(container.ParentID)
	if err != nil {
		return err
	}
	createdBy, err := uuidOf(container.CreatedBy)
	if err != nil {
		return err
	}

	err = queries.InsertContainer(ctx, sqlc.InsertContainerParams{
		ID:          id,
		Type:        sqlc.ContainerType(container.Type),
		ParentID:    parent,
		Name:        container.Name,
		Description: optionalText(container.Description),
		Icon:        optionalText(container.Icon),
		ColorToken:  optionalText(container.ColorToken),
		OrderKey:    container.OrderKey,
		CreatedBy:   createdBy,
		CreatedAt:   timestampOf(container.CreatedAt),
	})
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == containerNameIndex {
		return shared.ErrConflict.
			WithDetail("containers.name_taken").
			WithParams(map[string]string{"name": container.Name})
	}
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("writing the container: %w", err))
}

func containerFrom(row sqlc.FindContainerRow) (work.Container, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return work.Container{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return work.Container{}, err
	}
	createdBy, err := idFrom(row.CreatedBy)
	if err != nil {
		return work.Container{}, err
	}
	parentID, err := optionalID(row.ParentID)
	if err != nil {
		return work.Container{}, err
	}
	trashBatchID, err := optionalID(row.TrashBatchID)
	if err != nil {
		return work.Container{}, err
	}

	// The policy is parsed rather than cast. A value the column should not hold is a validation error
	// with the key named, not a silent fall back to the default: a collection working one way while its
	// configuration says another is the failure that would be hardest to notice.
	policy, err := work.ParseCompletionPolicy(row.CompletionPolicy)
	if err != nil {
		return work.Container{}, err
	}

	return work.Container{
		ID:               id,
		TenantID:         tenantID,
		Type:             work.ContainerType(row.Type),
		ParentID:         parentID,
		Name:             row.Name,
		Description:      stringFrom(row.Description),
		Icon:             stringFrom(row.Icon),
		ColorToken:       stringFrom(row.ColorToken),
		OrderKey:         row.OrderKey,
		CompletionPolicy: policy,
		ArchivedAt:       optionalTime(row.ArchivedAt),
		DeletedAt:        optionalTime(row.DeletedAt),
		TrashBatchID:     trashBatchID,
		CreatedBy:        createdBy,
		CreatedAt:        timeFrom(row.CreatedAt),
		UpdatedAt:        timeFrom(row.UpdatedAt),
		Version:          int(row.Version),
	}, nil
}

// optionalUUID turns the absent identifier into SQL NULL. A hub has no parent, and an empty
// string would be a malformed uuid rather than an absent one.
func optionalUUID(id shared.ID) (pgtype.UUID, error) {
	if id.IsZero() {
		return pgtype.UUID{}, nil
	}
	return uuidOf(id)
}

func optionalID(value pgtype.UUID) (shared.ID, error) {
	if !value.Valid {
		return "", nil
	}
	return idFrom(value)
}

// optionalText turns the empty string into SQL NULL. The domain uses "" for "not set", and a
// column full of empty strings is a column no query can filter on sensibly.
func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func timestampOf(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: !value.IsZero()}
}

// optionalTime is the nullable counterpart of timeFrom: a lifecycle timestamp that is absent is a
// nil pointer rather than the zero time, because the domain distinguishes them.
func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	at := value.Time.UTC()
	return &at
}
