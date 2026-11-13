// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
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
	batch := int32(min(max(ask.Batch, 1), 1000))
	return queries, hub, batch, err
}

// bound is one key of a kind as its three parameters (SearchKinds.sql); negated is nil where the
// key is not asked.
type bound struct {
	negated *bool
	values  []string
	none    bool
}

func boundOf(conditions []view.KindCondition, field string) bound {
	for _, c := range conditions {
		if c.Field == field {
			negated := c.Negated
			values := c.Values
			if values == nil {
				values = []string{}
			}
			return bound{negated: &negated, values: values, none: c.None}
		}
	}
	return bound{values: []string{}}
}

// lowered is the values compared with a lowered column.
func (b bound) lowered() []string {
	out := make([]string, len(b.values))
	for i, v := range b.values {
		out[i] = strings.ToLower(v)
	}
	return out
}

// ids is the values as identifiers; the domain read each as one.
func (b bound) ids() ([]pgtype.UUID, error) {
	out := make([]pgtype.UUID, len(b.values))
	for i, v := range b.values {
		key, err := optionalUUID(shared.ID(v))
		if err != nil {
			return nil, err
		}
		out[i] = key
	}
	return out, nil
}

// terms is never null: no words is every name.
func terms(words []string) []string {
	if words == nil {
		return []string{}
	}
	return words
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
	rows, err := queries.SearchWhiteboardNames(ctx, sqlc.SearchWhiteboardNamesParams{HubID: hub, Terms: terms(ask.Terms), Batch: batch})
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
	state := boundOf(ask.Conditions, view.FieldGoalState)
	rows, err := queries.SearchGoalNames(ctx, sqlc.SearchGoalNamesParams{
		HubID: hub, Terms: terms(ask.Terms), Batch: batch, StateNegated: state.negated, StateValues: state.values,
	})
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
	rows, err := queries.SearchDashboardNames(ctx, sqlc.SearchDashboardNamesParams{HubID: hub, Terms: terms(ask.Terms), Batch: batch})
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
	class, stage, owner := boundOf(ask.Conditions, view.FieldAssetClass), boundOf(ask.Conditions, view.FieldAssetStage), boundOf(ask.Conditions, view.FieldAssetOwner)
	location, tag := boundOf(ask.Conditions, view.FieldAssetLocation), boundOf(ask.Conditions, view.FieldAssetTag)
	owners, err := owner.ids()
	if err != nil {
		return nil, err
	}
	locations, err := location.ids()
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchCiNames(ctx, sqlc.SearchCiNamesParams{
		HubID: hub, Terms: terms(ask.Terms), Batch: batch,
		ClassNegated: class.negated, ClassValues: class.lowered(), StageNegated: stage.negated, StageValues: stage.values,
		OwnerNegated: owner.negated, OwnerValues: owners, OwnerNone: owner.none,
		LocationNegated: location.negated, LocationValues: locations, LocationNone: location.none,
		TagNegated: tag.negated, TagValues: tag.lowered(), TagNone: tag.none,
	})
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

// Portfolios answers the workspace's portfolios by their names; a portfolio stands in no hub.
func (SearchNamesRepository) Portfolios(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, _, batch, err := searchArgs(ctx, repository.Ask{Batch: ask.Batch})
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchPortfolioNames(ctx, sqlc.SearchPortfolioNamesParams{Terms: terms(ask.Terms), Batch: batch})
	if err != nil {
		return nil, searchFailed("portfolios", err)
	}
	return mapRows(rows, func(r sqlc.SearchPortfolioNamesRow) (repository.Row, error) {
		id, err := idFrom(r.ID)
		return repository.Row{ID: id, Title: r.Name, State: r.Status, UpdatedAt: optionalTime(r.UpdatedAt)}, err
	})
}

// Releases answers the releases of living hubs by their names.
func (SearchNamesRepository) Releases(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchReleaseNames(ctx, sqlc.SearchReleaseNamesParams{HubID: hub, Terms: terms(ask.Terms), Batch: batch})
	if err != nil {
		return nil, searchFailed("releases", err)
	}
	return mapRows(rows, func(r sqlc.SearchReleaseNamesRow) (repository.Row, error) {
		id, place, err := hitIDs(r.ID, r.HubID)
		if err != nil {
			return repository.Row{}, err
		}
		return repository.Row{ID: id, HubID: place[0], Title: r.Name, State: r.State, UpdatedAt: optionalTime(r.UpdatedAt)}, nil
	})
}

// Sprints answers the sprints of living collections by their names, with the collection's hub.
func (SearchNamesRepository) Sprints(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchSprintNames(ctx, sqlc.SearchSprintNamesParams{HubID: hub, Terms: terms(ask.Terms), Batch: batch})
	if err != nil {
		return nil, searchFailed("sprints", err)
	}
	return mapRows(rows, func(r sqlc.SearchSprintNamesRow) (repository.Row, error) {
		id, place, err := hitIDs(r.ID, r.HubID, r.CollectionID)
		if err != nil {
			return repository.Row{}, err
		}
		return repository.Row{
			ID: id, HubID: place[0], CollectionID: place[1], Title: r.Name, State: r.State, UpdatedAt: optionalTime(r.UpdatedAt),
		}, nil
	})
}

// People answers the people by their display names.
func (SearchNamesRepository) People(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, _, batch, err := searchArgs(ctx, repository.Ask{Batch: ask.Batch})
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchPeopleNames(ctx, sqlc.SearchPeopleNamesParams{Terms: terms(ask.Terms), Batch: batch})
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
	rows, err := queries.SearchEntryFileNames(ctx, sqlc.SearchEntryFileNamesParams{HubID: hub, Terms: terms(ask.Terms), Batch: batch})
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
	rows, err := queries.SearchEntryComments(ctx, sqlc.SearchEntryCommentsParams{HubID: hub, Terms: terms(ask.Terms), Batch: batch})
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

// onRow is a file or comment found on a page or a board.
func onRow(id, on, hub pgtype.UUID, title, tile, text string, at pgtype.Timestamptz) (repository.Row, error) {
	own, place, err := hitIDs(id, on, hub)
	if err != nil {
		return repository.Row{}, err
	}
	return repository.Row{ID: own, OnID: place[0], HubID: place[1], Title: title, Tile: tile, Text: text, UpdatedAt: optionalTime(at)}, nil
}

// PageFiles answers the files on living pages by their names.
func (SearchNamesRepository) PageFiles(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchPageFileNames(ctx, sqlc.SearchPageFileNamesParams{HubID: hub, Terms: terms(ask.Terms), Batch: batch})
	if err != nil {
		return nil, searchFailed("page files", err)
	}
	return mapRows(rows, func(r sqlc.SearchPageFileNamesRow) (repository.Row, error) {
		return onRow(r.ID, r.PageID, r.HubID, r.FileName, r.MimeType, "", r.CreatedAt)
	})
}

// PageComments answers the comments on living pages by their words.
func (SearchNamesRepository) PageComments(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchPageComments(ctx, sqlc.SearchPageCommentsParams{HubID: hub, Terms: terms(ask.Terms), Batch: batch})
	if err != nil {
		return nil, searchFailed("page comments", err)
	}
	return mapRows(rows, func(r sqlc.SearchPageCommentsRow) (repository.Row, error) {
		return onRow(r.ID, r.PageID, r.HubID, r.PageTitle, "", r.Body, r.ChangedAt)
	})
}

// WhiteboardFiles answers the files on living boards by their names.
func (SearchNamesRepository) WhiteboardFiles(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchWhiteboardFileNames(ctx, sqlc.SearchWhiteboardFileNamesParams{HubID: hub, Terms: terms(ask.Terms), Batch: batch})
	if err != nil {
		return nil, searchFailed("whiteboard files", err)
	}
	return mapRows(rows, func(r sqlc.SearchWhiteboardFileNamesRow) (repository.Row, error) {
		return onRow(r.ID, r.WhiteboardID, r.HubID, r.FileName, r.MimeType, "", r.CreatedAt)
	})
}

// WhiteboardComments answers the comments on living boards by their words.
func (SearchNamesRepository) WhiteboardComments(ctx context.Context, ask repository.Ask) ([]repository.Row, error) {
	queries, hub, batch, err := searchArgs(ctx, ask)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchWhiteboardComments(ctx, sqlc.SearchWhiteboardCommentsParams{HubID: hub, Terms: terms(ask.Terms), Batch: batch})
	if err != nil {
		return nil, searchFailed("whiteboard comments", err)
	}
	return mapRows(rows, func(r sqlc.SearchWhiteboardCommentsRow) (repository.Row, error) {
		return onRow(r.ID, r.WhiteboardID, r.HubID, r.WhiteboardTitle, "", r.Body, r.ChangedAt)
	})
}

// EntryByKey answers the living entry a ticket key names.
func (SearchNamesRepository) EntryByKey(ctx context.Context, key repository.Key, hubID shared.ID) ([]repository.Row, error) {
	queries, hub, _, err := searchArgs(ctx, repository.Ask{HubID: hubID})
	if err != nil {
		return nil, err
	}
	rows, err := queries.SearchEntryByKey(ctx, sqlc.SearchEntryByKeyParams{Word: key.Word, Number: key.Number, HubID: hub})
	if err != nil {
		return nil, searchFailed("a key", err)
	}
	return mapRows(rows, func(r sqlc.SearchEntryByKeyRow) (repository.Row, error) {
		id, place, err := hitIDs(r.ID, r.CollectionID, r.HubID)
		if err != nil {
			return repository.Row{}, err
		}
		state := "OPEN"
		if r.IsCompleted {
			state = "DONE"
		}
		row := repository.Row{
			ID: id, CollectionID: place[0], HubID: place[1], Title: r.Title, Tile: r.Level, State: state,
			UpdatedAt: optionalTime(r.UpdatedAt),
		}
		if r.Number != nil {
			row.Key = r.KeyWord + "-" + strconv.FormatInt(*r.Number, 10)
		}
		return row, nil
	})
}
