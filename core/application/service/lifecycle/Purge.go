// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package lifecycle is where data stops existing: the hard delete, and the rules that decide when
// it is allowed (ADR-0020, data-retention.md).
//
// Trashing and archiving live in the work context, because they are changes to an entry - a stamp on
// the aggregate, reversible, with the entry still there afterwards. What lives here is the operation
// that has no aggregate left when it is done, and whose rules come from somewhere else entirely: a
// tenant's configured period, a legal obligation, and the state of every device that ever held a
// copy.
package lifecycle

import (
	"context"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The target types an audit entry and a metric name.
const (
	itemTarget      = "item"
	containerTarget = "container"
	trashTarget     = "trash"
)

// The reasons a removal can be refused, kept here as the names this package has always used and
// defined where the precedence of data-retention.md §4 is - in the domain, because "which reason
// wins" is a rule about the data rather than a detail of the engine (E-07).
const (
	BlockedByLegalHold       = domain.BlockedByLegalHold
	BlockedByTombstoneWindow = domain.BlockedByTombstoneWindow
)

// Purger removes trashed rows for good.
//
// One engine behind every path that removes: a person purging one entry, a person emptying a trash,
// and the retention job. They differ in what they select and in whether a refusal is an error or a
// number in a report - not in what removing owes, which is the whole point of it being one thing.
// A second implementation would be a second place for a purge to forget its tombstone.
type Purger struct {
	Trash    workrepo.Trash
	Expired  repository.Expired
	Holds    repository.LegalHolds
	Removals repository.Removals
	Events   outbox.Events
	Audit    audit.Sink
	Clock    clock.Clock
	IDs      clock.IDGenerator
	// TombstoneWindow is the maximum offline window: how long the marker of a removal has to
	// survive it, and the lower bound an automatic run observes before removing at all.
	TombstoneWindow time.Duration
	// BatchSize is how many rows one sweep reads. A deletion run works in batches so that a large
	// one does not hold a single transaction open across the whole of it (data-retention.md §5).
	BatchSize int
}

// Outcome is what one pass did.
type Outcome struct {
	// Matched is how many rows were past their period, whether or not they were removed.
	Matched int
	// Removed is how many actually went.
	Removed int
	// Blocked counts what was refused, by reason. The reasons are the closed set above, so this is
	// a report an operator can read and a metric can be labelled from.
	Blocked map[string]int
}

// blocked records a refusal.
func (o *Outcome) blocked(reason string) {
	if o.Blocked == nil {
		o.Blocked = map[string]int{}
	}
	o.Blocked[reason]++
}

// Subtree removes one entry and everything under it for good, and reports how many rows went.
//
// Called inside the caller's transaction, like every repository method here: the rows, the journal,
// the tombstones and the events commit together or not at all. A purge that reached the tables and
// not the journal would come back on the next restore from backup (ADR-0020 §6).
//
// A legal hold is an error here rather than a number, because the caller named this entry: somebody
// asked for one thing, and "it was skipped" is not an answer they can act on.
func (p Purger) Subtree(
	ctx context.Context, actor appshared.ActorContext, item work.WorkItem, hub shared.ID,
	reason domain.DeletionReason, now time.Time,
) (int, error) {
	holds, err := p.Holds.Active(ctx)
	if err != nil {
		return 0, err
	}
	if hold, blocked := holds.Blocking(domain.Target{
		ItemID:          item.ID,
		ContainerIDs:    nonZero(hub, item.CollectionID),
		AncestorItemIDs: work.PathIDs(item.Path),
	}); blocked {
		return 0, legalHoldRefusal(hold)
	}

	ids, err := p.Trash.SubtreeIDs(ctx, item.Path)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	removals := make([]domain.Removal, 0, len(ids))
	for _, id := range ids {
		removals = append(removals, domain.Removal{
			Entity: workItemEntity, EntityID: id, Reason: reason,
		})
	}
	if err := p.record(ctx, removals, now); err != nil {
		return 0, err
	}

	removed, err := p.Trash.PurgeItems(ctx, ids)
	if err != nil {
		return 0, err
	}

	// One event per row removed, unlike the deletion into the trash, which announces its root alone.
	// The difference is what the two are for: a client holding a subtree drops it by prefix when the
	// root goes, but a purge is the last moment a media store, a search index or a vector store can
	// clean up what it holds *for that row* - and completeness across every storage location is the
	// rule this path is judged by (data-retention.md §5). The payload carries no content, so the
	// volume is bounded and cheap.
	for _, id := range ids {
		if err := p.announce(ctx, actor, event.Purge{
			TenantID: item.TenantID, ItemID: id, Type: item.Type,
			CollectionID: item.CollectionID, Path: item.Path, Reason: reason,
		}, now); err != nil {
			return 0, err
		}
	}
	return removed, nil
}

// Selection is what one sweep is to remove.
type Selection struct {
	// Cutoff is the deletion date a row has to be older than. The retention job passes the tenant's
	// period; a person emptying a trash passes now, which is everything in it.
	Cutoff time.Time
	// Reason is why, for the journal, the events and the metric.
	Reason domain.DeletionReason
	// ObserveTombstoneWindow holds back rows deleted more recently than the maximum offline window
	// (offline-sync.md §7).
	//
	// True for the automatic paths and false for an explicit one, which is the distinction the two
	// documents together draw. A retention rule removing an object a device has not yet heard about
	// would let that device recreate it on its next push, and nobody asked for the removal to happen
	// today. A person emptying their own trash did ask, and the tombstone the purge writes is what
	// stops the resurrection there - the marker outlives the row by the whole window either way.
	ObserveTombstoneWindow bool
}

// Sweep removes everything the selection covers, in one batch, and reports what it did.
//
// One batch per call. The caller decides whether to come back for another - the retention job loops
// until a pass finds nothing, and a person emptying a trash gets one pass and a count - because that
// decision belongs with whoever holds the transaction, and a loop in here would hold one open across
// the whole of a large deletion (data-retention.md §5).
//
// A refusal is counted rather than raised. This selects rather than names, so "one of these is held"
// is a fact about the run and not a failure of it.
func (p Purger) Sweep(
	ctx context.Context, actor appshared.ActorContext, selection Selection, now time.Time,
) (Outcome, error) {
	var outcome Outcome
	cutoff, reason := selection.Cutoff, selection.Reason

	holds, err := p.Holds.Active(ctx)
	if err != nil {
		return outcome, err
	}

	// The floor offline clients impose. An object may only disappear for good once every known
	// device has had the chance to learn of the deletion; until then it is past its period and still
	// not removable, which is a state the run reports rather than hides.
	safe := now.Add(-p.TombstoneWindow)
	if !selection.ObserveTombstoneWindow {
		// An explicit act. Everything selected is old enough by definition.
		safe = now
	}

	items, err := p.Expired.Items(ctx, cutoff, p.BatchSize)
	if err != nil {
		return outcome, err
	}
	removableItems := make([]shared.ID, 0, len(items))
	for _, expired := range items {
		outcome.Matched++
		if _, held := holds.Blocking(domain.Target{
			ItemID:          expired.ID,
			ContainerIDs:    nonZero(expired.HubID, expired.CollectionID),
			AncestorItemIDs: work.PathIDs(expired.Path),
		}); held {
			outcome.blocked(BlockedByLegalHold)
			continue
		}
		if expired.DeletedAt.After(safe) {
			outcome.blocked(BlockedByTombstoneWindow)
			continue
		}
		removableItems = append(removableItems, expired.ID)
	}

	containers, err := p.Expired.Containers(ctx, cutoff, p.BatchSize)
	if err != nil {
		return outcome, err
	}
	removableContainers := make([]shared.ID, 0, len(containers))
	for _, expired := range containers {
		outcome.Matched++
		if _, held := holds.Blocking(domain.Target{
			ContainerIDs: nonZero(expired.ParentID, expired.ID),
		}); held {
			outcome.blocked(BlockedByLegalHold)
			continue
		}
		if expired.DeletedAt.After(safe) {
			outcome.blocked(BlockedByTombstoneWindow)
			continue
		}
		removableContainers = append(removableContainers, expired.ID)
	}

	removals := make([]domain.Removal, 0, len(removableItems)+len(removableContainers))
	for _, id := range removableItems {
		removals = append(removals, domain.Removal{Entity: workItemEntity, EntityID: id, Reason: reason})
	}
	for _, id := range removableContainers {
		removals = append(removals, domain.Removal{Entity: containerEntity, EntityID: id, Reason: reason})
	}
	if err := p.record(ctx, removals, now); err != nil {
		return outcome, err
	}

	// Entries before containers, and within each in the order the read handed them over: deepest
	// entry first, collections before the hubs that hold them. The database insists on the second
	// through ON DELETE RESTRICT; the first is what keeps a cascade from removing rows this run has
	// not journalled.
	removedItems, err := p.Trash.PurgeItems(ctx, removableItems)
	if err != nil {
		return outcome, err
	}
	removedContainers, err := p.Trash.PurgeContainers(ctx, removableContainers)
	if err != nil {
		return outcome, err
	}
	outcome.Removed = removedItems + removedContainers

	for _, expired := range items {
		if !contains(removableItems, expired.ID) {
			continue
		}
		if err := p.announce(ctx, actor, event.Purge{
			TenantID: actor.TenantID, ItemID: expired.ID, Type: expired.Type,
			CollectionID: expired.CollectionID, Path: expired.Path, Reason: reason,
		}, now); err != nil {
			return outcome, err
		}
	}
	return outcome, nil
}

// The tables a removal names, in the words the journal and the tombstone use.
const (
	workItemEntity  = "work_item"
	containerEntity = "container"
)

// record writes the journal entry and the tombstone for every row about to go.
//
// Before the rows are removed rather than after, so that a transaction that fails between the two
// leaves a record of a deletion that did not happen rather than a deletion with no record. The first
// is harmless - the journal names a row that is still there, and a restore that skips it skips
// nothing - and the second is the orphan the completeness rule forbids.
func (p Purger) record(ctx context.Context, removals []domain.Removal, now time.Time) error {
	if len(removals) == 0 {
		return nil
	}
	return p.Removals.Record(ctx, removals, now, now.Add(p.TombstoneWindow))
}

// announce tells the outside world that a row is gone.
func (p Purger) announce(
	ctx context.Context, actor appshared.ActorContext, purge event.Purge, now time.Time,
) error {
	announcement, err := event.NewItemPurged(
		p.IDs.NewID(), purge, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return err
	}
	return p.Events.Append(ctx, announcement)
}

// RecordAudit writes the summary a removal owes the trail.
//
// A summary rather than an entry per row, which is what data-retention.md §5 asks for: an audit that
// grew with every deleted object would grow faster than the payload data it is about. What an
// auditor needs is who removed how much, when, and why - and the journal is where the individual
// rows are, if it ever comes to that.
func (p Purger) RecordAudit(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, target string,
	targetID shared.ID, outcome Outcome, reason domain.DeletionReason, now time.Time,
) error {
	changes := []audit.Change{
		{Field: "reason", Classification: audit.Open, To: string(reason)},
		{Field: "matched", Classification: audit.Open, To: strconv.Itoa(outcome.Matched)},
		{Field: "removed", Classification: audit.Open, To: strconv.Itoa(outcome.Removed)},
	}
	for _, blocked := range []string{BlockedByLegalHold, BlockedByTombstoneWindow} {
		if count := outcome.Blocked[blocked]; count > 0 {
			changes = append(changes, audit.Change{
				Field: "blocked_" + blocked, Classification: audit.Open, To: strconv.Itoa(count),
			})
		}
	}

	return p.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		// A warning rather than a notice: this is the entry an auditor is looking for when work has
		// gone and nobody knows where. Nothing else in this system destroys data (audit.md §2).
		Severity:   audit.SeverityWarning,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: target,
		TargetID:   targetID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// legalHoldRefusal is what a caller that named one thing is told.
//
// A conflict rather than a refusal of permission: the request is well formed and the actor may
// ordinarily do this - the state is what makes it impossible, and that is the distinction that tells
// a client whether waiting or asking somebody would help (api-guidelines.md §6). The hold's reason
// does not travel: it is what an operator wrote for an auditor, not an explanation owed to whoever
// tried to delete something.
func legalHoldRefusal(hold domain.LegalHold) error {
	return shared.ErrConflict.
		WithDetail("lifecycle.legal_hold").
		WithParams(map[string]string{"scope": string(hold.Scope)})
}

// nonZero collects the identifiers that are set, in order. A collection always has a hub and an
// entry always has a collection, so this is about the rows where one of them is legitimately absent
// rather than about tolerating bad data.
func nonZero(ids ...shared.ID) []shared.ID {
	present := make([]shared.ID, 0, len(ids))
	for _, id := range ids {
		if !id.IsZero() {
			present = append(present, id)
		}
	}
	return present
}

func contains(ids []shared.ID, wanted shared.ID) bool {
	for _, id := range ids {
		if id == wanted {
			return true
		}
	}
	return false
}
