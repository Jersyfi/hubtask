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
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// Ask is one question to the store: the words, lowered, every one of which must occur (none is
// every name); an optional hub; the keys of the kind asked, `@me` already the reader; and how
// many candidates at most.
type Ask struct {
	Terms      []string
	HubID      shared.ID
	Conditions []view.KindCondition
	Batch      int
}

// Row is one candidate.
type Row struct {
	ID    shared.ID
	HubID shared.ID
	// CollectionID is a sprint's collection, or that of the entry a file or comment is on.
	CollectionID shared.ID
	// OnID is the entry, page or board a file or comment is on.
	OnID shared.ID
	// OwnerID, Sharing and GroupIDs are a dashboard's, for the question who sees it.
	OwnerID  shared.ID
	Sharing  string
	GroupIDs []shared.ID
	// Title is the name, the file name, or the title of the entry a comment is on.
	Title string
	// Tile is a class key, a media type or an account kind.
	Tile string
	// State is a goal's or a portfolio's status, a release's or a sprint's state, an item's stage
	// or an account's status.
	State string
	// Text is a comment's body, which the snippet is cut from.
	Text string
	// Key is an entry's ticket key as people write it, "SA-12".
	Key       string
	UpdatedAt *time.Time
}

// Key is a ticket key as typed: the word names a collection by its key prefix or its handle.
type Key struct {
	Word   string
	Number int64
}

// Names is the store's half of the search.
type Names interface {
	Whiteboards(ctx context.Context, ask Ask) ([]Row, error)
	Goals(ctx context.Context, ask Ask) ([]Row, error)
	Dashboards(ctx context.Context, ask Ask) ([]Row, error)
	Cis(ctx context.Context, ask Ask) ([]Row, error)
	// Portfolios ignores the hub: a portfolio is the workspace's.
	Portfolios(ctx context.Context, ask Ask) ([]Row, error)
	Releases(ctx context.Context, ask Ask) ([]Row, error)
	// Sprints names each sprint's collection and, as HubID, that collection's hub.
	Sprints(ctx context.Context, ask Ask) ([]Row, error)
	// People ignores the hub: a person stands in none.
	People(ctx context.Context, ask Ask) ([]Row, error)
	EntryFiles(ctx context.Context, ask Ask) ([]Row, error)
	EntryComments(ctx context.Context, ask Ask) ([]Row, error)
	// PageFiles and PageComments are on living pages, OnID the page; a comment's Title is the
	// page's.
	PageFiles(ctx context.Context, ask Ask) ([]Row, error)
	PageComments(ctx context.Context, ask Ask) ([]Row, error)
	// WhiteboardFiles and WhiteboardComments are on living boards, OnID the board.
	WhiteboardFiles(ctx context.Context, ask Ask) ([]Row, error)
	WhiteboardComments(ctx context.Context, ask Ask) ([]Row, error)
	// EntryByKey is the living entry a ticket key names, at most one, in the hub where one is
	// given; Tile is its level, State OPEN or DONE.
	EntryByKey(ctx context.Context, key Key, hubID shared.ID) ([]Row, error)
}
