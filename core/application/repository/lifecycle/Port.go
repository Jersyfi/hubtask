// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package lifecycle declares what the end of data's life needs stored: the instructions not to
// delete, and the record of what was deleted anyway.
package lifecycle

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// LegalHolds reads the instructions that override every retention decision (data-retention.md §4.1).
type LegalHolds interface {
	// Active returns the holds in force for this tenant, released ones left out.
	//
	// The whole set rather than a question per target. A purge run judges a batch of a thousand
	// rows, and a query per row would be a thousand round trips for an answer that is the same
	// every time - the holds of a tenant are few and change rarely, and deciding against them in
	// the domain is what keeps the rule readable (lifecycle.Holds.Blocking).
	Active(ctx context.Context) (domain.Holds, error)
}

// HoldWriter places and lifts them (E-08).
//
// Its own port rather than methods on LegalHolds, for the reason the read and the write of a
// deletion are kept apart: the deletion paths take the reading half, and a port that carried both
// would let a purge reach a statement that lifts the hold stopping it.
type HoldWriter interface {
	// Place writes a hold.
	Place(ctx context.Context, hold domain.LegalHold) error

	// Find answers one hold, released or not, or ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.LegalHold, error)

	// List answers the tenant's holds, newest first. Released ones only when asked for: a list of
	// holds is read to answer "what is frozen now", and one that has been lifted is what an
	// auditor reads to see that it was.
	List(ctx context.Context, includeReleased bool) ([]domain.LegalHold, error)

	// Release lifts one, and answers false for a hold that was already lifted.
	//
	// The guard is the statement rather than a check the caller ran a moment earlier: two requests
	// lifting the same hold would otherwise both succeed, and the second would overwrite who lifted
	// it and when.
	Release(ctx context.Context, hold domain.LegalHold) (bool, error)
}

// Removals records what a hard delete leaves behind.
//
// One method writing both records rather than two that a caller has to remember to pair. They are
// not independently useful: a journal entry without a tombstone lets a device that was offline
// recreate the row on its next push, and a tombstone without a journal entry lets a restore from
// backup bring it back. Either on its own is the orphan the completeness rule forbids (ADR-0020 §6).
type Removals interface {
	// Record writes a journal entry and a tombstone for each removal, inside the caller's
	// transaction - so what was removed and the record of its removal commit together.
	//
	// Both are idempotent on the row's identity: a run that dies after writing them and is picked
	// up again writes the same records rather than failing, which is what makes the retention job
	// safe to retry at all (ADR-0008, at-least-once).
	//
	// The removals may span both tables; the adapter groups them. Grouping here rather than at every
	// call site is what keeps a purge of a hub - containers and entries in one act - a single call.
	Record(ctx context.Context, removals []domain.Removal, deletedAt, purgeAfter time.Time) error
}

// ExpiredItem is one entry whose time in the trash is up, with what deciding its removal needs.
//
// A projection rather than the aggregate: a purge does not read an entry in order to change it, and
// loading the whole of one - the notes, the completion, the labels - for a row that is about to stop
// existing would be work done to throw away.
type ExpiredItem struct {
	ID   shared.ID
	Type work.ItemType
	// Path is the chain of entries above it, which is what a legal hold placed higher up is judged
	// against, and what a purge event carries so a consumer can clean up below it (I-W2).
	Path         string
	CollectionID shared.ID
	// HubID is the hub of the collection. A hold placed on a hub has to reach an entry three levels
	// below it, and the entry alone does not know which hub it is under.
	HubID     shared.ID
	DeletedAt time.Time
}

// ExpiredContainer is one hub or collection whose time in the trash is up.
type ExpiredContainer struct {
	ID   shared.ID
	Type work.ContainerType
	// ParentID is the hub a collection sits in, and empty for a hub.
	ParentID  shared.ID
	DeletedAt time.Time
}

// Expired reads what may now be removed for good.
//
// Its own interface rather than methods on the work repositories, because the question is the
// lifecycle context's: "what is past its period" is not something a board or a list ever asks, and a
// port that mixed them would let a read path reach a statement written for a deletion.
type Expired interface {
	// Items returns entries deleted before the cutoff, deepest first, at most batchSize of them.
	//
	// Deepest first because a purge works a subtree from the bottom up (data-retention.md §4.6): a
	// parent removed before its children would take them through the foreign key, and rows removed
	// by a cascade nobody counted leave no journal entry and no tombstone behind.
	//
	// The cutoff is an instant rather than a period, so that one run reads the clock once - two
	// readings would let a long batch use two different definitions of "expired".
	Items(ctx context.Context, cutoff time.Time, batchSize int) ([]ExpiredItem, error)

	// Containers returns hubs and collections deleted before the cutoff, collections before the hubs
	// that hold them - which is the order `container.parent_id` being ON DELETE RESTRICT insists on.
	Containers(ctx context.Context, cutoff time.Time, batchSize int) ([]ExpiredContainer, error)
}

// Policies stores the periods a tenant keeps things for (ADR-0020: periods are data, not code).
type Policies interface {
	// Ensure writes the documented defaults for a tenant that has none, and leaves a tenant that has
	// decided something of its own alone.
	//
	// Called by the run rather than by a migration, because a migration covers the tenants that
	// existed when it ran and no others - the first tenant created afterwards would be one with no
	// policy at all.
	Ensure(ctx context.Context, policies []domain.Policy) error

	// Find returns the period in force for one kind, or ErrNotFound when the tenant has none. Not an
	// error the caller should ever see after Ensure, and still an answer rather than a zero value: a
	// period read as zero would be a trash emptied the moment something landed in it.
	Find(ctx context.Context, kind domain.DataKind) (domain.Policy, error)
}

// Rules stores the rule model of data-retention.md §2 (E-07).
//
// Beside Policies rather than replacing it for one release: the old table's key allows one period
// per kind per tenant and the model is scoped, so the two live alongside each other while a rolling
// update is possible, and CarryOver is what moves a tenant from one to the other.
type Rules interface {
	// Insert writes a rule. The unique index over (kind, scope) is what refuses a second rule for
	// one kind at one level, and the refusal reaches the caller as a conflict.
	Insert(ctx context.Context, rule domain.Rule) error

	// List answers every rule the tenant has, narrowest scope first - so a reader walking the list
	// meets the winner before the ones it beats.
	List(ctx context.Context) ([]domain.Rule, error)

	// Find answers one rule, or ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.Rule, error)

	// CarryOver writes the old table's period for one kind as a tenant-wide rule, and does nothing
	// for a tenant that already has one. Called by the sweep rather than by a migration, for the
	// reason Ensure is: a migration covers the tenants that existed when it ran.
	CarryOver(ctx context.Context, id shared.ID, kind domain.DataKind, now time.Time) error
}

// Candidate is one entry a retention pass is judging.
//
// A projection rather than the aggregate, for the reason ExpiredItem is one: a pass judges a
// thousand at a time and loading the whole of each would be work done to throw away.
type Candidate struct {
	ID   shared.ID
	Type work.ItemType
	// Path is the chain above it, which is what a legal hold placed higher up is judged against and
	// what the referential safeguard reads.
	Path         string
	CollectionID shared.ID
	// HubID is the hub of the collection, which a hub-scoped rule is matched against - an entry
	// three levels down does not know which hub it is under.
	HubID shared.ID
	// AnchoredAt is the value of the column this pass counted from.
	AnchoredAt time.Time
	// Title is user content and travels for exactly one reason: §5's preview shows sample objects,
	// and a sample without a title is a list of identifiers. It never reaches a log, a metric or an
	// audit entry (rule 10).
	Title string
	// Pending, Rule and Action are what a marked entry carries between the phases, and are empty on
	// a candidate that has not been marked yet.
	Pending time.Time
	Rule    shared.ID
	Action  domain.Action
	// BlockedBy is why the act is not happening, and empty when nothing is stopping it.
	BlockedBy string
}

// Marking is the two phases of data-retention.md §5 against the objects themselves.
//
// Everything here takes a batch. A pass works in batches of a thousand by design, and a method per
// row would make the transaction as long as the batch - which is the shape §5 asks for in as many
// words.
type Marking interface {
	// Due answers entries whose anchor lies before the cutoff and which are not marked yet.
	//
	// The anchor is a value of the domain's closed set rather than a column name from a caller:
	// no byte of a request becomes SQL text (rule 9), so the adapter holds one statement per
	// anchor and refuses one it has none for.
	Due(ctx context.Context, anchor domain.Anchor, cutoff time.Time, batch int) ([]Candidate, error)

	// DueInChain is the same question for a chain's second stage, restricted to the entries the
	// rule's own first stage acted on. An entry somebody archived by hand is not part of anybody's
	// chain, and a rule that swept it up would be acting outside what it matched.
	DueInChain(
		ctx context.Context, anchor domain.Anchor, ruleID shared.ID, cutoff time.Time, batch int,
	) ([]Candidate, error)

	// Mark writes what is coming, when, and under which rule. Answers how many it marked, which is
	// fewer than it was given when another pass got there first.
	Mark(
		ctx context.Context, ids []shared.ID, ruleID shared.ID,
		action domain.Action, effectiveAt time.Time,
	) (int, error)

	// Block records what a rule would do and what is stopping it (§4, §6).
	//
	// No due moment, deliberately: an entry that is held back has none, and the absence of one is
	// what keeps the second phase off it rather than a flag somebody has to remember to check. It
	// is written on every pass, because a block is a fact about now - the hold may have been
	// lifted since, and the marking then takes over.
	Block(
		ctx context.Context, ids []shared.ID, ruleID shared.ID,
		action domain.Action, reason string,
	) (int, error)

	// MarkedDue answers the entries whose grace period has run out.
	MarkedDue(ctx context.Context, now time.Time, batch int) ([]Candidate, error)

	// Marking answers one entry's marking, or ErrNotFound for an entry that has none.
	Marking(ctx context.Context, id shared.ID) (Candidate, error)

	// Clear takes entries out of the running period. keepRule is the difference between a person
	// calling `:retain` - which ends the rule's claim on the entry - and a stage that has acted,
	// after which the chain's next stage still owns it.
	Clear(ctx context.Context, ids []shared.ID, keepRule bool, now time.Time) (int, error)

	// Archive and Trash are the two acts that leave the entry in place, and the two a chain can
	// have a second stage after.
	Archive(ctx context.Context, ids []shared.ID, at time.Time) (int, error)
	Trash(ctx context.Context, ids []shared.ID, batchID shared.ID, at time.Time) (int, error)

	// RetainedDescendants is §4.6: how many entries below each of these are not going in this
	// pass. A parent with any is kept back, and goes on the pass after the last of them.
	RetainedDescendants(ctx context.Context, ids, going []shared.ID) (map[shared.ID]int, error)

	// CountDue is the numerator of §5's five-per-cent switch and of a preview: how many entries in
	// the rule's scope are past its cutoff.
	//
	// Exact rather than bounded, because the switch is about a proportion and a count that stopped
	// at a batch would under-report exactly the runs it exists to catch. It ignores narrower rules
	// inside the scope, which over-counts where one exists - and over-counting errs towards
	// NOTIFY_ONLY, which is the side to err on.
	CountDue(ctx context.Context, anchor domain.Anchor, scope domain.Scope, cutoff time.Time) (int, error)

	// CountScope is the denominator of §5's five-per-cent switch: how much the tenant holds in the
	// scope a rule covers.
	CountScope(ctx context.Context, scope domain.Scope) (int, error)
}

// RunStatus is how a retention run ended.
type RunStatus string

const (
	RunSucceeded RunStatus = "SUCCEEDED"
	RunFailed    RunStatus = "FAILED"
)

// RunResult is what one run did, as the log records it.
type RunResult struct {
	Matched    int
	Removed    int
	Blocked    map[string]int
	Status     RunStatus
	FinishedAt time.Time
}

// Runs is the log of what the retention did (data-retention.md §5).
//
// Two methods rather than one write at the end, so that a run killed halfway leaves a row saying it
// started and never finished. That is the state an operator needs to see: a deletion run that
// vanished without trace is indistinguishable from one that never started.
type Runs interface {
	// Start opens the log entry for one run.
	Start(ctx context.Context, id shared.ID, kind domain.DataKind, startedAt time.Time) error

	// Finish closes it with what the run did.
	Finish(ctx context.Context, id shared.ID, result RunResult) error
}
