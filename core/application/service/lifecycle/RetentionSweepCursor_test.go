// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A read of the rule sweep stops at its budget and keeps its place, so a workspace with more
// entries to pass over than one deadline can read is crossed in passes rather than restarted on
// each (data-retention.md §5).

// cursorStore is the place each read keeps between passes.
type cursorStore struct {
	at     map[repository.Reader]repository.Position
	writes int
	fail   error
}

func newCursorStore() *cursorStore {
	return &cursorStore{at: map[repository.Reader]repository.Position{}}
}

func (c *cursorStore) Find(_ context.Context, reader repository.Reader) (repository.Position, bool, error) {
	at, found := c.at[reader]
	return at, found, nil
}

func (c *cursorStore) Set(_ context.Context, reader repository.Reader, at repository.Position) error {
	if c.fail != nil {
		return c.fail
	}
	c.writes++
	c.at[reader] = at
	return nil
}

func (c *cursorStore) Clear(_ context.Context, reader repository.Reader) error {
	if c.fail != nil {
		return c.fail
	}
	c.writes++
	delete(c.at, reader)
	return nil
}

func entryID(i int) shared.ID {
	return shared.MustParseID(fmt.Sprintf("0192f000-0000-7000-8000-%012d", 300+i))
}

// excludedThenDue is six entries a condition excludes ahead of one it matches, oldest first.
func excludedThenDue(h *sweepHarness, engine *conditions) shared.ID {
	var due []repository.Candidate
	for i := range 7 {
		id := entryID(i)
		h.items.stored[id] = work.WorkItem{ID: id, Type: work.ItemTask}
		due = append(due, candidate(id, now.AddDate(0, 0, -400).Add(time.Duration(i)*time.Minute)))
		engine.answer[id] = i == 6
	}
	h.marking.due = due
	return entryID(6)
}

func (h *sweepHarness) budgeted(engine *conditions, cursors *cursorStore) Sweeper {
	sweeper := h.sweeper()
	sweeper.Conditions = engine
	sweeper.Cursors = cursors
	sweeper.Batch = 2
	sweeper.ReadBudget = 4
	return sweeper
}

func TestAReadStopsAtItsBudgetAndContinuesWhereItStopped(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(in *domain.NewRuleInput) { in.Condition = "item.completed_at != null" })
	engine := newConditions()
	behind := excludedThenDue(h, engine)
	cursors := newCursorStore()
	sweeper := h.budgeted(engine, cursors)
	reader := repository.Reader{Phase: repository.ReaderAnnounce, DataKind: domain.KindCompletedItem}

	// Pass one judges its budget - two pages of excluded entries - and stops there.
	first, err := sweeper.Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("pass one: %v", err)
	}
	if len(h.marking.marked) != 0 {
		t.Fatalf("pass one announced %v before reaching the due entry", h.marking.marked)
	}
	if first.Read != 4 || !first.Unfinished {
		t.Errorf("pass one read %d (unfinished %v), want its budget of 4 and unfinished", first.Read, first.Unfinished)
	}
	if at := cursors.at[reader]; at.ID != entryID(3) {
		t.Errorf("pass one kept its place at %s, want after the fourth entry %s", at.ID, entryID(3))
	}

	// Pass two continues there, reaches the due entry, and the read ends.
	second, err := sweeper.Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("pass two: %v", err)
	}
	if !slices.Contains(h.marking.marked, behind) {
		t.Fatalf("pass two did not reach the due entry behind the excluded ones: %v", h.marking.marked)
	}
	if second.Read != 3 {
		t.Errorf("pass two read %d, want the three after its place", second.Read)
	}
	// Pass one read what lies before its place, so reaching the end is the end of the cycle: the
	// place is forgotten and the next cycle starts at the beginning.
	if _, kept := cursors.at[reader]; kept {
		t.Error("a read that reached the end kept its place")
	}
	if second.Unfinished {
		t.Error("a read that reached the end reported the cycle unfinished")
	}

	// The next cycle starts at the beginning again.
	third, err := sweeper.Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("pass three: %v", err)
	}
	if at := cursors.at[reader]; third.Read != 4 || at.ID != entryID(3) {
		t.Errorf("pass three read %d and kept %s, want four from the start, kept at %s", third.Read, at.ID, entryID(3))
	}
}

// Without a place to keep, a read has no budget: stopping would starve what lies past it.
func TestWithoutCursorsAReadHasNoBudget(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(in *domain.NewRuleInput) { in.Condition = "item.completed_at != null" })
	engine := newConditions()
	behind := excludedThenDue(h, engine)
	sweeper := h.budgeted(engine, nil)
	sweeper.Cursors = nil

	outcome, err := sweeper.Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if !slices.Contains(h.marking.marked, behind) || outcome.Unfinished {
		t.Errorf("announced %v (unfinished %v), want the due entry in one pass", h.marking.marked, outcome.Unfinished)
	}
}

// A read that stops at a full batch keeps its place after the last row it judged, and the rest of
// the page is the next pass's; the full batch already brings the job back, so it is not unfinished.
func TestAReadThatFillsItsBatchKeepsItsPlace(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(*domain.NewRuleInput) {})
	var due []repository.Candidate
	for i := range 3 {
		due = append(due, candidate(entryID(i), now.AddDate(0, 0, -400).Add(time.Duration(i)*time.Minute)))
	}
	h.marking.due = due
	cursors := newCursorStore()
	sweeper := h.budgeted(newConditions(), cursors)

	outcome, err := sweeper.Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	reader := repository.Reader{Phase: repository.ReaderAnnounce, DataKind: domain.KindCompletedItem}
	if at := cursors.at[reader]; at.ID != entryID(1) {
		t.Errorf("the place is %s, want after the second entry %s", at.ID, entryID(1))
	}
	if outcome.Unfinished {
		t.Error("a full batch was reported unfinished")
	}
}

// Phase two keeps a place of its own: announcements ahead of a due act are passed in budgets.
func TestPhaseTwoContinuesWhereItStopped(t *testing.T) {
	h := newSweepHarness()
	rule := h.ruleIn(t, func(*domain.NewRuleInput) {})
	var marked []repository.Candidate
	for i := range 5 {
		action := domain.ActionNotifyOnly
		if i == 4 {
			action = domain.ActionArchive
		}
		id := entryID(i)
		marked = append(marked, repository.Candidate{
			ID: id, Type: work.ItemTask, Path: work.RootPath(id),
			CollectionID: collectionID, HubID: hubID,
			AnchoredAt: now.Add(-time.Hour).Add(time.Duration(i) * time.Minute),
			Pending:    now.Add(-time.Hour), Rule: rule.ID, Action: action,
		})
	}
	h.marking.dueMarked = marked
	cursors := newCursorStore()
	sweeper := h.budgeted(newConditions(), cursors)

	if _, err := sweeper.Pass(context.Background(), actor()); err != nil {
		t.Fatalf("pass one: %v", err)
	}
	if len(h.marking.archived) != 0 {
		t.Fatalf("pass one acted on %v before reaching it", h.marking.archived)
	}
	if at := cursors.at[repository.Reader{Phase: repository.ReaderAct}]; at.ID != entryID(3) {
		t.Errorf("phase two kept its place at %s, want %s", at.ID, entryID(3))
	}
	if _, err := sweeper.Pass(context.Background(), actor()); err != nil {
		t.Fatalf("pass two: %v", err)
	}
	if !slices.Contains(h.marking.archived, entryID(4)) {
		t.Errorf("pass two did not act on the due entry: %v", h.marking.archived)
	}
}

// A chain's second stage keeps a place per rule.
func TestAChainKeepsItsOwnPlace(t *testing.T) {
	h := newSweepHarness()
	rule := h.ruleIn(t, func(in *domain.NewRuleInput) {
		in.ThenAfterDays, in.ThenAction = 730, domain.ActionHardDelete
	})
	h.holdEach(entryID(0), entryID(1), entryID(2), entryID(3))
	var chained []repository.Candidate
	for i := range 5 {
		chained = append(chained, candidate(entryID(i), now.AddDate(0, 0, -800).Add(time.Duration(i)*time.Minute)))
	}
	h.marking.inChain = chained
	cursors := newCursorStore()
	sweeper := h.budgeted(newConditions(), cursors)

	if _, err := sweeper.Pass(context.Background(), actor()); err != nil {
		t.Fatalf("pass one: %v", err)
	}
	reader := repository.Reader{Phase: repository.ReaderChain, RuleID: rule.ID}
	if at := cursors.at[reader]; at.ID != entryID(3) {
		t.Fatalf("the chain kept its place at %s, want %s", at.ID, entryID(3))
	}
	if _, err := sweeper.Pass(context.Background(), actor()); err != nil {
		t.Fatalf("pass two: %v", err)
	}
	if !slices.Contains(h.marking.marked, entryID(4)) {
		t.Errorf("the chain's second pass did not reach the entry behind the held ones: %v", h.marking.marked)
	}
}

// A place that cannot be written fails the pass, which rolls back with it: the place describes
// work that did not commit.
func TestAPlaceThatCannotBeWrittenFailsThePass(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(in *domain.NewRuleInput) { in.Condition = "item.completed_at != null" })
	engine := newConditions()
	excludedThenDue(h, engine)
	cursors := newCursorStore()
	cursors.fail = shared.ErrUnavailable
	sweeper := h.budgeted(engine, cursors)

	if _, err := sweeper.Pass(context.Background(), actor()); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("error %v, want the store's failure", err)
	}
}

// An unfinished read brings the job straight back however little it matched, so a cycle through the
// workspace's rows runs back to back rather than one budget an hour.
func TestAnUnfinishedReadIsNotTheEnd(t *testing.T) {
	run := RunRetention{Purger: Purger{BatchSize: 2}}
	if run.Exhausted(Outcome{Unfinished: true}) {
		t.Error("an unfinished read reads as the end")
	}
	if !run.Exhausted(Outcome{}) {
		t.Error("a finished pass that matched nothing is not the end")
	}
}
