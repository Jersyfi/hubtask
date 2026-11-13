// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package search is the port of the search over every kind besides entries and pages (UC-WRK-18
// check 9): the candidates whose name, or a comment's words, hold every word asked for. The
// rows name their place; which of them a reader may read is the application's question (rule 2).
package search

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Ask is one question to the store: the words, lowered, every one of which must occur; an
// optional hub; and how many candidates at most.
type Ask struct {
	Terms []string
	HubID shared.ID
	Batch int
}

// Row is one candidate.
type Row struct {
	ID    shared.ID
	HubID shared.ID
	// CollectionID is the collection of the entry a file or comment is on.
	CollectionID shared.ID
	// OnID is the entry a file or comment is on.
	OnID shared.ID
	// OwnerID, Sharing and GroupIDs are a dashboard's, for the question who sees it.
	OwnerID  shared.ID
	Sharing  string
	GroupIDs []shared.ID
	// Title is the name, the file name, or the title of the entry a comment is on.
	Title string
	// Tile is a class key, a media type or an account kind.
	Tile string
	// State is a goal's status, an item's stage or an account's status.
	State string
	// Text is a comment's body, which the snippet is cut from.
	Text      string
	UpdatedAt *time.Time
}

// Names is the store's half of the search.
type Names interface {
	Whiteboards(ctx context.Context, ask Ask) ([]Row, error)
	Goals(ctx context.Context, ask Ask) ([]Row, error)
	Dashboards(ctx context.Context, ask Ask) ([]Row, error)
	Cis(ctx context.Context, ask Ask) ([]Row, error)
	// People ignores the hub: a person stands in none.
	People(ctx context.Context, ask Ask) ([]Row, error)
	EntryFiles(ctx context.Context, ask Ask) ([]Row, error)
	EntryComments(ctx context.Context, ask Ask) ([]Row, error)
}
