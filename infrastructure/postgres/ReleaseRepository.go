// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PlanningReleases stores a hub's releases and their entries (migration 0272, M2).
type PlanningReleases struct{}

var _ repository.Releases = PlanningReleases{}

// Releases answers the releases' side of the adapter.
func (r PlanningRepository) Releases() PlanningReleases { return PlanningReleases{} }

// Insert writes a planned release.
func (PlanningReleases) Insert(ctx context.Context, r planning.Release) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(r.ID)
	if err != nil {
		return err
	}
	hub, err := uuidOf(r.HubID)
	if err != nil {
		return err
	}
	createdBy, err := optionalUUID(r.CreatedBy)
	if err != nil {
		return err
	}
	if err := queries.InsertRelease(ctx, sqlc.InsertReleaseParams{
		ID: id, HubID: hub, Name: r.Name, Description: r.Description, ReleaseOn: planDate(r.ReleaseOn),
		CreatedAt: timestampOf(r.CreatedAt), CreatedBy: createdBy,
	}); err != nil {
		return planningError(err, "writing a release")
	}
	return nil
}

// Update writes a release held to the version read.
func (PlanningReleases) Update(ctx context.Context, r planning.Release, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(r.ID)
	if err != nil {
		return err
	}
	page, err := optionalUUID(r.NotesPageID)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a version held by the row
	affected, err := queries.UpdateRelease(ctx, sqlc.UpdateReleaseParams{
		ID: id, Name: r.Name, Description: r.Description, ReleaseOn: planDate(r.ReleaseOn), State: string(r.State),
		ReleasedAt: optionalTimestamp(r.ReleasedAt), NotesPageID: page, UpdatedAt: timestampOf(r.UpdatedAt),
		ExpectedVersion: int32(expected),
	})
	if err != nil {
		return planningError(err, "writing a release")
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("releases.version_conflict").
			WithParams(map[string]string{"release_id": r.ID.String(), "expected_version": strconv.Itoa(expected)})
	}
	return nil
}

// Find returns a release.
func (PlanningReleases) Find(ctx context.Context, id shared.ID) (planning.Release, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return planning.Release{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return planning.Release{}, err
	}
	row, err := queries.FindRelease(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return planning.Release{}, shared.ErrNotFound.WithDetail("releases.not_found")
	}
	if err != nil {
		return planning.Release{}, planningError(err, "reading a release")
	}
	return releaseFrom(row)
}

// List returns a hub's releases, the latest day first.
func (PlanningReleases) List(ctx context.Context, hubID shared.ID, state planning.ReleaseState, max int) ([]planning.Release, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: a bound the use case clamps
	rows, err := queries.ListReleases(ctx, sqlc.ListReleasesParams{HubID: key, State: string(state), Max: int32(max)})
	if err != nil {
		return nil, planningError(err, "listing releases")
	}
	out := make([]planning.Release, 0, len(rows))
	for _, row := range rows {
		r, err := releaseFrom(sqlc.FindReleaseRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Delete removes a release; its memberships go with it.
func (PlanningReleases) Delete(ctx context.Context, id shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteRelease(ctx, key)
	if err != nil {
		return planningError(err, "deleting a release")
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail("releases.not_found")
	}
	return nil
}

// Entries returns a release's entries.
func (PlanningReleases) Entries(ctx context.Context, releaseID shared.ID, max int) ([]repository.ReleaseMember, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(releaseID)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: a bound the use case clamps
	rows, err := queries.ListReleaseEntries(ctx, sqlc.ListReleaseEntriesParams{ReleaseID: key, Max: int32(max)})
	if err != nil {
		return nil, planningError(err, "reading a release's entries")
	}
	out := make([]repository.ReleaseMember, 0, len(rows))
	for _, row := range rows {
		entry, err := idFrom(row.EntryID)
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
		out = append(out, repository.ReleaseMember{
			EntryID: entry, AddedAt: timeFrom(row.AddedAt), Title: row.Title, TypeName: row.TypeName,
			Done: row.IsCompleted, CollectionID: collection, HubID: hub,
		})
	}
	return out, nil
}

// Count is how many entries a release holds.
func (PlanningReleases) Count(ctx context.Context, releaseID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	key, err := uuidOf(releaseID)
	if err != nil {
		return 0, err
	}
	n, err := queries.CountReleaseEntries(ctx, key)
	if err != nil {
		return 0, planningError(err, "counting a release's entries")
	}
	return int(n), nil
}

// Of answers the release each of the entries is in.
func (PlanningReleases) Of(ctx context.Context, entryIDs []shared.ID) ([]repository.EntryRelease, error) {
	if len(entryIDs) == 0 {
		return nil, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(entryIDs)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListEntryReleases(ctx, keys)
	if err != nil {
		return nil, planningError(err, "reading entries' releases")
	}
	out := make([]repository.EntryRelease, 0, len(rows))
	for _, row := range rows {
		entry, err := idFrom(row.EntryID)
		if err != nil {
			return nil, err
		}
		release, err := idFrom(row.ReleaseID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.EntryRelease{EntryID: entry, ReleaseID: release})
	}
	return out, nil
}

// Add puts an entry into a release; an entry already in another one is a conflict.
func (PlanningReleases) Add(ctx context.Context, releaseID, entryID shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	release, err := uuidOf(releaseID)
	if err != nil {
		return err
	}
	entry, err := uuidOf(entryID)
	if err != nil {
		return err
	}
	err = queries.AddReleaseEntry(ctx, sqlc.AddReleaseEntryParams{ReleaseID: release, EntryID: entry, AddedAt: timestampOf(at)})
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("releases.entry_in_another_release")
	}
	if err != nil {
		return planningError(err, "adding an entry to a release")
	}
	return nil
}

// Remove takes an entry out of a release.
func (PlanningReleases) Remove(ctx context.Context, releaseID, entryID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	release, err := uuidOf(releaseID)
	if err != nil {
		return false, err
	}
	entry, err := uuidOf(entryID)
	if err != nil {
		return false, err
	}
	affected, err := queries.RemoveReleaseEntry(ctx, sqlc.RemoveReleaseEntryParams{ReleaseID: release, EntryID: entry})
	if err != nil {
		return false, planningError(err, "removing an entry from a release")
	}
	return affected > 0, nil
}

func releaseFrom(row sqlc.FindReleaseRow) (planning.Release, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return planning.Release{}, err
	}
	hub, err := idFrom(row.HubID)
	if err != nil {
		return planning.Release{}, err
	}
	createdBy, err := optionalID(row.CreatedBy)
	if err != nil {
		return planning.Release{}, err
	}
	page, err := optionalID(row.NotesPageID)
	if err != nil {
		return planning.Release{}, err
	}
	return planning.Release{
		ID: id, HubID: hub, Name: row.Name, Description: row.Description, ReleaseOn: planDay(row.ReleaseOn),
		State: planning.ReleaseState(row.State), ReleasedAt: optionalTime(row.ReleasedAt), NotesPageID: page,
		CreatedAt: timeFrom(row.CreatedAt), CreatedBy: createdBy, UpdatedAt: timeFrom(row.UpdatedAt), Version: int(row.Version),
	}, nil
}
