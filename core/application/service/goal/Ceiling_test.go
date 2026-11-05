// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"fmt"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// fill puts n goals into the period straight into the store, as a workspace of an older release
// may hold them.
func fill(t *testing.T, store *memory, periodID string, n int) {
	t.Helper()
	p := shared.MustParseID(periodID)
	for i := 0; i < n; i++ {
		id := shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8000-ee%010x", i))
		store.goals[id] = domain.Goal{ID: id, PeriodID: p, Title: "g", Confidence: -1, Version: 1}
		store.order = append(store.order, id)
	}
}

// A workspace holds as many goals as every reader of them reads at once - the progress, a
// portfolio, the close: one more is refused with goals.limit rather than left out of each.
func TestAGoalPastTheWorkspacesCeilingIsRefused(t *testing.T) {
	s, store, _, _ := newSteward(t)
	p := period(t, s)
	fill(t, store, p, maxGoals)
	if _, err := call(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": p, "title": "One more"}); code(err) != "goals.limit" {
		t.Fatalf("a goal past the ceiling: %v", err)
	}
}

// A period holding more goals than the close grades at once is refused with
// goal_periods.too_many_goals and stays open, never closed with the rest ungraded.
func TestAPeriodWithMoreGoalsThanTheCloseGradesStaysOpen(t *testing.T) {
	s, store, _, _ := newSteward(t)
	p := period(t, s)
	fill(t, store, p, maxGoals+1)
	if _, err := call(t, CloseGoalPeriod{s}.Descriptor(), admin, usecase.Input{"period_id": p, "expected_version": 1}); code(err) != "goal_periods.too_many_goals" {
		t.Fatalf("closing a crowded period: %v", err)
	}
	if store.periods[shared.MustParseID(p)].State != domain.PeriodOpen {
		t.Fatal("the period was closed")
	}
}
