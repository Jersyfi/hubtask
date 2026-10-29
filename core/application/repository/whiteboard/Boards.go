// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package whiteboard is the storage port of the whiteboard module's boards (M4). Every method runs
// inside the caller's transaction, under the tenant it names (rule 3); another workspace's board is
// not found.
package whiteboard

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
)

// Summary is a board without its scene, as a list or a reference shows it.
type Summary struct {
	Board    whiteboard.Board
	Elements int
}

// Pointing is a board that points at an object, with how it draws it.
type Pointing struct {
	ID       shared.ID
	HubID    shared.ID
	Title    string
	Strength string
}

// VersionRow is a version as a history lists it: without its scene.
type VersionRow struct {
	Seq       int
	Title     string
	AuthorID  shared.ID
	Name      string
	CreatedAt time.Time
	Elements  int
}

// Boards stores boards.
type Boards interface {
	Insert(ctx context.Context, board whiteboard.Board) error
	// Find answers a board, trashed ones included.
	Find(ctx context.Context, id shared.ID) (whiteboard.Board, error)
	FindForUpdate(ctx context.Context, id shared.ID) (whiteboard.Board, error)
	// Update writes a board at the version expected; another version is a conflict.
	Update(ctx context.Context, board whiteboard.Board, expected int) error
	// List answers a hub's boards without their scenes, the trashed ones or the others.
	List(ctx context.Context, hubID shared.ID, trashed bool, limit int) ([]Summary, error)
	Count(ctx context.Context, hubID shared.ID) (int, error)
	// Referencing answers the boards that point at an object, at most limit.
	Referencing(ctx context.Context, targetKind string, targetID shared.ID, limit int) ([]Pointing, error)
	// Summaries answers the boards of these identifiers that are not trashed.
	Summaries(ctx context.Context, ids []shared.ID) ([]Summary, error)
	// LastVersion answers a board's newest version; found false when it has none.
	LastVersion(ctx context.Context, boardID shared.ID) (whiteboard.Version, bool, error)
	InsertVersion(ctx context.Context, version whiteboard.Version) error
	// FoldVersion rewrites a version's state in place: a save of the same drawing session.
	FoldVersion(ctx context.Context, version whiteboard.Version) error
	NameVersion(ctx context.Context, boardID shared.ID, seq int, name string) error
	// Versions answers a board's history newest first, before a place (0 for the newest).
	Versions(ctx context.Context, boardID shared.ID, before, limit int) ([]VersionRow, error)
	Version(ctx context.Context, boardID shared.ID, seq int) (whiteboard.Version, error)
	// AttachMedia places a media object on a board (UC-CNV-08); placing it twice is once.
	AttachMedia(ctx context.Context, boardID, mediaID shared.ID) error
	CountMedia(ctx context.Context, boardID shared.ID) (int, error)
}
