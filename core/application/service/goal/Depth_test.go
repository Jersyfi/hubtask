// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
)

// UC-GOL-01 check 6: a goal moved under another brings the goals below it, so the tree it lands in
// counts them - else the deepest ones fall out of the top goal's progress.
func TestAGoalMovedWithItsOwnStaysWithinTheDepth(t *testing.T) {
	s, _, _, _ := newSteward(t)
	p := period(t, s)
	parent := ""
	for i := 0; i < domain.MaxDepth-1; i++ {
		in := usecase.Input{"period_id": p, "title": "Level"}
		if parent != "" {
			in["parent_id"] = parent
		}
		parent = ok(t, CreateGoal{s}.Descriptor(), ana, in).String("id")
	}
	moved := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": p, "title": "Moved"})
	below := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": p, "title": "Below", "parent_id": moved.String("id")})
	ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": p, "title": "Deepest", "parent_id": below.String("id")})

	_, err := call(t, UpdateGoal{s}.Descriptor(), ana, usecase.Input{
		"goal_id": moved.String("id"), "expected_version": 1, "period_id": p, "title": "Moved", "parent_id": parent,
	})
	if code(err) != "goals.too_deep" {
		t.Fatalf("a goal with two levels below it moved under the seventh level: %v", err)
	}
}

// UC-GOL-03: a portfolio moved under another brings the portfolios below it.
func TestAPortfolioMovedWithItsOwnStaysWithinTheNesting(t *testing.T) {
	s, _, _, _ := newSteward(t)
	parent := ""
	for i := 0; i < domain.MaxPortfolioNested-1; i++ {
		in := usecase.Input{"name": "Level"}
		if parent != "" {
			in["parent_id"] = parent
		}
		parent = ok(t, CreatePortfolio{s}.Descriptor(), admin, in).String("id")
	}
	moved := ok(t, CreatePortfolio{s}.Descriptor(), admin, usecase.Input{"name": "Moved"})
	ok(t, CreatePortfolio{s}.Descriptor(), admin, usecase.Input{"name": "Below", "parent_id": moved.String("id")})

	_, err := call(t, UpdatePortfolio{s}.Descriptor(), admin, usecase.Input{
		"portfolio_id": moved.String("id"), "expected_version": 1, "name": "Moved", "parent_id": parent,
	})
	if code(err) != "portfolios.too_deep" {
		t.Fatalf("a portfolio with one level below it moved under the deepest level: %v", err)
	}
}
