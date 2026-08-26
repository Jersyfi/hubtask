// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	syncrepo "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// ExportBeforeDelete writes an archive to a backup target before a rule removes anything
// (data-retention.md §6).
//
// A seam rather than the backup performer itself, so that the retention engine does not depend on
// the backup context to run - an installation with no target configured refuses `EXPORT_THEN_DELETE`
// rather than failing to sweep anything at all.
type ExportBeforeDelete interface {
	// Export writes one archive of the tenant to the target and answers the run it produced.
	//
	// The whole tenant rather than the objects going: the archive format's scope is a tenant or a
	// container (backup-restore.md §3), and an archive of "these forty entries" is not a thing it
	// can write. What §6 asks for is that the data exists somewhere before it stops existing here,
	// and a tenant archive is that with room to spare.
	Export(ctx context.Context, targetID shared.ID) (shared.ID, error)
}

// Sweeper is the engine data-retention.md §5 describes: two phases, in batches, per tenant.
//
// It is the rule-driven half. The trash and the notification history keep their own sweeps, and
// that is not a leftover: both are kinds with no marking phase - the trash is its own grace period
// and nobody can take a notification record out - so they are a cutoff and a delete, which is what
// they already were.
type Sweeper struct {
	Rules   repository.Rules
	Marking repository.Marking
	Holds   repository.LegalHolds
	Items   workrepo.Items
	// Purger is the one engine behind every removal, which is what keeps a retention hard delete
	// owing exactly what a person's purge owes: a journal entry, a tombstone, and an event per row.
	Purger  Purger
	Changes syncrepo.ChangeLog
	// Export is optional. A rule that asks for one when there is nothing to write it with is
	// refused at the act rather than silently downgraded to a deletion.
	Export ExportBeforeDelete
	Clock  clock.Clock
	IDs    clock.IDGenerator
	HLC    clock.HLCSource
	// Batch is how many objects one pass judges. A thousand by default (§5).
	Batch int
}

// Pass runs one pass of both phases for the tenant the transaction is bound to.
//
// Phase one first and phase two after it, in that order and in one pass. A rule with no grace
// period then announces and acts in the same pass, which is what "grace_days: 0" means - and one
// with a grace period announces now and acts on a pass some days later, which is what everything
// in between is for.
func (s Sweeper) Pass(ctx context.Context, actor appshared.ActorContext) (Outcome, error) {
	now := s.Clock.Now()

	rules, err := s.Rules.List(ctx)
	if err != nil {
		return Outcome{}, err
	}
	holds, err := s.Holds.Active(ctx)
	if err != nil {
		return Outcome{}, err
	}

	outcome := Outcome{Blocked: map[string]int{}}
	for _, kind := range domain.SweptKinds() {
		if !kind.Marks {
			// The trash and the notification history. Their own sweeps remove them, and a marking
			// phase on top would be a second grace period on a grace period.
			continue
		}
		marked, err := s.announce(ctx, rules, holds, kind, now)
		if err != nil {
			return outcome, err
		}
		outcome.add(marked)
	}

	acted, err := s.act(ctx, actor, holds, now)
	if err != nil {
		return outcome, err
	}
	outcome.add(acted)
	return outcome, nil
}

// announce is phase one: what is due, what may not go, and what is now on notice.
func (s Sweeper) announce(
	ctx context.Context, rules []domain.Rule, holds domain.Holds,
	kind domain.Kind, now time.Time,
) (Outcome, error) {
	applicable := rulesFor(rules, kind.Name)
	if len(applicable) == 0 {
		return Outcome{}, nil
	}

	// The loosest cutoff of any rule for this kind: whichever keeps things for the shortest time
	// catches the most, so reading with that one and deciding each entry against *its own* rule is
	// one query where a query per rule would be one per scope. Which rule an entry belongs to is
	// the domain's answer, and it is the same answer the listing gives.
	loosest := applicable[0].Cutoff(now)
	for _, rule := range applicable[1:] {
		if cutoff := rule.Cutoff(now); cutoff.After(loosest) {
			loosest = cutoff
		}
	}

	candidates, err := s.Marking.Due(ctx, kind.Anchor, loosest, s.batch())
	if err != nil {
		return Outcome{}, err
	}

	outcome := Outcome{Matched: len(candidates), Blocked: map[string]int{}}
	byRule := map[shared.ID][]repository.Candidate{}
	var order []shared.ID

	for _, candidate := range candidates {
		rule, found := domain.Effective(applicable, kind.Name, candidate.HubID, candidate.CollectionID)
		if !found || !candidate.AnchoredAt.Before(rule.Cutoff(now)) {
			// Caught by the loosest cutoff and not by its own rule's. Not a block - nothing is
			// keeping it, its period simply has not run out.
			outcome.Matched--
			continue
		}
		if reason, blocked := s.blocked(holds, candidate); blocked {
			outcome.blocked(reason)
			continue
		}
		if _, seen := byRule[rule.ID]; !seen {
			order = append(order, rule.ID)
		}
		byRule[rule.ID] = append(byRule[rule.ID], candidate)
	}

	for _, ruleID := range order {
		rule := ruleByID(applicable, ruleID)
		group := byRule[ruleID]
		ids := make([]shared.ID, 0, len(group))
		for _, candidate := range group {
			ids = append(ids, candidate.ID)
		}

		// An announcement removes nothing; the act is phase two's. What Mark answers is how many
		// it actually claimed, which is fewer than it was given when another pass got there first.
		marked, err := s.Marking.Mark(ctx, ids, rule.ID, rule.Action, s.dueAt(rule, now))
		if err != nil {
			return outcome, err
		}

		// Offline clients are told what is coming, because §6's whole point is that the object
		// says so - and because a device that learns of the announcement has the grace period to
		// react, which is what makes removing a live entry safe for a client that was away.
		for _, candidate := range group[:min(marked, len(group))] {
			if err := s.announceChange(ctx, candidate, rule, now); err != nil {
				return outcome, err
			}
		}
	}
	return outcome, nil
}

// act is phase two: what the grace period has run out on.
func (s Sweeper) act(
	ctx context.Context, actor appshared.ActorContext, holds domain.Holds, now time.Time,
) (Outcome, error) {
	due, err := s.Marking.MarkedDue(ctx, now, s.batch())
	if err != nil {
		return Outcome{}, err
	}

	outcome := Outcome{Matched: len(due), Blocked: map[string]int{}}
	if len(due) == 0 {
		return outcome, nil
	}

	// §4.6, worked from the bottom up: a parent whose children are staying is kept back and goes on
	// the pass after the last of them. Answered for the batch, because the question is about the
	// batch: "what is below this that is not going with it".
	going := make([]shared.ID, 0, len(due))
	parents := make([]shared.ID, 0, len(due))
	for _, candidate := range due {
		going = append(going, candidate.ID)
		parents = append(parents, candidate.ID)
	}
	retained, err := s.Marking.RetainedDescendants(ctx, parents, going)
	if err != nil {
		return outcome, err
	}

	byAction := map[domain.Action][]repository.Candidate{}
	for _, candidate := range due {
		if candidate.Action == domain.ActionNotifyOnly {
			// An announcement acts on nothing. The marking stands so the object keeps saying what
			// the rule would do, and `:retain` is how somebody takes it off.
			outcome.Matched--
			continue
		}
		if reason, blocked := s.blocked(holds, candidate); blocked {
			outcome.blocked(reason)
			continue
		}
		if retained[candidate.ID] > 0 {
			outcome.blocked(domain.BlockedByDescendant)
			continue
		}
		byAction[candidate.Action] = append(byAction[candidate.Action], candidate)
	}

	for _, action := range []domain.Action{
		domain.ActionArchive, domain.ActionTrash,
		domain.ActionExportThenDelete, domain.ActionHardDelete,
	} {
		group := byAction[action]
		if len(group) == 0 {
			continue
		}
		removed, err := s.perform(ctx, actor, action, group, now)
		if err != nil {
			return outcome, err
		}
		outcome.Removed += removed
	}
	return outcome, nil
}

// perform is one action against one group, and the only place an act on a retained object happens.
func (s Sweeper) perform(
	ctx context.Context, actor appshared.ActorContext, action domain.Action,
	group []repository.Candidate, now time.Time,
) (int, error) {
	ids := make([]shared.ID, 0, len(group))
	for _, candidate := range group {
		ids = append(ids, candidate.ID)
	}

	switch action {
	case domain.ActionArchive:
		if _, err := s.Marking.Archive(ctx, ids, now); err != nil {
			return 0, err
		}
	case domain.ActionTrash:
		// One batch identifier for the act, which is what makes one act one restore (F-09).
		if _, err := s.Marking.Trash(ctx, ids, s.IDs.NewID(), now); err != nil {
			return 0, err
		}
	case domain.ActionExportThenDelete:
		if err := s.exportFirst(ctx, group); err != nil {
			return 0, err
		}
		return s.remove(ctx, actor, group, now)
	case domain.ActionHardDelete:
		return s.remove(ctx, actor, group, now)
	}

	// The marking goes and the rule's claim stays: a chain's second stage is what owns the entry
	// now, and it counts from the column this stage just wrote.
	if _, err := s.Marking.Clear(ctx, ids, true, now); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// exportFirst writes the archive §6 asks for before anything is removed.
//
// One archive per target per pass rather than one per object: the archive format's scope is a
// tenant, and forty entries going in one pass is one export of the tenant they were in. A rule
// asking for an export where nothing can write one is refused rather than quietly carried out as a
// deletion - "export then delete" with the export missing is just a deletion.
func (s Sweeper) exportFirst(ctx context.Context, group []repository.Candidate) error {
	if s.Export == nil {
		return shared.ErrConflict.WithDetail(domain.CodeExportUnavailable)
	}

	written := map[shared.ID]bool{}
	for _, candidate := range group {
		rule, err := s.Rules.Find(ctx, candidate.Rule)
		if err != nil {
			return err
		}
		if rule.ExportTargetID.IsZero() {
			return shared.ErrConflict.WithDetail(domain.CodeExportTargetRequired).
				WithParams(map[string]string{"policy_id": rule.ID.String()})
		}
		if written[rule.ExportTargetID] {
			continue
		}
		if _, err := s.Export.Export(ctx, rule.ExportTargetID); err != nil {
			return err
		}
		written[rule.ExportTargetID] = true
	}
	return nil
}

// remove is a hard delete, through the one engine every removal goes through.
//
// The subtree per entry rather than the identifiers in a batch, because that is what a removal owes:
// a journal entry and a tombstone for every row that goes, and an event for each so that a media
// store and a search index can clean up what they hold for it (data-retention.md §5). A batch delete
// by identifier would take the descendants through the foreign key, uncounted and unrecorded.
//
// The offline window of §4.5 is not applied here, and that is a decision rather than an omission.
// The window exists so that a device which has not heard of a deletion cannot recreate the object;
// what a device hears here is the announcement of phase one, and the grace period between the two
// phases is the time it has to hear it. The window still bounds the trash, where the deletion was
// announced when somebody made it.
func (s Sweeper) remove(
	ctx context.Context, actor appshared.ActorContext,
	group []repository.Candidate, now time.Time,
) (int, error) {
	removed := 0
	for _, candidate := range group {
		item, err := s.Items.Find(ctx, candidate.ID)
		if err != nil {
			return removed, err
		}
		gone, err := s.Purger.Subtree(ctx, actor, item, candidate.HubID,
			domain.DeletedByRetention, now)
		if err != nil {
			return removed, err
		}
		removed += gone
	}
	return removed, nil
}

// blocked is data-retention.md §4, in its order, against one object.
//
// The restriction of §4.2 has no source yet - E-10 builds the data subject request - and its place
// in the order is here so that the task which fills it does not also have to decide where it sits.
// Until then nothing sets it, which is the honest state rather than a silent gap.
func (s Sweeper) blocked(holds domain.Holds, candidate repository.Candidate) (string, bool) {
	found := map[string]bool{}
	if _, held := holds.Blocking(domain.Target{
		ItemID:          candidate.ID,
		ContainerIDs:    nonZero(candidate.HubID, candidate.CollectionID),
		AncestorItemIDs: work.PathIDs(candidate.Path),
	}); held {
		found[domain.BlockedByLegalHold] = true
	}
	return domain.FirstBlock(found)
}

// announceChange tells offline clients what is coming.
func (s Sweeper) announceChange(
	ctx context.Context, candidate repository.Candidate, rule domain.Rule, now time.Time,
) error {
	return s.Changes.Record(ctx, syncrepo.Change{
		Entity: itemEntity, EntityID: candidate.ID, Op: syncrepo.Upsert,
		ContainerID: candidate.CollectionID, HLC: s.HLC.Next(),
		Payload: map[string]any{
			"retention": map[string]any{
				"action":       string(rule.Action),
				"effective_at": s.dueAt(rule, now),
				"policy_id":    rule.ID.String(),
			},
		},
	})
}

// dueAt is when the act falls due for something marked now: the grace period the announcement buys.
func (s Sweeper) dueAt(rule domain.Rule, now time.Time) time.Time {
	return now.AddDate(0, 0, rule.GraceDays)
}

func (s Sweeper) batch() int {
	if s.Batch > 0 {
		return s.Batch
	}
	return DefaultSweepBatch
}

// DefaultSweepBatch is the thousand objects per transaction data-retention.md §5 asks for.
const DefaultSweepBatch = 1000

func rulesFor(rules []domain.Rule, kind domain.DataKind) []domain.Rule {
	var applicable []domain.Rule
	for _, rule := range rules {
		if rule.DataKind == kind && rule.Enabled {
			applicable = append(applicable, rule)
		}
	}
	return applicable
}

func ruleByID(rules []domain.Rule, id shared.ID) domain.Rule {
	for _, rule := range rules {
		if rule.ID == id {
			return rule
		}
	}
	return domain.Rule{}
}

// add folds one phase's outcome into the pass's.
func (o *Outcome) add(other Outcome) {
	o.Matched += other.Matched
	o.Removed += other.Removed
	if o.Blocked == nil {
		o.Blocked = map[string]int{}
	}
	for reason, count := range other.Blocked {
		o.Blocked[reason] += count
	}
}
