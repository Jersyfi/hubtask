// SPDX-License-Identifier: Apache-2.0
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
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// The constraints a write of a link or a link type can meet (migration 0213).
const (
	linkTypeKeyIndex = "link_type_key_uq"
	entryLinkIndex   = "entry_link_uq"
	entryLinkType    = "entry_link_type_fkey"
)

// LinkTypeRepository stores a workspace's own link types, through the unit of work's transaction
// (rule 3).
type LinkTypeRepository struct{}

func NewLinkTypeRepository() LinkTypeRepository { return LinkTypeRepository{} }

// List returns the workspace's rows by position and key.
func (r LinkTypeRepository) List(ctx context.Context) ([]work.LinkType, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListLinkTypes(ctx)
	if err != nil {
		return nil, linkReadError(err, "listing the link types")
	}
	types := make([]work.LinkType, 0, len(rows))
	for _, row := range rows {
		linkType, err := linkTypeFrom(row)
		if err != nil {
			return nil, err
		}
		types = append(types, linkType)
	}
	return types, nil
}

// Find returns the row of a key; another workspace's is not found (multi-tenancy.md §2).
func (r LinkTypeRepository) Find(ctx context.Context, key string) (work.LinkType, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.LinkType{}, err
	}
	row, err := queries.FindLinkType(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return work.LinkType{}, shared.ErrNotFound.WithDetail("link_types.not_found")
		}
		return work.LinkType{}, linkReadError(err, "reading the link type")
	}
	return linkTypeFrom(row)
}

// Insert writes a new row.
func (r LinkTypeRepository) Insert(ctx context.Context, linkType work.LinkType) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(linkType.ID)
	if err != nil {
		return err
	}
	err = queries.InsertLinkType(ctx, sqlc.InsertLinkTypeParams{
		ID: id, Key: linkType.Key, Name: linkType.Name, InverseName: linkType.InverseName,
		Direction: string(linkType.Direction), Cardinality: string(linkType.Cardinality),
		Cycles: string(linkType.Cycles), Waits: string(linkType.Waits),
		//nolint:gosec // G115: a position is a small rank among a workspace's types
		Position:   int32(linkType.Position),
		ArchivedAt: optionalTimestamp(linkType.ArchivedAt),
		CreatedAt:  timestampOf(linkType.CreatedAt),
	})
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == linkTypeKeyIndex {
		return shared.ErrConflict.WithDetail("link_types.key_taken").
			WithParams(map[string]string{"key": linkType.Key})
	}
	return linkReadError(err, "writing the link type")
}

// Update writes a row's names, position and archive under the optimistic lock.
func (r LinkTypeRepository) Update(ctx context.Context, linkType work.LinkType, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	affected, err := queries.UpdateLinkType(ctx, sqlc.UpdateLinkTypeParams{
		Name: linkType.Name, InverseName: linkType.InverseName,
		//nolint:gosec // G115: as Insert
		Position:   int32(linkType.Position),
		ArchivedAt: optionalTimestamp(linkType.ArchivedAt),
		UpdatedAt:  timestampOf(linkType.UpdatedAt),
		Key:        linkType.Key,
		//nolint:gosec // G115: a version is a row counter
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return linkReadError(err, "updating the link type")
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("link_types.version_conflict").
			WithParams(map[string]string{"key": linkType.Key, "expected_version": strconv.Itoa(expectedVersion)})
	}
	return nil
}

// Delete removes a type no link names.
func (r LinkTypeRepository) Delete(ctx context.Context, key string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteLinkType(ctx, key)
	if err != nil {
		if isForeignKeyViolation(err) {
			return shared.ErrConflict.WithDetail("link_types.in_use")
		}
		return linkReadError(err, "deleting the link type")
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail("link_types.not_found")
	}
	return nil
}

// InUse reports whether any link names the type.
func (r LinkTypeRepository) InUse(ctx context.Context, key string) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	inUse, err := queries.LinkTypeInUse(ctx, key)
	if err != nil {
		return false, linkReadError(err, "asking whether the link type is in use")
	}
	return inUse, nil
}

func linkTypeFrom(row sqlc.LinkType) (work.LinkType, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return work.LinkType{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return work.LinkType{}, err
	}
	return work.LinkType{
		ID: id, TenantID: tenantID, Key: row.Key, Name: row.Name, InverseName: row.InverseName,
		Direction: work.LinkDirection(row.Direction), Cardinality: work.LinkCardinality(row.Cardinality),
		Cycles: work.LinkCycles(row.Cycles), Waits: work.LinkWaits(row.Waits),
		Position: int(row.Position), ArchivedAt: optionalTime(row.ArchivedAt),
		CreatedAt: timeFrom(row.CreatedAt), UpdatedAt: timeFrom(row.UpdatedAt), Version: int(row.Version),
	}, nil
}

// EntryLinkRepository stores the links between entries and reads what references resolve
// (F-3, P-19), through the unit of work's transaction (rule 3).
type EntryLinkRepository struct{}

func NewEntryLinkRepository() EntryLinkRepository { return EntryLinkRepository{} }

// Lock takes the workspace's link lock until the transaction ends.
func (r EntryLinkRepository) Lock(ctx context.Context) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if err := queries.LockEntryLinks(ctx); err != nil {
		return linkReadError(err, "locking the links")
	}
	return nil
}

// Insert writes a link.
func (r EntryLinkRepository) Insert(ctx context.Context, link work.EntryLink) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := uuidsOf([]shared.ID{link.ID, link.SourceID, link.TargetID})
	if err != nil {
		return err
	}
	createdBy, err := optionalUUID(link.CreatedBy)
	if err != nil {
		return err
	}
	if link.Timing.Kind == "" {
		link.Timing.Kind = planning.FinishToStart
	}
	err = queries.InsertEntryLink(ctx, sqlc.InsertEntryLinkParams{
		ID: ids[0], LinkType: link.Type, SourceID: ids[1], TargetID: ids[2],
		CreatedBy: createdBy, CreatedAt: timestampOf(link.CreatedAt),
		DependencyKind: string(link.Timing.Kind), LagDays: int32(link.Timing.LagDays), //nolint:gosec // G115: bounded by planning.MaxLagDays
	})
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == uniqueViolation && pgErr.ConstraintName == entryLinkIndex:
			return shared.ErrConflict.WithDetail("links.exists")
		case isForeignKeyViolation(err) && pgErr.ConstraintName == entryLinkType:
			return shared.ErrValidation.WithDetail("links.type_unknown")
		case isForeignKeyViolation(err):
			// An end that is not an entry of this workspace, said as a missing entry (T-04).
			return shared.ErrNotFound.WithDetail("items.not_found")
		}
	}
	return linkReadError(err, "writing the link")
}

// Find returns a link.
func (r EntryLinkRepository) Find(ctx context.Context, id shared.ID) (work.EntryLink, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.EntryLink{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return work.EntryLink{}, err
	}
	row, err := queries.FindEntryLink(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return work.EntryLink{}, shared.ErrNotFound.WithDetail("links.not_found")
		}
		return work.EntryLink{}, linkReadError(err, "reading the link")
	}
	return entryLinkFrom(row)
}

// Delete removes a link.
func (r EntryLinkRepository) Delete(ctx context.Context, id shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteEntryLink(ctx, key)
	if err != nil {
		return linkReadError(err, "deleting the link")
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail("links.not_found")
	}
	return nil
}

// Of returns an entry's links from both ends.
func (r EntryLinkRepository) Of(ctx context.Context, entryID shared.ID, max int) ([]work.EntryLink, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(entryID)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: a bound the use case clamps
	rows, err := queries.ListEntryLinksOf(ctx, sqlc.ListEntryLinksOfParams{EntryID: key, Max: int32(max)})
	if err != nil {
		return nil, linkReadError(err, "listing an entry's links")
	}
	links := make([]work.EntryLink, 0, len(rows))
	for _, row := range rows {
		link, err := entryLinkFrom(sqlc.FindEntryLinkRow(row))
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, nil
}

// Touching returns the links with an end among the entries.
func (r EntryLinkRepository) Touching(ctx context.Context, entryIDs []shared.ID, max int) ([]work.EntryLink, error) {
	if len(entryIDs) == 0 {
		return []work.EntryLink{}, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(entryIDs)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: a bound the use case clamps
	rows, err := queries.ListEntryLinksTouching(ctx, sqlc.ListEntryLinksTouchingParams{EntryIds: keys, Max: int32(max)})
	if err != nil {
		return nil, linkReadError(err, "listing the links among entries")
	}
	links := make([]work.EntryLink, 0, len(rows))
	for _, row := range rows {
		link, err := entryLinkFrom(sqlc.FindEntryLinkRow(row))
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, nil
}

// CountFrom counts the links of a type an entry starts.
func (r EntryLinkRepository) CountFrom(ctx context.Context, sourceID shared.ID, linkType string) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	key, err := uuidOf(sourceID)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountEntryLinksFrom(ctx, sqlc.CountEntryLinksFromParams{SourceID: key, LinkType: linkType})
	if err != nil {
		return 0, linkReadError(err, "counting an entry's links")
	}
	return int(count), nil
}

// PathExists walks the graph from one entry towards another.
func (r EntryLinkRepository) PathExists(ctx context.Context, from, to shared.ID, forward, backward []string) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	keys, err := uuidsOf([]shared.ID{from, to})
	if err != nil {
		return false, err
	}
	reaches, err := queries.EntryLinkPathExists(ctx, sqlc.EntryLinkPathExistsParams{
		Start: keys[0], Goal: keys[1], Forward: nonNil(forward), Backward: nonNil(backward),
	})
	if err != nil {
		return false, linkReadError(err, "walking the links")
	}
	return reaches, nil
}

// Entries returns the summaries of the entries among the identifiers.
func (r EntryLinkRepository) Entries(ctx context.Context, ids []shared.ID) ([]repository.EntrySummary, error) {
	if len(ids) == 0 {
		return []repository.EntrySummary{}, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(ids)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListEntrySummaries(ctx, keys)
	if err != nil {
		return nil, linkReadError(err, "reading the entries a reference points at")
	}
	out := make([]repository.EntrySummary, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		hub, err := optionalID(row.HubID)
		if err != nil {
			return nil, err
		}
		summary := repository.EntrySummary{
			ID: id, CollectionID: collection, HubID: hub, Level: work.ItemType(row.Level),
			Title: row.Title, Completed: row.IsCompleted, StartAt: optionalTime(row.StartAt),
			DueAt: optionalTime(row.DueAt), Archived: row.ArchivedAt.Valid,
		}
		if row.Handle != nil {
			summary.Handle = *row.Handle
		}
		if row.EntryType != nil {
			summary.EntryType = *row.EntryType
		}
		if row.Number != nil {
			summary.Number = *row.Number
		}
		out = append(out, summary)
	}
	return out, nil
}

// Containers returns the summaries of the hubs and collections among the identifiers.
func (r EntryLinkRepository) Containers(ctx context.Context, ids []shared.ID) ([]repository.ContainerSummary, error) {
	if len(ids) == 0 {
		return []repository.ContainerSummary{}, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(ids)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListContainerSummaries(ctx, keys)
	if err != nil {
		return nil, linkReadError(err, "reading the containers a reference points at")
	}
	out := make([]repository.ContainerSummary, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		parent, err := optionalID(row.ParentID)
		if err != nil {
			return nil, err
		}
		summary := repository.ContainerSummary{
			ID: id, Kind: work.ContainerType(row.Kind), ParentID: parent, Name: row.Name,
			Archived: row.ArchivedAt.Valid,
		}
		if row.Handle != nil {
			summary.Handle = *row.Handle
		}
		out = append(out, summary)
	}
	return out, nil
}

// Backlinks returns the texts that point at the object.
func (r EntryLinkRepository) Backlinks(ctx context.Context, target reference.Reference, max int) ([]repository.Backlink, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(target.ID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListBacklinkReferences(ctx, sqlc.ListBacklinkReferencesParams{
		//nolint:gosec // G115: a bound the use case clamps
		TargetKind: string(target.Kind), TargetID: key, Max: int32(max),
	})
	if err != nil {
		return nil, linkReadError(err, "reading the backlinks")
	}
	out := make([]repository.Backlink, 0, len(rows))
	for _, row := range rows {
		source, err := idFrom(row.SourceID)
		if err != nil {
			return nil, err
		}
		entry, err := idFrom(row.EntryID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.Backlink{
			SourceKind: row.SourceKind, SourceID: source, EntryID: entry, Anchor: row.Anchor,
			Strength: reference.Strength(row.Strength),
		})
	}
	return out, nil
}

func entryLinkFrom(row sqlc.FindEntryLinkRow) (work.EntryLink, error) {
	fields := []pgtype.UUID{row.ID, row.TenantID, row.SourceID, row.TargetID}
	ids := make([]shared.ID, len(fields))
	for i, field := range fields {
		id, err := idFrom(field)
		if err != nil {
			return work.EntryLink{}, err
		}
		ids[i] = id
	}
	createdBy, err := optionalID(row.CreatedBy)
	if err != nil {
		return work.EntryLink{}, err
	}
	return work.EntryLink{
		ID: ids[0], TenantID: ids[1], Type: row.LinkType, SourceID: ids[2], TargetID: ids[3],
		CreatedBy: createdBy, CreatedAt: timeFrom(row.CreatedAt),
		Timing: planning.Timing{Kind: planning.DependencyKind(row.DependencyKind), LagDays: int(row.LagDays)},
	}, nil
}

func linkReadError(err error, what string) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}
