// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package automation is the repository port of the rules (G-05, automation.md §1).
package automation

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
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

// RunQuery narrows a listing of runs.
type RunQuery struct {
	// RuleID, Status and Trigger are zero for "any", which is what an absent query parameter means.
	RuleID  shared.ID
	Status  domain.RunStatus
	Trigger domain.TriggerKind
	Cursor  string
	Size    int
}

// RunPage is one page of runs and where the walk stands.
type RunPage struct {
	Runs       []domain.Run
	NextCursor string
	HasMore    bool
}

// Runs is the log of what the rules have done (G-07, automation.md §2).
//
// A run outlives the rule that produced it, which is why deleting a rule is soft: a record of
// actions nobody can account for would be worse than the rule staying visible.
type Runs interface {
	// Start writes a run in the RUNNING state, before any condition is evaluated. Before rather
	// than after, because a row written when a run ends loses exactly the runs somebody needs to
	// see - a process that dies mid-run leaves RUNNING behind.
	Start(ctx context.Context, run domain.Run) error

	// Finish writes how the run ended, whichever way it ended.
	Finish(ctx context.Context, run domain.Run) error

	// Find returns one run, or an error wrapping shared.ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.Run, error)

	// List returns a page of the tenant's runs, newest first.
	List(ctx context.Context, query RunQuery) (RunPage, error)

	// CountSince is what the throttle asks: how often this rule has run in the window. Runs the
	// throttle itself held back are not counted - a rule held back did not run, and counting the
	// refusals would make the bound tighten on itself.
	CountSince(ctx context.Context, ruleID shared.ID, since time.Time) (int, error)
}

// Failures is the consecutive-failure counter the table has carried since phase 0
// (automation.md §2).
//
// Its own interface rather than three more methods on Rules, because the writer that manages rules
// and the engine that runs them are different callers with different rights - and a use case that
// only writes rules should not be handed the ability to switch one off behind a person's back.
type Failures interface {
	// Bump records one more consecutive failure and answers the count afterwards.
	//
	// Answered by the same statement that incremented it, because the decision that follows is made
	// on that value: a read after the write is a second statement another run can commit between,
	// and two runs failing together would each see the other's count and neither would act.
	Bump(ctx context.Context, ruleID shared.ID, at time.Time) (int, error)

	// Clear ends the streak. One success resets it rather than decrementing, because
	// `consecutive` is what the counter means.
	Clear(ctx context.Context, ruleID shared.ID, at time.Time) error

	// Disable switches a rule off because it kept failing, and reports whether it changed
	// anything. False means somebody or something got there first, which is not an error.
	Disable(ctx context.Context, ruleID shared.ID, threshold int, at time.Time) (bool, error)
}

// Schedules is what one tenant's schedule pass asks (G-08, automation.md §1.1).
//
// Its own interface rather than three more methods on Rules, for Failures' reason: a pass that
// fires schedules has no business being able to write a rule's definition, and the one field it
// does move - the next moment - is not part of the definition at all.
//
// The tenant is the transaction's, never a parameter. A pass is opened under one tenant's scope by
// that tenant's own poller, and nothing in this system may enumerate tenants (multi-tenancy.md
// §2.1) - so the leader cannot see a tenant's schedules even if it wanted to.
type Schedules interface {
	// Due answers this tenant's enabled SCHEDULE rules whose moment has come, oldest first, at
	// most limit of them. The bound is what turns a backlog - a worker that was down for a week -
	// into several rounds rather than a hundred jobs in one transaction.
	Due(ctx context.Context, at time.Time, limit int) ([]domain.Rule, error)

	// NextDue is the earliest moment this tenant owes anything, and the zero time when it owes
	// nothing - which is what lets the poller finish instead of spinning.
	NextDue(ctx context.Context) (time.Time, error)

	// SetNextRun moves one rule on. The zero time stores none, which is a rule whose recurrence is
	// exhausted: it stays, visible and editable, and fires no more.
	SetNextRun(ctx context.Context, id shared.ID, at time.Time) error
}

// Occurrences is what a RELATIVE_DATE rule owes its entries (G-08, automation.md §1.1).
//
// The tenant is the transaction's throughout, like every other repository here.
type Occurrences interface {
	// Upsert writes or moves the moment one rule owes for one entry. One statement rather than a
	// delete and an insert, because "the due date moved" is one fact: two statements would leave a
	// window in which the tenant owed nothing.
	Upsert(ctx context.Context, occurrence domain.Occurrence) error

	// Forget is the anchor being cleared: this rule owes this entry nothing.
	Forget(ctx context.Context, ruleID, itemID shared.ID) error

	// ForgetItem is the entry going. Every rule's moment for it goes with it.
	ForgetItem(ctx context.Context, itemID shared.ID) error

	// ClaimDue takes the moments that have come and removes them in the same statement. The row
	// *is* the debt: once the run is queued the tenant no longer owes it, and a status column
	// would be a second place for "already fired" to be recorded.
	ClaimDue(ctx context.Context, at time.Time, limit int) ([]domain.Occurrence, error)

	// NextOccurrence is the earliest moment this tenant owes an occurrence, and the zero time for
	// none. Its own name rather than NextDue, because one repository answers both this and the
	// schedules' - and two methods called the same thing on one type would be one of them.
	NextOccurrence(ctx context.Context) (time.Time, error)
}

// Matching is what the dispatcher asks per event: the enabled rules whose trigger is this type.
//
// Narrow rather than part of Rules, for Failures' reason: a subscriber running inside the
// dispatcher's transaction has no business being able to write one.
type Matching interface {
	ForEventType(ctx context.Context, eventType event.Type) ([]domain.Rule, error)

	// ByTriggerKind is what a producer that is not the event dispatcher asks: this tenant's
	// enabled rules of one kind. The relative-date producer asks it, and it is on this interface
	// rather than on Rules for the same reason - a subscriber running inside the dispatcher's
	// transaction has no business being able to write one.
	ByTriggerKind(ctx context.Context, kind domain.TriggerKind) ([]domain.Rule, error)
}
