// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// ContainerRepository stores hubs and collections.
//
// Nothing here names a tenant. The transaction the caller opened decided that, and row level
// security applies it to every statement below (ADR-0010) - which is why the cross-tenant tests
// in test/integration are the ones that prove this file correct, not a unit test with a fake.
type ContainerRepository struct {
	cursors security.CursorCodec
}

func NewContainerRepository(cursors security.CursorCodec) ContainerRepository {
	return ContainerRepository{cursors: cursors}
}

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

// List returns one page of one level, in the containers' manual order.
func (r ContainerRepository) List(
	ctx context.Context, query repository.ContainerQuery,
) (repository.ContainerPage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.ContainerPage{}, err
	}

	parent, err := optionalUUID(query.ParentID)
	if err != nil {
		return repository.ContainerPage{}, err
	}
	from, err := cursorAfter(r.cursors, query.Page.Cursor)
	if err != nil {
		return repository.ContainerPage{}, err
	}

	var containerType *sqlc.ContainerType
	if query.Type != "" {
		of := sqlc.ContainerType(query.Type)
		containerType = &of
	}

	rows, err := queries.ListContainers(ctx, sqlc.ListContainersParams{
		ParentID:        parent,
		Type:            containerType,
		IncludeArchived: query.IncludeArchived,
		CursorOrderKey:  from.sortKey,
		CursorID:        from.id,
		// One beyond the page, which is what answers "is there more" without a second query. The
		// extra row is dropped below rather than returned.
		PageSize: pageProbe(query.Page.Size),
	})
	if err != nil {
		return repository.ContainerPage{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the containers: %w", err))
	}

	page := repository.ContainerPage{Containers: make([]work.Container, 0, len(rows))}
	for _, row := range rows {
		container, err := containerFrom(sqlc.FindContainerRow(row))
		if err != nil {
			return repository.ContainerPage{}, err
		}
		page.Containers = append(page.Containers, container)
	}

	page.Containers, page.Info = pageOf(page.Containers, query.Page.Size, r.cursors,
		func(last work.Container) security.Position {
			return security.At(last.OrderKey, last.ID)
		})
	return page, nil
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

// SetAttributes writes the container's own descriptive fields.
//
// The name check is the unique index rather than a query before the update, for the reason Insert
// relies on it: a check followed by a write is two statements with a gap, and two renames arriving
// inside that gap both pass the check.
func (r ContainerRepository) SetAttributes(
	ctx context.Context, container work.Container, expectedVersion int,
) error {
	queries, id, err := containerWrite(ctx, container.ID)
	if err != nil {
		return err
	}

	affected, err := queries.SetContainerAttributes(ctx, sqlc.SetContainerAttributesParams{
		Name:        container.Name,
		Description: optionalText(container.Description),
		Icon:        optionalText(container.Icon),
		ColorToken:  optionalText(container.ColorToken),
		UpdatedAt:   timestampOf(container.UpdatedAt),
		ID:          id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return containerWriteError(err, container, "writing the attributes")
	}
	return containerConflict(affected, container, expectedVersion)
}

// SetPolicies writes one key of the policies document and leaves the others alone.
func (r ContainerRepository) SetPolicies(
	ctx context.Context, container work.Container, expectedVersion int,
) error {
	queries, id, err := containerWrite(ctx, container.ID)
	if err != nil {
		return err
	}

	affected, err := queries.SetContainerPolicies(ctx, sqlc.SetContainerPoliciesParams{
		CompletionPolicy: string(container.CompletionPolicy.OrDefault()),
		UpdatedAt:        timestampOf(container.UpdatedAt),
		ID:               id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return containerWriteError(err, container, "writing the policies")
	}
	return containerConflict(affected, container, expectedVersion)
}

// SetArchived writes the container's own archive stamp, set or cleared.
func (r ContainerRepository) SetArchived(
	ctx context.Context, container work.Container, expectedVersion int,
) error {
	queries, id, err := containerWrite(ctx, container.ID)
	if err != nil {
		return err
	}

	// A nil stamp is SQL NULL rather than the zero time: unarchiving clears the column, and a zero
	// timestamp in it would read back as "archived in the year one".
	archivedAt := pgtype.Timestamptz{}
	if container.ArchivedAt != nil {
		archivedAt = timestampOf(*container.ArchivedAt)
	}

	affected, err := queries.SetContainerArchived(ctx, sqlc.SetContainerArchivedParams{
		ArchivedAt: archivedAt,
		UpdatedAt:  timestampOf(container.UpdatedAt),
		ID:         id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return containerWriteError(err, container, "writing the archive stamp")
	}
	return containerConflict(affected, container, expectedVersion)
}

// SetPlacement writes where a collection sits and how it ranks there.
func (r ContainerRepository) SetPlacement(
	ctx context.Context, container work.Container, expectedVersion int,
) error {
	queries, id, err := containerWrite(ctx, container.ID)
	if err != nil {
		return err
	}
	parent, err := uuidOf(container.ParentID)
	if err != nil {
		return err
	}

	affected, err := queries.SetContainerPlacement(ctx, sqlc.SetContainerPlacementParams{
		ParentID:  parent,
		OrderKey:  container.OrderKey,
		UpdatedAt: timestampOf(container.UpdatedAt),
		ID:        id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return containerWriteError(err, container, "writing the placement")
	}
	return containerConflict(affected, container, expectedVersion)
}

// Neighbours returns the ranks either side of a position at one container level.
func (r ContainerRepository) Neighbours(
	ctx context.Context, parentID, beforeID, movingID shared.ID,
) (string, string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", "", err
	}

	parent, err := optionalUUID(parentID)
	if err != nil {
		return "", "", err
	}
	before, err := optionalUUID(beforeID)
	if err != nil {
		return "", "", err
	}
	moving, err := uuidOf(movingID)
	if err != nil {
		return "", "", err
	}

	bounds, err := queries.ContainerOrderKeyNeighbours(ctx, sqlc.ContainerOrderKeyNeighboursParams{
		ParentID: parent,
		MovingID: moving,
		BeforeID: before,
	})
	if err != nil {
		return "", "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the rank bounds: %w", err))
	}
	return bounds.PreviousKey, bounds.NextKey, nil
}

// containerWrite is the preamble every write shares: the queries of the caller's transaction, and
// the identifier as the driver's type.
func containerWrite(ctx context.Context, id shared.ID) (*sqlc.Queries, pgtype.UUID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	return queries, key, nil
}

// containerWriteError translates what the driver reports. A name already taken is the unique index
// speaking, and it is the one failure a client can act on.
//
// No name in the message: the error text reaches the log, and a container's name is user content
// (rule 10, ADR-0017). The conflict carries it, because that answer goes to the client that sent it.
func containerWriteError(err error, container work.Container, what string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == containerNameIndex {
		return shared.ErrConflict.
			WithDetail("containers.name_taken").
			WithParams(map[string]string{"name": container.Name})
	}
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("%s of %s: %w", what, container.ID, err))
}

// containerConflict turns "no row matched" into the version conflict it is.
//
// Either the container is gone or somebody else moved it on, and a row belonging to another tenant
// is the same answer: row level security removed it from the update's reach, and a caller must not
// be able to tell that apart from a version that moved (multi-tenancy.md §2).
func containerConflict(affected int64, container work.Container, expectedVersion int) error {
	if affected != 0 {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("containers.version_conflict").
		WithParams(map[string]string{
			"container_id": container.ID.String(), "expected_version": strconv.Itoa(expectedVersion),
		})
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
		ParentArchivedAt: optionalTime(row.ParentArchivedAt),
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
