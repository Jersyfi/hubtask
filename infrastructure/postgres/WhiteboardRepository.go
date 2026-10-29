// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// WhiteboardRepository stores the whiteboard module's boards and their history (migrations 0233,
// 0234), through the unit of work's transaction (rule 3).
type WhiteboardRepository struct{}

// NewWhiteboardRepository answers the store.
func NewWhiteboardRepository() WhiteboardRepository { return WhiteboardRepository{} }

var _ repository.Boards = WhiteboardRepository{}

func boardFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// sceneElement is an object as the `scene` column stores it: the contract's shape (schema
// WhiteboardElement), which the trigger of migration 0233 reads for `kind`, `text` and `target`.
type sceneElement struct {
	ID         string       `json:"id"`
	Kind       string       `json:"kind"`
	X          float64      `json:"x"`
	Y          float64      `json:"y"`
	Width      float64      `json:"width"`
	Height     float64      `json:"height"`
	Text       string       `json:"text,omitempty"`
	ColorToken string       `json:"color_token,omitempty"`
	Shape      string       `json:"shape,omitempty"`
	From       string       `json:"from,omitempty"`
	To         string       `json:"to,omitempty"`
	Route      string       `json:"route,omitempty"`
	Start      string       `json:"start,omitempty"`
	End        string       `json:"end,omitempty"`
	Target     *sceneTarget `json:"target,omitempty"`
	Locked     bool         `json:"locked,omitempty"`
	Points     []float64    `json:"points,omitempty"`
	Pen        string       `json:"pen,omitempty"`
	URL        string       `json:"url,omitempty"`
	Group      string       `json:"group,omitempty"`
}

type sceneTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// EncodeScene is the scene as the column and the archive store it.
func EncodeScene(scene whiteboard.Scene) ([]byte, error) {
	out := make([]sceneElement, len(scene.Elements))
	for i, e := range scene.Elements {
		out[i] = sceneElement{
			ID: e.ID, Kind: string(e.Kind), X: e.X, Y: e.Y, Width: e.Width, Height: e.Height, Text: e.Text,
			ColorToken: string(e.Colour), Shape: string(e.Shape), From: e.From, To: e.To, Route: string(e.Route),
			Start: string(e.Start), End: string(e.End), Locked: e.Locked, Pen: string(e.Pen), URL: e.URL, Group: e.Group,
		}
		for _, point := range e.Points {
			out[i].Points = append(out[i].Points, point.X, point.Y)
		}
		if e.Kind == whiteboard.KindCard {
			out[i].Target = &sceneTarget{Kind: string(e.Target.Kind), ID: e.Target.ID.String()}
		}
	}
	return json.Marshal(out)
}

// DecodeScene reads a stored scene.
func DecodeScene(raw []byte) (whiteboard.Scene, error) {
	var stored []sceneElement
	if err := json.Unmarshal(raw, &stored); err != nil {
		return whiteboard.Scene{}, shared.ErrInternal.WithDetail("postgres.scene_unreadable").WithCause(err)
	}
	scene := whiteboard.Scene{Elements: make([]whiteboard.Element, len(stored))}
	for i, e := range stored {
		scene.Elements[i] = whiteboard.Element{
			ID: e.ID, Kind: whiteboard.Kind(e.Kind), X: e.X, Y: e.Y, Width: e.Width, Height: e.Height, Text: e.Text,
			Colour: shared.LabelToken(e.ColorToken), Shape: whiteboard.Shape(e.Shape), From: e.From, To: e.To,
			Route: whiteboard.Route(e.Route), Start: whiteboard.Head(e.Start), End: whiteboard.Head(e.End), Locked: e.Locked,
			Pen: whiteboard.Pen(e.Pen), URL: e.URL, Group: e.Group,
		}
		for j := 0; j+1 < len(e.Points); j += 2 {
			scene.Elements[i].Points = append(scene.Elements[i].Points, whiteboard.Point{X: e.Points[j], Y: e.Points[j+1]})
		}
		if e.Target != nil {
			id, err := shared.ParseID(e.Target.ID)
			if err != nil {
				return whiteboard.Scene{}, shared.ErrInternal.WithDetail("postgres.scene_unreadable").WithCause(err)
			}
			scene.Elements[i].Target = reference.Reference{Kind: reference.Kind(e.Target.Kind), ID: id}
		}
	}
	return scene, nil
}

// Insert writes a new board.
func (WhiteboardRepository) Insert(ctx context.Context, board whiteboard.Board) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(board.ID, board.HubID, board.CreatedBy)
	if err != nil {
		return err
	}
	scene, err := EncodeScene(board.Scene)
	if err != nil {
		return boardFailed("encoding the scene", err)
	}
	if err := queries.InsertWhiteboard(ctx, sqlc.InsertWhiteboardParams{
		ID: ids[0], HubID: ids[1], Title: board.Title, Scene: scene, CreatedBy: ids[2], At: timestampOf(board.CreatedAt),
	}); err != nil {
		return boardFailed("writing the board", err)
	}
	return nil
}

// Find answers a board, trashed ones included.
func (WhiteboardRepository) Find(ctx context.Context, id shared.ID) (whiteboard.Board, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return whiteboard.Board{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return whiteboard.Board{}, shared.ErrNotFound.WithDetail("whiteboards.not_found")
	}
	row, err := queries.FindWhiteboard(ctx, key)
	if err != nil {
		return whiteboard.Board{}, boardReadError(err)
	}
	return boardFrom(row)
}

// FindForUpdate answers a board and locks it.
func (WhiteboardRepository) FindForUpdate(ctx context.Context, id shared.ID) (whiteboard.Board, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return whiteboard.Board{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return whiteboard.Board{}, shared.ErrNotFound.WithDetail("whiteboards.not_found")
	}
	row, err := queries.FindWhiteboardForUpdate(ctx, key)
	if err != nil {
		return whiteboard.Board{}, boardReadError(err)
	}
	return boardFrom(sqlc.FindWhiteboardRow(row))
}

// Update writes a board at the version expected.
func (WhiteboardRepository) Update(ctx context.Context, board whiteboard.Board, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(board.ID, board.UpdatedBy)
	if err != nil {
		return err
	}
	scene, err := EncodeScene(board.Scene)
	if err != nil {
		return boardFailed("encoding the scene", err)
	}
	rows, err := queries.UpdateWhiteboard(ctx, sqlc.UpdateWhiteboardParams{
		Title: board.Title, Scene: scene, DeletedAt: optionalTimestamp(board.DeletedAt), UpdatedBy: ids[1],
		UpdatedAt: timestampOf(board.UpdatedAt), ID: ids[0],
		ExpectedVersion: int32(expected), //nolint:gosec // a row's version
	})
	if err != nil {
		return boardFailed("writing the board", err)
	}
	if rows == 0 {
		return shared.ErrVersionConflict.WithDetail("whiteboards.version_conflict")
	}
	return nil
}

// List answers a hub's boards without their scenes.
func (WhiteboardRepository) List(ctx context.Context, hubID shared.ID, trashed bool, limit int) ([]repository.Summary, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListHubWhiteboards(ctx, sqlc.ListHubWhiteboardsParams{
		HubID: hub, Trashed: trashed, Batch: int32(limit), //nolint:gosec // bounded by whiteboard.MaxBoardsPerHub
	})
	if err != nil {
		return nil, boardFailed("listing the boards", err)
	}
	return mapRows(rows, func(row sqlc.ListHubWhiteboardsRow) (repository.Summary, error) {
		board, err := boardFrom(sqlc.FindWhiteboardRow{
			ID: row.ID, HubID: row.HubID, Title: row.Title, Scene: []byte("[]"), DeletedAt: row.DeletedAt,
			CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
			Version: row.Version,
		})
		return repository.Summary{Board: board, Elements: int(row.Elements)}, err
	})
}

// Count answers how many boards a hub holds outside the trash.
func (WhiteboardRepository) Count(ctx context.Context, hubID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountHubWhiteboards(ctx, hub)
	if err != nil {
		return 0, boardFailed("counting the boards", err)
	}
	return int(count), nil
}

// Referencing answers the boards that point at an object.
func (WhiteboardRepository) Referencing(ctx context.Context, targetKind string, targetID shared.ID, limit int) ([]repository.Pointing, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	target, err := uuidOf(targetID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.WhiteboardsReferencing(ctx, sqlc.WhiteboardsReferencingParams{
		TargetKind: targetKind, TargetID: target, Batch: int32(limit), //nolint:gosec // bounded by the caller
	})
	if err != nil {
		return nil, boardFailed("reading the boards pointing at an object", err)
	}
	return mapRows(rows, func(row sqlc.WhiteboardsReferencingRow) (repository.Pointing, error) {
		ids, err := pageIDs(row.ID, row.HubID)
		if err != nil {
			return repository.Pointing{}, err
		}
		return repository.Pointing{ID: ids[0], HubID: ids[1], Title: row.Title, Strength: row.Strength}, nil
	})
}

// Summaries answers the boards of these identifiers that are not trashed.
func (WhiteboardRepository) Summaries(ctx context.Context, ids []shared.ID) ([]repository.Summary, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := pageUUIDs(ids...)
	if err != nil {
		return nil, err
	}
	rows, err := queries.WhiteboardsByID(ctx, keys)
	if err != nil {
		return nil, boardFailed("reading the boards", err)
	}
	return mapRows(rows, func(row sqlc.WhiteboardsByIDRow) (repository.Summary, error) {
		ids, err := pageIDs(row.ID, row.HubID)
		if err != nil {
			return repository.Summary{}, err
		}
		return repository.Summary{Board: whiteboard.Board{ID: ids[0], HubID: ids[1], Title: row.Title}, Elements: int(row.Elements)}, nil
	})
}

func boardReadError(err error) error {
	if IsNoRows(err) {
		return shared.ErrNotFound.WithDetail("whiteboards.not_found")
	}
	return boardFailed("reading the board", err)
}

func boardFrom(row sqlc.FindWhiteboardRow) (whiteboard.Board, error) {
	ids, err := pageIDs(row.ID, row.HubID, row.CreatedBy, row.UpdatedBy)
	if err != nil {
		return whiteboard.Board{}, err
	}
	scene, err := DecodeScene(row.Scene)
	if err != nil {
		return whiteboard.Board{}, err
	}
	return whiteboard.Board{
		ID: ids[0], HubID: ids[1], Title: row.Title, Scene: scene, DeletedAt: optionalTime(row.DeletedAt),
		CreatedBy: ids[2], CreatedAt: timeFrom(row.CreatedAt), UpdatedBy: ids[3], UpdatedAt: timeFrom(row.UpdatedAt),
		Version: int(row.Version),
	}, nil
}

// LastVersion answers a board's newest version.
func (WhiteboardRepository) LastVersion(ctx context.Context, boardID shared.ID) (whiteboard.Version, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return whiteboard.Version{}, false, err
	}
	key, err := uuidOf(boardID)
	if err != nil {
		return whiteboard.Version{}, false, err
	}
	row, err := queries.LastWhiteboardVersion(ctx, key)
	if IsNoRows(err) {
		return whiteboard.Version{}, false, nil
	}
	if err != nil {
		return whiteboard.Version{}, false, boardFailed("reading the last version", err)
	}
	version, err := boardVersionFrom(boardID, sqlc.FindWhiteboardVersionRow(row))
	return version, err == nil, err
}

// InsertVersion writes a new version.
func (WhiteboardRepository) InsertVersion(ctx context.Context, version whiteboard.Version) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	board, author, scene, err := versionParts(version)
	if err != nil {
		return err
	}
	if err := queries.InsertWhiteboardVersion(ctx, sqlc.InsertWhiteboardVersionParams{
		WhiteboardID: board, Seq: int32(version.Seq), Title: version.Title, Scene: scene, //nolint:gosec // a version's place
		AuthorID: author, At: timestampOf(version.CreatedAt),
	}); err != nil {
		return boardFailed("writing the version", err)
	}
	return nil
}

// FoldVersion rewrites a version's state in place.
func (WhiteboardRepository) FoldVersion(ctx context.Context, version whiteboard.Version) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	board, author, scene, err := versionParts(version)
	if err != nil {
		return err
	}
	if err := queries.FoldWhiteboardVersion(ctx, sqlc.FoldWhiteboardVersionParams{
		Title: version.Title, Scene: scene, AuthorID: author, At: timestampOf(version.CreatedAt),
		WhiteboardID: board, Seq: int32(version.Seq), //nolint:gosec // a version's place
	}); err != nil {
		return boardFailed("folding the version", err)
	}
	return nil
}

// NameVersion names a version, or takes its name away.
func (WhiteboardRepository) NameVersion(ctx context.Context, boardID shared.ID, seq int, name string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	board, err := uuidOf(boardID)
	if err != nil {
		return err
	}
	rows, err := queries.NameWhiteboardVersion(ctx, sqlc.NameWhiteboardVersionParams{
		WhiteboardID: board, Seq: int32(seq), Name: name, //nolint:gosec // a version's place
	})
	if err != nil {
		return boardFailed("naming the version", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("whiteboards.version_not_found")
	}
	return nil
}

// Versions answers a board's history newest first, before a place (0 for the newest).
func (WhiteboardRepository) Versions(ctx context.Context, boardID shared.ID, before, limit int) ([]repository.VersionRow, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	board, err := uuidOf(boardID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListWhiteboardVersions(ctx, sqlc.ListWhiteboardVersionsParams{
		WhiteboardID: board, Before: int32(before), Batch: int32(limit), //nolint:gosec // bounded by the use case
	})
	if err != nil {
		return nil, boardFailed("listing the versions", err)
	}
	return mapRows(rows, func(row sqlc.ListWhiteboardVersionsRow) (repository.VersionRow, error) {
		author, err := optionalID(row.AuthorID)
		return repository.VersionRow{
			Seq: int(row.Seq), Title: row.Title, AuthorID: author, Name: row.Name,
			CreatedAt: timeFrom(row.CreatedAt), Elements: int(row.Elements),
		}, err
	})
}

// Version answers one version with its scene.
func (WhiteboardRepository) Version(ctx context.Context, boardID shared.ID, seq int) (whiteboard.Version, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return whiteboard.Version{}, err
	}
	board, err := uuidOf(boardID)
	if err != nil {
		return whiteboard.Version{}, shared.ErrNotFound.WithDetail("whiteboards.version_not_found")
	}
	row, err := queries.FindWhiteboardVersion(ctx, sqlc.FindWhiteboardVersionParams{WhiteboardID: board, Seq: int32(seq)}) //nolint:gosec // a version's place
	if IsNoRows(err) {
		return whiteboard.Version{}, shared.ErrNotFound.WithDetail("whiteboards.version_not_found")
	}
	if err != nil {
		return whiteboard.Version{}, boardFailed("reading the version", err)
	}
	return boardVersionFrom(boardID, row)
}

func versionParts(version whiteboard.Version) (board, author pgtype.UUID, scene []byte, err error) {
	if board, err = uuidOf(version.BoardID); err != nil {
		return board, author, nil, err
	}
	if author, err = optionalUUID(version.AuthorID); err != nil {
		return board, author, nil, err
	}
	if scene, err = EncodeScene(version.Scene); err != nil {
		return board, author, nil, boardFailed("encoding the scene", err)
	}
	return board, author, scene, nil
}

func boardVersionFrom(boardID shared.ID, row sqlc.FindWhiteboardVersionRow) (whiteboard.Version, error) {
	author, err := optionalID(row.AuthorID)
	if err != nil {
		return whiteboard.Version{}, err
	}
	scene, err := DecodeScene(row.Scene)
	if err != nil {
		return whiteboard.Version{}, err
	}
	return whiteboard.Version{
		BoardID: boardID, Seq: int(row.Seq), Title: row.Title, Scene: scene, AuthorID: author, Name: row.Name,
		CreatedAt: timeFrom(row.CreatedAt),
	}, nil
}
