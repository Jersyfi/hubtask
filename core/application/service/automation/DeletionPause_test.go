// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/recurrence"
)

// paused is the marked rules, and what the cancel wrote back for each.
type paused struct {
	rules   []domain.Rule
	resumed map[shared.ID]time.Time
}

func (p *paused) List(context.Context) ([]domain.Rule, error) { return p.rules, nil }

func (p *paused) Resume(_ context.Context, id shared.ID, next, _ time.Time) (bool, error) {
	if _, done := p.resumed[id]; done {
		return false, nil
	}
	p.resumed[id] = next
	return true, nil
}

// A cancelled deletion switches the rules back on as enabling by hand does: a schedule from now,
// none of the occurrences missed while the workspace was pending, and one wake for the earliest.
func TestACancelledDeletionSwitchesThePausedRulesBackOn(t *testing.T) {
	event := ruleAt(domain.Scope{Type: domain.ScopeTenant}, shared.ID("01936f2a-7c1e-7000-8000-0000000000d1"))
	schedule := scheduledRule(now.Add(-72 * time.Hour))
	schedule.ID = shared.ID("01936f2a-7c1e-7000-8000-0000000000d2")
	store := &paused{rules: []domain.Rule{event, schedule}, resumed: map[shared.ID]time.Time{}}
	queued := &jobs{}

	resumed, err := PausedRules{Paused: store, Expander: expander{step: time.Hour}, Jobs: queued}.
		Resume(t.Context(), tenant, now)
	if err != nil || resumed != 2 {
		t.Fatalf("resumed (%d, %v), want both", resumed, err)
	}
	if next := store.resumed[event.ID]; !next.IsZero() {
		t.Errorf("an event rule was given a moment %v", next)
	}
	if next := store.resumed[schedule.ID]; !next.Equal(now.Add(time.Hour)) {
		t.Errorf("the schedule's next moment is %v, want an hour from now - not the missed ones", next)
	}
	if len(queued.queued) != 1 || queued.queued[0].Kind != queue.KindAutomationSchedule ||
		queued.queued[0].TenantID != tenant || !queued.queued[0].RunAt.Equal(now.Add(time.Hour)) {
		t.Errorf("the poller was woken with %+v", queued.queued)
	}
}

// A schedule this build can no longer expand stays off rather than on with no moment; the others
// come back regardless, and with no schedule among them nothing wakes the poller.
func TestAnUnreadableScheduleStaysOff(t *testing.T) {
	event := ruleAt(domain.Scope{Type: domain.ScopeTenant}, shared.ID("01936f2a-7c1e-7000-8000-0000000000d1"))
	schedule := scheduledRule(now)
	schedule.ID = shared.ID("01936f2a-7c1e-7000-8000-0000000000d2")
	store := &paused{rules: []domain.Rule{event, schedule}, resumed: map[shared.ID]time.Time{}}
	queued := &jobs{}

	resumed, err := PausedRules{
		Paused: store, Expander: expander{err: recurrence.ErrRuleUnreadable}, Jobs: queued,
	}.Resume(t.Context(), tenant, now)
	if err != nil || resumed != 1 {
		t.Fatalf("resumed (%d, %v), want the event rule alone", resumed, err)
	}
	if _, on := store.resumed[schedule.ID]; on {
		t.Error("an unreadable schedule was switched on")
	}
	if len(queued.queued) != 0 {
		t.Errorf("the poller was woken for nothing: %+v", queued.queued)
	}
}
