// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

// A rule whose condition cannot be evaluated on a candidate is set aside for the pass, and the
// workspace's other rules go on (data-retention.md §2, UC-LIF-04 check 10).

var (
	elsewhere  = shared.MustParseID("0192f000-0000-7000-8000-0000000000cc")
	matching   = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	failing    = shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	collected  = shared.MustParseID("0192f000-0000-7000-8000-0000000000f3")
	markedOnce = shared.MustParseID("0192f000-0000-7000-8000-0000000000f4")
	markedElse = shared.MustParseID("0192f000-0000-7000-8000-0000000000f5")
)

// setAsideHarness is a workspace with a conditioned rule for everything and an unconditioned one
// for one collection: an entry in that collection is the second rule's, the others the first's.
func setAsideHarness(t *testing.T) (*sweepHarness, domain.Rule, domain.Rule, *conditions) {
	t.Helper()
	h := newSweepHarness()
	wide := h.ruleIn(t, func(in *domain.NewRuleInput) {
		in.Condition = "item.labels[0] == 'x'"
		in.Action = domain.ActionTrash
	})
	narrow := h.ruleIn(t, func(in *domain.NewRuleInput) {
		in.ID = shared.MustParseID("0192f000-0000-7000-8000-0000000000e2")
		in.Scope = domain.Scope{Kind: domain.ScopeCollection, ID: collectionID}
	})
	outside := func(id shared.ID) repository.Candidate {
		c := candidate(id, now.AddDate(0, 0, -400))
		c.CollectionID = elsewhere
		return c
	}
	for _, id := range []shared.ID{matching, failing, collected} {
		h.items.stored[id] = work.WorkItem{ID: id, Type: work.ItemTask}
	}
	// The matching entry comes first, so the rule has collected it before its condition fails on
	// the next one.
	h.marking.due = []repository.Candidate{outside(matching), outside(failing), candidate(collected, now.AddDate(0, 0, -400))}

	engine := newConditions()
	engine.answer[matching] = true
	engine.fails = map[shared.ID]error{
		failing: shared.ErrValidation.WithDetail(expression.CodeEvaluationFailed).
			WithParams(map[string]string{"kind": "index_out_of_range"}),
	}
	return h, wide, narrow, engine
}

func TestAFailingConditionSetsItsRuleAsideAndTheOthersGoOn(t *testing.T) {
	h, wide, narrow, engine := setAsideHarness(t)
	// Marked in an earlier pass: one by the rule that fails now, one by the other.
	h.marking.dueMarked = []repository.Candidate{
		{ID: markedOnce, Type: work.ItemTask, Path: work.RootPath(markedOnce), CollectionID: elsewhere,
			HubID: hubID, Rule: wide.ID, Action: domain.ActionTrash, Pending: now.AddDate(0, 0, -1)},
		{ID: markedElse, Type: work.ItemTask, Path: work.RootPath(markedElse), CollectionID: collectionID,
			HubID: hubID, Rule: narrow.ID, Action: domain.ActionTrash, Pending: now.AddDate(0, 0, -1)},
	}

	sweeper := h.sweeper()
	sweeper.Conditions = engine
	outcome, err := sweeper.Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("one rule's condition stopped the pass: %v", err)
	}

	if !slices.Equal(h.marking.marked, []shared.ID{collected}) {
		t.Errorf("announced %v, want only the other rule's entry", h.marking.marked)
	}
	if !slices.Equal(h.marking.trashed, []shared.ID{markedElse}) {
		t.Errorf("phase two acted on %v, want only the other rule's entry", h.marking.trashed)
	}
	failure, aside := outcome.SetAside[wide.ID]
	if !aside || failure.Code != expression.CodeEvaluationFailed || !failure.At.Equal(now) {
		t.Errorf("the rule was set aside with %+v (%v)", failure, aside)
	}
	if clean := outcome.Clean(); !slices.Equal(clean, []shared.ID{narrow.ID}) {
		t.Errorf("the clean rules are %v, want only the other one", clean)
	}
}

// What is nobody's rule - the database unreadable while the entry is resolved - still fails the
// pass, to be tried again; setting the rule aside would mark it failing for an outage.
func TestAnUnavailableReadStillFailsThePass(t *testing.T) {
	h, _, _, engine := setAsideHarness(t)
	engine.fails[failing] = shared.ErrUnavailable.WithDetail("postgres.query_failed")

	sweeper := h.sweeper()
	sweeper.Conditions = engine
	_, err := sweeper.Pass(context.Background(), actor())
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("error %v, want the outage", err)
	}
}
