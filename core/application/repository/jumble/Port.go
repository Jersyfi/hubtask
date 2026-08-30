// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package jumble is the repository port of the inbox (G-10, domain-model.md §2).
package jumble

import (
	"context"

	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Query narrows a listing.
type Query struct {
	// Status and Channel are zero for "any", which is what an absent query parameter means.
	Status  domain.Status
	Channel domain.Channel
	Cursor  string
	Size    int
}

// Page is one page of the jumble and where the walk stands, the shape every listing here has
// (api-guidelines.md §4).
type Page struct {
	Entries    []domain.Entry
	NextCursor string
	HasMore    bool
}

// Entries stores and finds the jumble.
//
// It judges nothing. Whether the caller may read or settle an entry is the use case's question
// (ADR-0005, rule 2); what belongs here is the one race the table can decide better than any
// read-then-write: Settle matches only a NEW row, so two conversions racing produce one item and
// one refusal rather than two items.
type Entries interface {
	// Insert writes a new arrival.
	Insert(ctx context.Context, entry domain.Entry) error

	// Find returns one entry, or an error wrapping shared.ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.Entry, error)

	// List returns a page, newest first.
	List(ctx context.Context, query Query) (Page, error)

	// Settle writes the decision an aggregate already made - PROCESSED with its target, or
	// DISMISSED - and reports whether this call was the one that decided. False means another
	// settlement got there first, which the caller answers as the conflict it is.
	Settle(ctx context.Context, entry domain.Entry) (bool, error)
}
