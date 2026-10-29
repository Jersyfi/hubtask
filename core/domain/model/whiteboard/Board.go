// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package whiteboard is the suite's whiteboard module (M4): a hub's boards and the objects on them -
// sticky notes, shapes, text, connectors, frames and cards of entries.
//
// A board sits in a hub and inherits its memberships, as a page does (ADR-0073). It is an object
// every reference can point at (P-19): `hubtask:whiteboard/<id>`, and `#<frame>` names one frame
// of it. While several people draw at once the live document (realtime, kind `whiteboard`) is the
// editors' CRDT; the client saves the scene - the board's objects in drawing order - beside it, and
// the scene is what references, export and the archive read. The server never reads a CRDT update
// (offline-sync.md §13), exactly as for a page's body.
package whiteboard

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The bounds of a board.
const (
	MaxTitleLength = 200
	// MaxBoardsPerHub bounds the list a client reads whole.
	MaxBoardsPerHub = 1000
)

// DocumentKind is the live document kind a board registers (realtime.Kind).
const DocumentKind = "whiteboard"

// Board is one whiteboard of a hub.
type Board struct {
	ID        shared.ID
	TenantID  shared.ID
	HubID     shared.ID
	Title     string
	Scene     Scene
	DeletedAt *time.Time
	CreatedBy shared.ID
	CreatedAt time.Time
	UpdatedBy shared.ID
	UpdatedAt time.Time
	Version   int
}

// IsTrashed reports whether the board is in the trash.
func (b Board) IsTrashed() bool { return b.DeletedAt != nil }

// References are the objects the board points at: its cards, and the `hubtask:` links in its
// texts. A trashed board points nowhere.
func (b Board) References() []reference.Found {
	if b.IsTrashed() {
		return nil
	}
	return b.Scene.References(b.ID)
}

// NewBoardInput is what a new board is made from.
type NewBoardInput struct {
	ID, TenantID, HubID shared.ID
	Title               string
	Scene               Scene
	CreatedBy           shared.ID
	Now                 time.Time
	Text                text.Normalizer
}

// NewBoard checks a new board. The scene was checked by ParseScene.
func NewBoard(in NewBoardInput) (Board, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.HubID.IsZero() || in.CreatedBy.IsZero() {
		return Board{}, shared.ErrInternal.WithDetail("whiteboards.identity_incomplete")
	}
	title, err := CheckTitle(in.Title, in.Text)
	if err != nil {
		return Board{}, err
	}
	return Board{
		ID: in.ID, TenantID: in.TenantID, HubID: in.HubID, Title: title, Scene: in.Scene,
		CreatedBy: in.CreatedBy, CreatedAt: in.Now, UpdatedBy: in.CreatedBy, UpdatedAt: in.Now, Version: 1,
	}, nil
}

// The fields a change names, as the trail records them.
const (
	FieldTitle = "title"
	FieldScene = "scene"
)

// Change is what a write changes; a nil member is left alone.
type Change struct {
	Title *string
	Scene *Scene
}

// Changed answers the board after the change and the fields it changed. A change that changes
// nothing answers no fields, and the caller writes nothing.
func (b Board) Changed(change Change, by shared.ID, now time.Time, form text.Normalizer) (Board, []string, error) {
	next, fields := b, []string{}
	if change.Title != nil {
		title, err := CheckTitle(*change.Title, form)
		if err != nil {
			return Board{}, nil, err
		}
		if title != b.Title {
			next.Title, fields = title, append(fields, FieldTitle)
		}
	}
	if change.Scene != nil && !change.Scene.Equal(b.Scene) {
		next.Scene, fields = *change.Scene, append(fields, FieldScene)
	}
	if len(fields) > 0 {
		next.UpdatedBy, next.UpdatedAt = by, now
	}
	return next, fields, nil
}

// CheckTitle brings a title to its stored form: normal form C, trimmed, one line, bounded.
func CheckTitle(raw string, form text.Normalizer) (string, error) {
	title, err := shared.NFC(strings.TrimSpace(raw), form)
	if err != nil {
		return "", err
	}
	switch {
	case title == "":
		return "", invalid("/title", "whiteboards.title_empty")
	case utf8.RuneCountInString(title) > MaxTitleLength:
		return "", invalid("/title", "whiteboards.title_too_long").WithParams(map[string]string{"maximum": "200"})
	case strings.ContainsAny(title, "\r\n"):
		return "", invalid("/title", "whiteboards.title_malformed")
	}
	return title, nil
}

func invalid(path, code string) *shared.Error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}
