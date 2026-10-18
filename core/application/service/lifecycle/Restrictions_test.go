// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	privacy "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// restrictions is the privacy read, in memory.
type restrictions struct {
	people []privacy.RestrictedPerson
	err    error
	asked  int
}

func (r *restrictions) RestrictedPeople(context.Context) ([]privacy.RestrictedPerson, error) {
	r.asked++
	return r.people, r.err
}

var colleagueID = shared.MustParseID("0192f000-0000-7000-8000-0000000000c7")

func restrictedAccount() *restrictions {
	return &restrictions{people: []privacy.RestrictedPerson{{AccountID: accountID}}}
}

// A rule does not touch what a restricted person contributed (UC-PRV-04 check 8): the entry is not
// announced, it is told a restriction holds it, and the pass counts it held. One restricted
// contributor is enough, however many others the entry has.
func TestARestrictionKeepsAnEntryBackFromARule(t *testing.T) {
	for name, contributors := range map[string][]shared.ID{
		"the restricted person alone":          {accountID},
		"a restricted and an unrestricted one": {colleagueID, accountID},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSweepHarness()
			h.ruleIn(t, func(*domain.NewRuleInput) {})
			h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -400))}
			h.holds.contributors = map[shared.ID][]shared.ID{taskID: contributors}
			sweeper := h.sweeper()
			sweeper.Restrictions = restrictedAccount()

			outcome, err := sweeper.Pass(t.Context(), actor())
			if err != nil {
				t.Fatalf("sweeping: %v", err)
			}
			if len(h.marking.marked) != 0 {
				t.Error("a restricted person's entry was announced")
			}
			if outcome.Blocked[domain.BlockedByRestriction] != 1 || outcome.Held != 1 {
				t.Errorf("the pass reported %+v, held %d", outcome.Blocked, outcome.Held)
			}
			if len(h.marking.blocked) != 1 || h.marking.blocked[0] != taskID {
				t.Errorf("the entry was not told what keeps it: %+v", h.marking.blocked)
			}
		})
	}
}

// Nobody restricted among the contributors: the rule acts as before.
func TestAnEntryOfUnrestrictedPeopleIsAnnounced(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(*domain.NewRuleInput) {})
	h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -400))}
	h.holds.contributors = map[shared.ID][]shared.ID{taskID: {colleagueID}}
	sweeper := h.sweeper()
	sweeper.Restrictions = restrictedAccount()

	if _, err := sweeper.Pass(t.Context(), actor()); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if len(h.marking.marked) != 1 {
		t.Errorf("an entry nobody restricted contributed to was not announced: %+v", h.marking.marked)
	}
}

// A legal hold outranks a restriction (data-retention.md §4's order): an entry both keep is reported
// as held by the hold.
func TestAHoldOutranksARestriction(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(*domain.NewRuleInput) {})
	h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -400))}
	h.holds.contributors = map[shared.ID][]shared.ID{taskID: {accountID}}
	h.holds.holds = domain.Holds{{ID: holdID, Scope: domain.HoldTenant, Reason: "Litigation", PlacedAt: now}}
	sweeper := h.sweeper()
	sweeper.Restrictions = restrictedAccount()

	outcome, err := sweeper.Pass(t.Context(), actor())
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if outcome.Blocked[domain.BlockedByLegalHold] != 1 || outcome.Blocked[domain.BlockedByRestriction] != 0 {
		t.Errorf("the pass reported %+v, want the hold", outcome.Blocked)
	}
}

// Phase two: an entry marked before its person was restricted is not acted on while the
// restriction stands, and is acted on by the first pass after the lift. The restriction is read
// once per pass, so a lift during a pass counts from the next one.
func TestAMarkedEntryWaitsForTheRestrictionAndGoesAfterTheLift(t *testing.T) {
	h := newSweepHarness()
	rule := h.ruleIn(t, func(*domain.NewRuleInput) {})
	h.marking.dueMarked = []repository.Candidate{{
		ID: taskID, Type: candidate(taskID, now).Type, Path: candidate(taskID, now).Path,
		CollectionID: collectionID, HubID: hubID,
		Pending: now.Add(-time.Hour), Rule: rule.ID, Action: domain.ActionArchive,
	}}
	h.holds.contributors = map[shared.ID][]shared.ID{taskID: {accountID}}
	read := restrictedAccount()
	sweeper := h.sweeper()
	sweeper.Restrictions = read

	outcome, err := sweeper.Pass(t.Context(), actor())
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if len(h.marking.archived) != 0 || outcome.Blocked[domain.BlockedByRestriction] != 1 {
		t.Fatalf("archived %v under a restriction, blocked %+v", h.marking.archived, outcome.Blocked)
	}
	if read.asked != 1 {
		t.Errorf("the pass read the restrictions %d times, want once", read.asked)
	}

	read.people = nil
	if _, err := sweeper.Pass(t.Context(), actor()); err != nil {
		t.Fatalf("sweeping after the lift: %v", err)
	}
	if len(h.marking.archived) != 1 {
		t.Errorf("the entry was not acted on after the lift: %v", h.marking.archived)
	}
}

// A restriction that cannot be read stops the pass rather than reading as "nobody restricted".
func TestAnUnreadableRestrictionStopsThePass(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(*domain.NewRuleInput) {})
	h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -400))}
	sweeper := h.sweeper()
	broken := errors.New("unavailable")
	sweeper.Restrictions = &restrictions{err: broken}

	if _, err := sweeper.Pass(t.Context(), actor()); !errors.Is(err, broken) {
		t.Fatalf("the pass answered %v", err)
	}
	if len(h.marking.marked) != 0 {
		t.Error("the pass marked without knowing who is restricted")
	}
}

// The automatic trash purge keeps a restricted person's deletion and takes the rest; a person
// emptying the trash is not stopped by it, because a restriction is not a lock.
func TestTheAutomaticPurgeKeepsARestrictedPersonsDeletion(t *testing.T) {
	longAgo := now.Add(-120 * 24 * time.Hour)

	t.Run("the retention job", func(t *testing.T) {
		h := newHarness()
		h.expired.items = []repository.ExpiredItem{expiredItem(taskID, longAgo), expiredItem(otherTaskID, longAgo)}
		h.holds.contributors = map[shared.ID][]shared.ID{taskID: {colleagueID, accountID}}
		h.purger.Restrictions = restrictedAccount()

		outcome, err := h.purger.Sweep(t.Context(), actor(), Selection{
			Cutoff: now, Reason: domain.DeletedByRetention, ObserveRestrictions: true,
		}, now)
		if err != nil {
			t.Fatalf("the sweep failed: %v", err)
		}
		if len(h.trash.purgedItem) != 1 || h.trash.purgedItem[0] != otherTaskID {
			t.Errorf("the sweep removed %v, want only the entry nobody restricted touched", h.trash.purgedItem)
		}
		if outcome.Blocked[BlockedByRestriction] != 1 || outcome.Held != 1 {
			t.Errorf("blocked %v, held %d", outcome.Blocked, outcome.Held)
		}
	})

	t.Run("a person emptying the trash", func(t *testing.T) {
		h := newHarness()
		h.expired.items = []repository.ExpiredItem{expiredItem(taskID, longAgo)}
		h.holds.contributors = map[shared.ID][]shared.ID{taskID: {accountID}}
		read := restrictedAccount()
		h.purger.Restrictions = read

		if _, err := h.purger.Sweep(t.Context(), actor(), Selection{Cutoff: now, Reason: domain.DeletedByUser}, now); err != nil {
			t.Fatalf("the sweep failed: %v", err)
		}
		if len(h.trash.purgedItem) != 1 {
			t.Errorf("emptying the trash removed %v", h.trash.purgedItem)
		}
		if read.asked != 0 {
			t.Error("an explicit purge read the restrictions")
		}
	})

	t.Run("a person purging one entry", func(t *testing.T) {
		h := newHarness()
		h.trash.subtree = []shared.ID{taskID}
		h.holds.contributors = map[shared.ID][]shared.ID{taskID: {accountID}}
		h.purger.Restrictions = restrictedAccount()

		if _, err := h.purger.Subtree(t.Context(), actor(), trashedTask(), hubID, domain.DeletedByUser, now); err != nil {
			t.Fatalf("a named purge was refused: %v", err)
		}
	})
}
