// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package view declares how the application layer stores saved views (D-07).
//
// Its own package rather than a corner of the work port, the way meta and media have their own:
// a saved view is a bookmark over the work, not a piece of it - it names no item, joins no
// subtree, and deleting one deletes no work.
package view

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// SavedViews stores and answers saved views.
//
// Every implementation answers only the caller's tenant (ADR-0010): the unit of work opened the
// transaction with the tenant bound, and row level security applies it to every statement below.
// A row of another tenant is therefore not found rather than forbidden (multi-tenancy.md §2).
type SavedViews interface {
	// Find answers one view, whoever owns it - whether the caller may see it is the application
	// layer's decision, taken with the row in hand (ADR-0005).
	Find(ctx context.Context, id shared.ID) (view.SavedView, error)

	// ListOwned answers the account's own views, whatever their scope and sharing.
	ListOwned(ctx context.Context, ownerID shared.ID) ([]view.SavedView, error)

	// ListReachable answers the account's own views plus what is shared into the given scopes -
	// the container identifiers along one path, with TENANT-wide shares included by their type.
	// The scopes are the authorisation's answer bound into the statement, never a filter after
	// the page (C-04's rule, applied here).
	ListReachable(ctx context.Context, ownerID shared.ID, scopeIDs []shared.ID) ([]view.SavedView, error)

	Insert(ctx context.Context, saved view.SavedView) error

	// SetAttributes writes the view's own fields whole, or reports a version conflict. The
	// sharing is not among them - one decision about one field, with a method of its own.
	SetAttributes(ctx context.Context, saved view.SavedView, expectedVersion int) error

	// SetSharing writes who sees the view, or reports a version conflict.
	SetSharing(ctx context.Context, saved view.SavedView, expectedVersion int) error

	// Delete removes the view, or reports a version conflict. Hard, deliberately: a view is a
	// bookmark, and a calendar feed that served it keeps its token and loses the reference
	// (ON DELETE SET NULL, migration 0005).
	Delete(ctx context.Context, saved view.SavedView, expectedVersion int) error
}
