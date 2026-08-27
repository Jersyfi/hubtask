// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package automation is the repository port of the rules (G-05, automation.md §1).
package automation

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Page is one page of rules and where the walk stands, the shape every listing here has
// (api-guidelines.md §4).
type Page struct {
	Rules      []domain.Rule
	NextCursor string
	HasMore    bool
}

// Query narrows a listing.
type Query struct {
	// Enabled is nil for "either", which is what an absent query parameter means. A bool would
	// make "not asked" and "asked for the disabled ones" the same request.
	Enabled *bool
	Cursor  string
	Size    int
}

// Rules stores and finds the automation rules.
//
// Every read hides a deleted rule. The deletion is soft so that the runs a rule produced stay
// readable - a run log whose rule vanished would be a record of actions nobody can account for -
// and not so that the rule goes on being found.
//
// It judges nothing. Whether the writer may write a rule at this scope, whether the `run_as`
// account may do what the rule asks, and whether an action kind exists at all are the use case's
// questions (ADR-0005, rule 2), so the rules stay where they can be tested without a database.
type Rules interface {
	// Insert writes a new rule, switched off.
	Insert(ctx context.Context, rule domain.Rule) error

	// Find returns one rule, or an error wrapping shared.ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.Rule, error)

	// List returns a page of the tenant's rules, newest first.
	List(ctx context.Context, query Query) (Page, error)

	// Update writes the whole definition and bumps the version, refusing when the expected version
	// is not the current one.
	//
	// The guard is in the statement rather than in a read-then-write: a check in the application
	// layer is a check something else can commit between, and two writers that both read version 3
	// would both find it current. A conflict comes back as an error wrapping shared.ErrConflict.
	Update(ctx context.Context, rule domain.Rule, expectedVersion int) error

	// SetEnabled switches a rule on or off, and nothing else - the two are different acts with
	// different audit entries, and one statement carrying both would let an edit switch a rule on
	// as a side effect of changing its name.
	SetEnabled(ctx context.Context, id shared.ID, enabled bool, expectedVersion int, at time.Time) error

	// Delete stamps the rule and reports whether it changed anything. False means it was already
	// deleted, which is not an error - the second call is somebody making sure.
	Delete(ctx context.Context, id shared.ID, at time.Time) (bool, error)
}
