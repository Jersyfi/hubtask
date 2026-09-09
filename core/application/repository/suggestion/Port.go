// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package suggestion is the outbound port of what AI proposed (J-05).
//
// No method takes a tenant: every read and write is bounded by the transaction it runs in, like
// every repository (ADR-0010).
package suggestion

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

// Page is one page of suggestions, in the order they were recorded, newest first.
type Page struct {
	Items      []domain.Suggestion
	NextCursor string
	HasMore    bool
}

// Query is what a read asks for.
type Query struct {
	TargetType domain.TargetType
	TargetID   shared.ID
	// Status narrows to one state. Empty answers what is still standing, which is what a reader
	// almost always wants: an inbox of proposals, not a history of them.
	Status domain.Status
	Cursor string
	Size   int
}

// Suggestions is the store.
type Suggestions interface {
	// Record writes one proposal. Produced by a job rather than by a request, so there is no
	// idempotency key here - a job that runs twice records twice, and the second is a duplicate
	// proposal somebody dismisses rather than a duplicate change somebody has to undo. That is
	// the whole safety of a suggestion being a record.
	Record(ctx context.Context, proposal domain.Suggestion) error

	// Find answers one suggestion, or an error wrapping shared.ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.Suggestion, error)

	// List answers one page.
	List(ctx context.Context, query Query) (Page, error)

	// Decide writes an answered suggestion, guarded on the version it was read at. False means
	// the version moved under the caller, which is two people answering one proposal.
	Decide(ctx context.Context, decided domain.Suggestion, expectedVersion int) (bool, error)
}

// Expiring is the slice the retention engine removes through (E-07).
//
// The same two methods as the jumble, the notification history and the outbox, and deliberately the
// same shape: the engine treats a fifth kind exactly as it treats the second. Declared here rather
// than reached for, so that what retention can do to a suggestion is visible in one place - it can
// count what is due and remove a batch of it, and it cannot read one, write one or decide one.
type Expiring interface {
	// DeleteExpired removes up to batch suggestions recorded before the cutoff, oldest first.
	DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error)
	// CountExpired reports how many are due, counted no higher than ceiling.
	CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error)
}
