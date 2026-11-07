// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-GOL-02 check 3: a goal set or moved into a period while it is closed is refused, never left
// ungraded in a closed period, taking check-ins for ever.
func TestAGoalNeverLandsInAPeriodClosedMeanwhile(t *testing.T) {
	s, store, _, _ := newSteward(t)
	p := period(t, s)
	other := ok(t, DefineGoalPeriod{s}.Descriptor(), admin, usecase.Input{
		"name": "2027 Q1", "start_on": "2027-01-01", "end_on": "2027-03-31",
	}).String("id")
	moved := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": p, "title": "Moved"})

	store.closeOnRead = true
	if _, err := call(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": other, "title": "Late"}); code(err) != "goal_periods.closed" {
		t.Fatalf("a goal was set in a period closed meanwhile: %v", err)
	}
	store.closeOnRead = false
	reopen(store, other)
	store.closeOnRead = true
	if _, err := call(t, UpdateGoal{s}.Descriptor(), ana, usecase.Input{
		"goal_id": moved.String("id"), "expected_version": 1, "period_id": other, "title": "Moved",
	}); code(err) != "goal_periods.closed" {
		t.Fatalf("a goal was moved into a period closed meanwhile: %v", err)
	}
	store.closeOnRead = false

	// The close holds the period before it lists the goals it grades.
	reopen(store, p)
	store.steps = nil
	ok(t, CloseGoalPeriod{s}.Descriptor(), admin, usecase.Input{"period_id": p, "expected_version": store.periods[mustID(t, p)].Version})
	lock := indexOf(store.steps, "lock "+p)
	list := indexOf(store.steps, "goals "+p)
	if lock < 0 || list < 0 || lock > list {
		t.Fatalf("the close listed its goals before holding the period: %v", store.steps)
	}
}

func indexOf(steps []string, prefix string) int {
	for i, s := range steps {
		if strings.HasPrefix(s, prefix) {
			return i
		}
	}
	return -1
}

func reopen(store *memory, id string) {
	for key, p := range store.periods {
		if key.String() == id {
			p.State = "OPEN"
			store.periods[key] = p
		}
	}
}

func mustID(t *testing.T, text string) shared.ID {
	t.Helper()
	id, err := shared.ParseID(text)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
