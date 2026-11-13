// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// SearchNamesRepository is the search over every kind besides entries and pages, through the
// transaction wrapper (rule 3): row level security keeps every row in the caller's workspace.
type SearchNamesRepository struct{}

// NewSearchNamesRepository is the store.
func NewSearchNamesRepository() SearchNamesRepository { return SearchNamesRepository{} }

var _ repository.Names = SearchNamesRepository{}

func searchFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("searching %s: %w", what, err))
}

func searchArgs(ctx context.Context, ask repository.Ask) (*sqlc.Queries, pgtype.UUID, int32, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, 0, err
	}
	hub, err := optionalUUID(ask.HubID)
	batch := int32(min(max(ask.Batch, 1), 1000)) //nolint:gosec // bounded here
	return queries, hub, batch, err
}

// hitIDs reads the identifiers of a row; an absent optional one is zero.
func hitIDs(required pgtype.UUID, optional ...pgtype.UUID) (shared.ID, []shared.ID, error) {
	id, err := idFrom(required)
	if err != nil {
		return "", nil, err
	}
	out := make([]shared.ID, len(optional))
	for i, value := range optional {
		if out[i], err = optionalID(value); err != nil {
			return "", nil, err
		}
	}
	return id, out, nil
}

// Whiteboards answers the living boards by their titles.
func (SearchNamesRepository) Whiteboards(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchWhiteboardNames(ctx, sqlc.SearchWhiteboardNamesParams{HubID: hub, Terms: ask.Terms, Batch: batch})
	if err != nil {
		return nil, searchFailed("whiteboards", err)
	}
	return mapRows(rows, func(r sqlc.SearchWhiteboardNamesRow) (repository.Row, error) {
		id, place, err := hitIDs(r.ID, r.HubID)
		if err != nil {
			return repository.Row{}, err
		}
		return repository.Row{ID: id, HubID: place[0], Title: r.Title, UpdatedAt: optionalTime(r.UpdatedAt)}, nil
	})
}

// Goals answers the goals by their titles.
func (SearchNamesRepository) Goals(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchGoalNames(ctx, sqlc.SearchGoalNamesParams{HubID: hub, Terms: ask.Terms, Batch: batch})
	if err != nil {
		return nil, searchFailed("goals", err)
	}
	return mapRows(rows, func(r sqlc.SearchGoalNamesRow) (repository.Row, error) {
		id, place, err := hitIDs(r.ID, r.HubID)
		if err != nil {
			return repository.Row{}, err
		}
		return repository.Row{ID: id, HubID: place[0], Title: r.Title, State: r.Status, UpdatedAt: optionalTime(r.UpdatedAt)}, nil
	})
}

// Dashboards answers the dashboards by their names, with what decides who sees them.
func (SearchNamesRepository) Dashboards(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchDashboardNames(ctx, sqlc.SearchDashboardNamesParams{HubID: hub, Terms: ask.Terms, Batch: batch})
	if err != nil {
		return nil, searchFailed("dashboards", err)
	}
	return mapRows(rows, func(r sqlc.SearchDashboardNamesRow) (repository.Row, error) {
		id, place, err := hitIDs(r.ID, append([]pgtype.UUID{r.HubID, r.OwnerID}, r.GroupIds...)...)
		if err != nil {
			return repository.Row{}, err
		}
		return repository.Row{
			ID: id, HubID: place[0], OwnerID: place[1], GroupIDs: place[2:], Title: r.Name,
			Sharing: r.Sharing, State: r.Sharing, UpdatedAt: optionalTime(r.UpdatedAt),
		}, nil
	})
}

// Cis answers the inventory items by their names.
func (SearchNamesRepository) Cis(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchCiNames(ctx, sqlc.SearchCiNamesParams{HubID: hub, Terms: ask.Terms, Batch: batch})
	if err != nil {
		return nil, searchFailed("inventory items", err)
	}
	return mapRows(rows, func(r sqlc.SearchCiNamesRow) (repository.Row, error) {
		id, place, err := hitIDs(r.ID, r.HubID)
		if err != nil {
			return repository.Row{}, err
		}
		return repository.Row{
			ID: id, HubID: place[0], Title: r.Name, Tile: r.ClassKey, State: r.Stage, UpdatedAt: optionalTime(r.UpdatedAt),
		}, nil
	})
}

// People answers the people by their display names.
func (SearchNamesRepository) People(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, _, batch, err := searchArgs(ctx, repository.Ask{Batch: ask.Batch})
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchPeopleNames(ctx, sqlc.SearchPeopleNamesParams{Terms: ask.Terms, Batch: batch})
	if err != nil {
		return nil, searchFailed("people", err)
	}
	return mapRows(rows, func(r sqlc.SearchPeopleNamesRow) (repository.Row, error) {
		id, err := idFrom(r.ID)
		return repository.Row{
			ID: id, Title: r.DisplayName, Tile: r.Kind, State: r.Status, UpdatedAt: optionalTime(r.UpdatedAt),
		}, err
	})
}

// EntryFiles answers the files on living entries by their names.
func (SearchNamesRepository) EntryFiles(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchEntryFileNames(ctx, sqlc.SearchEntryFileNamesParams{HubID: hub, Terms: ask.Terms, Batch: batch})
	if err != nil {
		return nil, searchFailed("files", err)
	}
	return mapRows(rows, func(r sqlc.SearchEntryFileNamesRow) (repository.Row, error) {
		id, place, err := hitIDs(r.ID, r.ItemID, r.CollectionID, r.HubID)
		if err != nil {
			return repository.Row{}, err
		}
		return repository.Row{
			ID: id, OnID: place[0], CollectionID: place[1], HubID: place[2], Title: r.FileName,
			Tile: r.MimeType, UpdatedAt: optionalTime(r.CreatedAt),
		}, nil
	})
}

// EntryComments answers a person's comments on living entries by their words.
func (SearchNamesRepository) EntryComments(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchEntryComments(ctx, sqlc.SearchEntryCommentsParams{HubID: hub, Terms: ask.Terms, Batch: batch})
	if err != nil {
		return nil, searchFailed("comments", err)
	}
	return mapRows(rows, func(r sqlc.SearchEntryCommentsRow) (repository.Row, error) {
		id, place, err := hitIDs(r.ID, r.ItemID, r.CollectionID, r.HubID)
		if err != nil {
			return repository.Row{}, err
		}
		return repository.Row{
			ID: id, OnID: place[0], CollectionID: place[1], HubID: place[2], Title: r.ItemTitle,
			Text: r.Body, UpdatedAt: optionalTime(r.ChangedAt),
		}, nil
	})
}
