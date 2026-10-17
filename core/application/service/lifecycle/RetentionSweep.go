// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/application/condition"
	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	syncrepo "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	notification "github.com/Jersyfi/hubtask/core/application/service/notification"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
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

// RetentionWarner is the slice of the notification context the sweep needs (R-1): tell the
// people a rule's audience names that this entry is about to be acted on.
//
// Declared here rather than imported, so that what the retention engine can do to somebody's inbox
// is visible in one place: it can warn about a marked entry, and it cannot read a notification,
// send one, or write one about anything else.
type RetentionWarner interface {
	Warn(ctx context.Context, warning notification.RetentionWarning) error
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
	// Entries reads a page's entries in one statement, the first time a condition on that page
	// asks for `item`. Optional: without it every candidate is read on its own through Items, which
	// is what a page of a thousand candidates costs a thousand statements for (data-retention.md §5).
	Entries workrepo.ItemBatch
	// Purger is the one engine behind every removal, which is what keeps a retention hard delete
	// owing exactly what a person's purge owes: a journal entry, a tombstone, and an event per row.
	Purger Purger
	// Warnings sends the advance warning of data-retention.md §6 (R-1). Optional: a build
	// without it marks and acts exactly as before, which is what an installation that has switched
	// its notifications off already looks like - and the marking is the visibility §6 asks for
	// first, so silence here loses the message rather than the warning.
	Warnings RetentionWarner
	// Conditions compiles a rule's expression, and it is the same port and the same language the
	// automation rules use (ADR-0009). Optional: a build with none refuses to act on a
	// conditioned rule rather than acting on all of it, which is the safe direction for a pass
	// whose job is deleting.
	Conditions expression.Compiler
	Changes    syncrepo.ChangeLog
	// Export is optional. A rule that asks for one when there is nothing to write it with is
	// refused at the act rather than silently downgraded to a deletion.
	Export ExportBeforeDelete
	Clock  clock.Clock
	IDs    clock.IDGenerator
	HLC    clock.HLCSource
	// Batch is how many objects one pass judges. A thousand by default (§5).
	Batch int
	// Cursors keeps where each read stopped, so the next pass continues there (§5). Optional: a
	// build without it reads from the start on every pass and without a budget, which is what it
	// did before - a budget without a place to continue from would starve what lies past it.
	Cursors repository.Cursors
	// ReadBudget is how many rows one read judges in a pass before it stops and keeps its place:
	// what keeps a pass inside the job's deadline however many entries the read has to pass over.
	// DefaultReadBudgetPages batches by default.
	ReadBudget int
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
		marked, err := s.announce(ctx, actor, rules, holds, kind, now)
		if err != nil {
			return outcome, err
		}
		outcome.add(marked)

		chained, err := s.announceChains(ctx, rules, holds, kind, now, outcome.SetAside)
		if err != nil {
			return outcome, err
		}
		outcome.add(chained)
	}

	acted, err := s.act(ctx, actor, holds, now, outcome.SetAside)
	if err != nil {
		return outcome, err
	}
	outcome.add(acted)
	return outcome, nil
}

// announce is phase one: what is due, what may not go, and what is now on notice.
func (s Sweeper) announce(
	ctx context.Context, actor appshared.ActorContext, rules []domain.Rule, holds domain.Holds,
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

	outcome := Outcome{Blocked: map[string]int{}}
	// One compilation per rule per pass rather than one per candidate. A pass judges a thousand
	// entries against a handful of rules, and compiling is the expensive half.
	programs := map[shared.ID]expression.Program{}
	byRule := map[shared.ID][]repository.Candidate{}
	var order []shared.ID
	markable := 0

	// Page by page until the batch holds what can be marked, or the rows end. An entry a hold, a
	// condition or a narrower rule's period keeps stays in the set the read answers, oldest first,
	// so a batch-full of them read alone would hide every due entry behind it on every pass. Nothing
	// read here leaves the set before the marking below, so the pages stay where they were.
	place, err := s.begin(ctx, repository.Reader{Phase: repository.ReaderAnnounce, DataKind: kind.Name})
	if err != nil {
		return Outcome{}, err
	}
	ended := false
	for after := place.start; ; {
		candidates, err := s.Marking.Due(ctx, kind.Anchor, loosest, after, s.batch())
		if err != nil {
			return Outcome{}, err
		}
		contributors, err := contributorsOf(ctx, s.Holds, holds, candidateIDs(candidates))
		if err != nil {
			return Outcome{}, err
		}
		blocks := pageBlocks{}
		entries := &pageEntries{batch: s.Entries, ids: candidateIDs(candidates)}

		for _, candidate := range candidates {
			if markable >= s.batch() {
				break
			}
			outcome.Read++
			place.judged(candidate)
			rule, found := domain.Effective(applicable, kind.Name, candidate.HubID, candidate.CollectionID)
			if !found || !candidate.AnchoredAt.Before(rule.Cutoff(now)) {
				// Caught by the loosest cutoff and not by its own rule's. Not a block - nothing is
				// keeping it, its period simply has not run out - and not matched either.
				continue
			}
			if _, aside := outcome.SetAside[rule.ID]; aside {
				continue
			}
			matches, err := s.matchesCondition(ctx, rule, candidate, now, programs, entries)
			if err != nil {
				code, rulesFault := conditionFailure(err)
				if !rulesFault {
					return Outcome{}, err
				}
				// The rule is set aside for this pass: what it collected so far is dropped, so it
				// announces nothing, and every other rule goes on (data-retention.md §2).
				outcome.setAside(rule.ID, domain.RuleFailure{Code: code, At: now})
				markable -= len(byRule[rule.ID])
				delete(byRule, rule.ID)
				continue
			}
			outcome.judged(rule.ID)
			if !matches {
				// Not a block and not a hold: the rule simply does not apply to this entry, and it
				// is not matched, for the same reason as the cutoff case above.
				continue
			}
			outcome.Matched++
			if reason, blocked := s.blocked(holds, contributors, candidate); blocked {
				outcome.blocked(reason)
				// Held as well as matched: a block sets no due moment, so the next pass reads this
				// entry again, and a batch of them is no reason to come straight back (Exhausted).
				outcome.Held++
				// The object is told what is stopping it. §6's whole argument is that retention
				// nobody can see surprises somebody, and "this would have been deleted and a legal
				// hold is holding it back" is the half of it a count in a run's report cannot say.
				blocks.add(rule, reason, candidate.ID)
				continue
			}
			if _, seen := byRule[rule.ID]; !seen {
				order = append(order, rule.ID)
			}
			byRule[rule.ID] = append(byRule[rule.ID], candidate)
			markable++
		}
		// One statement per rule and reason on the page rather than one per entry: a read-past
		// over a held collection would otherwise cost an update per row it reads.
		if err := blocks.flush(ctx, s.Marking); err != nil {
			return outcome, err
		}
		if markable >= s.batch() {
			break
		}
		if lastPage(s.batch(), candidates) {
			ended = true
			break
		}
		if s.atBudget(place) {
			break
		}
		after = place.last
	}
	if outcome.Unfinished, err = s.settle(ctx, place, ended); err != nil {
		return outcome, err
	}

	for _, ruleID := range order {
		if _, aside := outcome.SetAside[ruleID]; aside {
			continue
		}
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
			if err := s.announceStage(ctx, candidate, rule, rule.Action, now); err != nil {
				return outcome, err
			}
			// And the people who can answer it (§6, R-1). At the marking rather than some days
			// later: this is the first moment there is anything true to say, and `notify.
			// before_days` bounds how *late* a warning may be - a rule asking for seven days'
			// notice gets the grace period's fourteen, which is not less than it asked for.
			if err := s.warn(ctx, actor, rule, candidate); err != nil {
				return outcome, err
			}
		}
	}
	return outcome, nil
}

// pageBlocks gathers the blocks of one page by rule and reason, in the order they were met, so
// they are written in one statement each.
type pageBlocks struct {
	keys []blockKey
	ids  map[blockKey][]shared.ID
}

type blockKey struct {
	rule   shared.ID
	action domain.Action
	reason string
}

func (b *pageBlocks) add(rule domain.Rule, reason string, id shared.ID) {
	key := blockKey{rule: rule.ID, action: rule.Action, reason: reason}
	if b.ids == nil {
		b.ids = map[blockKey][]shared.ID{}
	}
	if _, seen := b.ids[key]; !seen {
		b.keys = append(b.keys, key)
	}
	b.ids[key] = append(b.ids[key], id)
}

func (b *pageBlocks) flush(ctx context.Context, marking repository.Marking) error {
	for _, key := range b.keys {
		if _, err := marking.Block(ctx, b.ids[key], key.rule, key.action, key.reason); err != nil {
			return err
		}
	}
	return nil
}

// warn tells the rule's audience about one marked entry, and does nothing for a rule that names
// nobody or a build wired without the notification path.
func (s Sweeper) warn(
	ctx context.Context, actor appshared.ActorContext, rule domain.Rule,
	candidate repository.Candidate,
) error {
	if s.Warnings == nil || rule.Notify.Silent() {
		return nil
	}
	return s.Warnings.Warn(ctx, notification.RetentionWarning{
		// The pass's tenant rather than the rule's. A rule read back from the repository carries
		// no tenant - it does not need one, because row level security bounds the read to the
		// transaction's (ADR-0010) - and the one thing a notification cannot be written without is
		// exactly that.
		TenantID: actor.TenantID,
		ItemID:   candidate.ID,
		// The entry's chain, which is what the administrators are resolved along: a role held on
		// the hub administers the collection under it.
		Path: []identity.Scope{
			identity.TenantScope(),
			identity.HubScope(candidate.HubID),
			identity.CollectionScope(candidate.CollectionID),
		},
		Recipients: rule.Notify.Recipients,
	})
}

// announceChains is phase one for a chain's second stage.
//
// Its own pass rather than a branch inside the first one, because it asks a different question of a
// different column: the first stage asks "what has been finished long enough", and this asks "what
// did *this rule* archive long enough ago". The restriction to the rule's own work is what keeps a
// chain from sweeping up an entry somebody archived by hand - that entry is not part of anybody's
// chain, and a rule acting on it would be acting outside what it matched.
func (s Sweeper) announceChains(
	ctx context.Context, rules []domain.Rule, holds domain.Holds,
	kind domain.Kind, now time.Time, setAside map[shared.ID]domain.RuleFailure,
) (Outcome, error) {
	outcome := Outcome{Blocked: map[string]int{}}

	for _, rule := range rulesFor(rules, kind.Name) {
		if !rule.Chained() {
			continue
		}
		if _, aside := setAside[rule.ID]; aside {
			// A rule set aside announces nothing in this pass, its second stage included.
			continue
		}
		anchor, ok := rule.StageAnchor(kind, true)
		if !ok {
			// A chain after an action that leaves nothing. The domain refuses one, so this is a
			// rule written before the check existed rather than one somebody just made.
			continue
		}

		// Read past held entries as announce does, and for its reason: they stay in the set.
		var ids []shared.ID
		var kept []repository.Candidate
		place, err := s.begin(ctx, repository.Reader{Phase: repository.ReaderChain, RuleID: rule.ID})
		if err != nil {
			return outcome, err
		}
		ended := false
		for after := place.start; ; {
			candidates, err := s.Marking.DueInChain(
				ctx, anchor, rule.ID, rule.ThenCutoff(now), after, s.batch())
			if err != nil {
				return outcome, err
			}
			contributors, err := contributorsOf(ctx, s.Holds, holds, candidateIDs(candidates))
			if err != nil {
				return outcome, err
			}
			for _, candidate := range candidates {
				if len(ids) >= s.batch() {
					break
				}
				outcome.Read++
				place.judged(candidate)
				outcome.Matched++
				if reason, blocked := s.blocked(holds, contributors, candidate); blocked {
					outcome.blocked(reason)
					outcome.Held++
					continue
				}
				ids = append(ids, candidate.ID)
				kept = append(kept, candidate)
			}
			if len(ids) >= s.batch() {
				break
			}
			if lastPage(s.batch(), candidates) {
				ended = true
				break
			}
			if s.atBudget(place) {
				break
			}
			after = place.last
		}
		unfinished, err := s.settle(ctx, place, ended)
		if err != nil {
			return outcome, err
		}
		outcome.Unfinished = outcome.Unfinished || unfinished
		if len(ids) == 0 {
			continue
		}

		marked, err := s.Marking.Mark(ctx, ids, rule.ID, rule.ThenAction, s.dueAt(rule, now))
		if err != nil {
			return outcome, err
		}
		for _, candidate := range kept[:min(marked, len(kept))] {
			if err := s.announceStage(ctx, candidate, rule, rule.ThenAction, now); err != nil {
				return outcome, err
			}
		}
	}
	return outcome, nil
}

// act is phase two: what the grace period has run out on.
func (s Sweeper) act(
	ctx context.Context, actor appshared.ActorContext, holds domain.Holds, now time.Time,
	setAside map[shared.ID]domain.RuleFailure,
) (Outcome, error) {
	outcome := Outcome{Blocked: map[string]int{}}
	byAction := map[domain.Action][]repository.Candidate{}
	acting := 0

	// Page by page, as phase one reads: an announcement, a held entry and a parent waiting for its
	// child all keep their marking and stay first in the read, so a batch-full of them read alone
	// would hide every due entry behind it. Nothing leaves the set before the acts below.
	place, err := s.begin(ctx, repository.Reader{Phase: repository.ReaderAct})
	if err != nil {
		return outcome, err
	}
	ended := false
	for after := place.start; ; {
		due, err := s.Marking.MarkedDue(ctx, now, after, s.batch())
		if err != nil {
			return outcome, err
		}

		// §4.6, worked from the bottom up: a parent whose children are staying is kept back and
		// goes on the pass after the last of them. Answered for the page, because the question is
		// about it: "what is below this that is not going with it" - a child due on a later page
		// counts as staying, which keeps its parent for one pass more and never takes it early.
		going := make([]shared.ID, 0, len(due))
		for _, candidate := range due {
			going = append(going, candidate.ID)
		}
		retained, err := s.Marking.RetainedDescendants(ctx, going, going)
		if err != nil {
			return outcome, err
		}
		contributors, err := contributorsOf(ctx, s.Holds, holds, going)
		if err != nil {
			return outcome, err
		}

		for _, candidate := range due {
			if acting >= s.batch() {
				break
			}
			outcome.Read++
			place.judged(candidate)
			if _, aside := setAside[candidate.Rule]; aside {
				// Marked by a rule this pass set aside: its entries are neither announced nor
				// removed until it evaluates again (data-retention.md §2). The marking stands.
				continue
			}
			if candidate.Action == domain.ActionNotifyOnly {
				// An announcement acts on nothing. The marking stands so the object keeps saying
				// what the rule would do, and `:retain` is how somebody takes it off.
				continue
			}
			outcome.Matched++
			// Both keep their marking, so the next pass reads them again: held, not a reason to
			// come straight back (Exhausted). The descendant counts here though the trash pass does
			// not count it, because here the parent is read again on every pass until its child
			// goes.
			if reason, blocked := s.blocked(holds, contributors, candidate); blocked {
				outcome.blocked(reason)
				outcome.Held++
				continue
			}
			if retained[candidate.ID] > 0 {
				outcome.blocked(domain.BlockedByDescendant)
				outcome.Held++
				continue
			}
			byAction[candidate.Action] = append(byAction[candidate.Action], candidate)
			acting++
		}
		if acting >= s.batch() {
			break
		}
		if lastPage(s.batch(), due) {
			ended = true
			break
		}
		if s.atBudget(place) {
			break
		}
		after = place.last
	}
	if outcome.Unfinished, err = s.settle(ctx, place, ended); err != nil {
		return outcome, err
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
		// One batch identifier for the act, which is what makes one act one restore.
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
// Only the legal hold is consulted: the restriction of processing (§4 item 2) has no source this
// sweep reads. Its place in the order is here, after the hold.

// matchesCondition answers whether this rule's condition holds for this entry.
//
// A rule with no condition matches everything its scope and period already selected, which is what a
// retention rule has always meant - and costs nothing, because the expression is never compiled.
//
// The entry is read only for a rule that has one, and only for the candidates that got this far.
// That is the whole point of the port's lazy activation: `item` is a query, and a pass over a
// thousand candidates must not make a thousand of them for rules that never ask.
//
// A condition that cannot be evaluated does not default either way. Defaulting to true would delete
// what the condition was written to protect; defaulting to false would quietly retain everything
// and look like a working rule. The error goes back, and the caller sets the rule aside for the pass
// where it is the condition's own (conditionFailure).
func (s Sweeper) matchesCondition(
	ctx context.Context, rule domain.Rule, candidate repository.Candidate, now time.Time,
	programs map[shared.ID]expression.Program, entries *pageEntries,
) (bool, error) {
	if rule.Condition == "" {
		return true, nil
	}
	if s.Conditions == nil {
		return false, shared.ErrInternal.WithDetail("lifecycle.expression_engine_unavailable")
	}

	program, compiled := programs[rule.ID]
	if !compiled {
		var err error
		if program, err = s.Conditions.Compile(
			rule.Condition, condition.RetentionEnvironment(), expression.Boolean); err != nil {
			// The rule was compiled when it was written, so this is a rule stored by a build whose
			// environment differed - a name since withdrawn, say. Refused rather than ignored:
			// acting on a rule whose condition this build cannot read is acting on a rule nobody
			// wrote.
			return false, err
		}
		programs[rule.ID] = program
	}

	out, err := program.Evaluate(ctx, &candidateValues{
		sweeper: s, candidate: candidate, now: now, entries: entries,
	})
	if err != nil {
		return false, err
	}
	return out.Bool, nil
}

// candidateValues resolves what a retention condition may name, and reads the entry only if the
// expression asks for it.
type candidateValues struct {
	sweeper   Sweeper
	candidate repository.Candidate
	now       time.Time
	entries   *pageEntries
}

func (v *candidateValues) Resolve(ctx context.Context, name string) (any, bool, error) {
	switch name {
	case condition.VarNow:
		// The pass's instant rather than a fresh reading: an expression evaluated twice in one pass
		// sees one moment (automation.md §1.2).
		return v.now, true, nil
	case condition.VarItem:
		item, found, err := v.entries.find(ctx, v.candidate.ID)
		if err != nil {
			return nil, false, err
		}
		if !found {
			// No batch reader, or the entry left the page's answer: one read, and its not-found
			// is the answer it always was.
			if item, err = v.sweeper.Items.Find(ctx, v.candidate.ID); err != nil {
				return nil, false, err
			}
		}
		return condition.ItemDocument(item), true, nil
	case condition.VarTenant:
		// Declared and empty. The workspace's settings are not something this pass reads, and an
		// empty document is the honest shape: a condition asking for a setting gets absent rather
		// than a failure.
		return map[string]any{"settings": map[string]any{}}, true, nil
	default:
		return nil, false, nil
	}
}

// pageEntries is one page's entries, read in one statement the first time a condition asks for one,
// and never for a page whose conditions do not name `item`.
type pageEntries struct {
	batch  workrepo.ItemBatch
	ids    []shared.ID
	loaded map[shared.ID]work.WorkItem
}

func (p *pageEntries) find(ctx context.Context, id shared.ID) (work.WorkItem, bool, error) {
	if p == nil || p.batch == nil {
		return work.WorkItem{}, false, nil
	}
	if p.loaded == nil {
		items, err := p.batch.FindMany(ctx, p.ids)
		if err != nil {
			return work.WorkItem{}, false, err
		}
		p.loaded = make(map[shared.ID]work.WorkItem, len(items))
		for _, item := range items {
			p.loaded[item.ID] = item
		}
	}
	item, found := p.loaded[id]
	return item, found, nil
}

// Until then nothing sets it, which is the honest state rather than a silent gap.
func (s Sweeper) blocked(
	holds domain.Holds, contributors map[shared.ID][]shared.ID, candidate repository.Candidate,
) (string, bool) {
	found := map[string]bool{}
	if _, held := holds.Blocking(domain.Target{
		ItemID:          candidate.ID,
		ContainerIDs:    nonZero(candidate.HubID, candidate.CollectionID),
		AncestorItemIDs: work.PathIDs(candidate.Path),
		Contributors:    contributors[candidate.ID],
	}); held {
		found[domain.BlockedByLegalHold] = true
	}
	return domain.FirstBlock(found)
}

// announceStage tells offline clients what is coming, and which stage announced it.
func (s Sweeper) announceStage(
	ctx context.Context, candidate repository.Candidate,
	rule domain.Rule, action domain.Action, now time.Time,
) error {
	return s.Changes.Record(ctx, syncrepo.Change{
		Entity: itemEntity, EntityID: candidate.ID, Op: syncrepo.Upsert,
		ContainerID: candidate.CollectionID, HLC: s.HLC.Next(),
		Payload: map[string]any{
			"retention": map[string]any{
				"action":       string(action),
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

// place follows one read across a pass: where it began, the last row it judged and how many it
// judged.
type place struct {
	reader repository.Reader
	start  repository.Position
	last   repository.Position
	read   int
}

func (p *place) judged(candidate repository.Candidate) {
	p.read++
	p.last = repository.PositionOf(candidate)
}

// begin answers where a read starts: where the last pass left it, or the start.
func (s Sweeper) begin(ctx context.Context, reader repository.Reader) (*place, error) {
	p := &place{reader: reader}
	if s.Cursors == nil {
		return p, nil
	}
	at, found, err := s.Cursors.Find(ctx, reader)
	if err != nil {
		return nil, err
	}
	if found {
		p.start, p.last = at, at
	}
	return p, nil
}

// atBudget reports whether a read has judged what one pass may: it stops between pages, so a read
// judges at most its budget and one page more.
func (s Sweeper) atBudget(p *place) bool {
	return s.Cursors != nil && p.read >= s.readBudget()
}

// settle records where a read stopped, in the pass's own transaction - a pass that rolls back
// leaves the place it started from, as it leaves its work undone - and reports whether the read is
// unfinished: stopped at its budget. A read that reached the end has gone through its rows once -
// the rows before its place were read by the passes that brought it there - so it forgets its place
// and the next cycle starts at the beginning, after the long interval; a row before the place that
// became actionable since is found then. A read that stopped at a full batch keeps its place, after
// the last row it judged.
func (s Sweeper) settle(ctx context.Context, p *place, ended bool) (bool, error) {
	if s.Cursors == nil {
		return false, nil
	}
	if ended {
		if p.start.IsStart() {
			return false, nil
		}
		return false, s.Cursors.Clear(ctx, p.reader)
	}
	if p.read == 0 {
		return false, nil
	}
	return s.atBudget(p), s.Cursors.Set(ctx, p.reader, p.last)
}

func (s Sweeper) readBudget() int {
	if s.ReadBudget > 0 {
		return s.ReadBudget
	}
	return DefaultReadBudgetPages * s.batch()
}

// DefaultReadBudgetPages is how many batches one read judges per pass by default: ten thousand
// rows, which the slowest read measured (a condition naming `item`) judges in a few seconds - well
// inside the job's deadline with every reader of a pass beside it.
const DefaultReadBudgetPages = 10

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

// conditionFailure answers whether an error from a condition is the condition's own - it does not
// compile in this build, or it failed on this entry's values - and its code. Those set the rule
// aside. Anything else - the database unreadable while resolving `item`, an engine not wired - is
// nobody's rule, and fails the pass as before, to be tried again.
func conditionFailure(err error) (string, bool) {
	var coded *shared.Error
	if !errors.Is(err, shared.ErrValidation) || !errors.As(err, &coded) || coded.DetailCode == "" {
		return "", false
	}
	return coded.DetailCode, true
}

// setAside records that a rule is set aside for this pass, the first failure being the one shown.
func (o *Outcome) setAside(ruleID shared.ID, failure domain.RuleFailure) {
	if o.SetAside == nil {
		o.SetAside = map[shared.ID]domain.RuleFailure{}
	}
	if _, already := o.SetAside[ruleID]; !already {
		o.SetAside[ruleID] = failure
	}
}

// judged records that a rule's condition held or not on a candidate without failing.
func (o *Outcome) judged(ruleID shared.ID) {
	if o.Judged == nil {
		o.Judged = map[shared.ID]bool{}
	}
	o.Judged[ruleID] = true
}

// Clean is the rules the pass judged and did not set aside.
func (o Outcome) Clean() []shared.ID {
	clean := make([]shared.ID, 0, len(o.Judged))
	for ruleID := range o.Judged {
		if _, aside := o.SetAside[ruleID]; !aside {
			clean = append(clean, ruleID)
		}
	}
	return clean
}

// add folds one phase's outcome into the pass's.
func (o *Outcome) add(other Outcome) {
	for ruleID, failure := range other.SetAside {
		o.setAside(ruleID, failure)
	}
	for ruleID := range other.Judged {
		o.judged(ruleID)
	}
	o.Unfinished = o.Unfinished || other.Unfinished
	o.Matched += other.Matched
	o.Held += other.Held
	o.Read += other.Read
	o.Removed += other.Removed
	if o.Blocked == nil {
		o.Blocked = map[string]int{}
	}
	for reason, count := range other.Blocked {
		o.Blocked[reason] += count
	}
}
