// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"errors"
	"testing"

	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PG-5: the retention job deletes after expiry and logs it; periods outside the bounds are rejected
// (data-protection.md §10).
//
// The second half is what this gate holds, because it is the half a build can decide: a period
// below the documented floor is refused, a chain that runs past the operator's ceiling needs a
// justification, and both refusals name the number the caller may not go past. That the job then
// deletes and records what it deleted is E-07's own suite against a real database
// (`test/retention`), which the nightly runs - a gate cannot prove a job ran by reading code.
//
// It exercises the rule rather than asserting that a test exists somewhere. A gate that checked
// for the presence of a test would go green on a test that was skipped.

func ruleInput(change func(*lifecycle.NewRuleInput)) lifecycle.NewRuleInput {
	in := lifecycle.NewRuleInput{
		ID:       shared.MustParseID("0192f000-0000-7000-8000-0000000000f1"),
		TenantID: shared.MustParseID("0192f000-0000-7000-8000-0000000000f2"),
		DataKind: lifecycle.KindTrash, RetainDays: 30, Action: lifecycle.ActionHardDelete,
		Scope: lifecycle.Scope{Kind: lifecycle.ScopeTenant},
	}
	change(&in)
	return in
}

func TestPG5APeriodBelowTheFloorIsRefused(t *testing.T) {
	kind, known := lifecycle.FindKind(lifecycle.KindTrash)
	if !known {
		t.Fatal("the trash has no entry in the retention catalogue")
	}
	if kind.MinDays <= 0 {
		t.Fatalf("the trash's lower bound is %d - a floor of zero bounds nothing", kind.MinDays)
	}

	_, err := lifecycle.NewRule(ruleInput(func(in *lifecycle.NewRuleInput) {
		in.RetainDays = kind.MinDays - 1
	}))
	if err == nil {
		t.Fatal("a period below the documented floor was accepted (PG-5)")
	}

	var problem *shared.Error
	if !errors.As(err, &problem) || problem.DetailCode != lifecycle.CodeBelowLowerBound {
		t.Fatalf("the refusal came back as %v", err)
	}
	// The number they may not go below is in the refusal: a rule that only said "no" would leave
	// the operator guessing at a bound the document already states.
	if problem.Params["min_days"] == "" {
		t.Errorf("the refusal does not name the floor: %v", problem.Params)
	}
}

func TestPG5APeriodPastTheCeilingNeedsAJustification(t *testing.T) {
	ceiling := 90

	_, err := lifecycle.NewRule(ruleInput(func(in *lifecycle.NewRuleInput) {
		in.RetainDays, in.Ceiling = ceiling+1, ceiling
	}))
	if err == nil {
		t.Fatal("a period past the operator's ceiling was accepted with no justification (PG-5)")
	}
	var problem *shared.Error
	if !errors.As(err, &problem) || problem.DetailCode != lifecycle.CodeJustificationRequired {
		t.Fatalf("the refusal came back as %v", err)
	}
	if problem.Params["max_days"] == "" {
		t.Errorf("the refusal does not name the ceiling: %v", problem.Params)
	}

	// And with one it is allowed, and the rule says it exceeded the bound - which is what makes
	// the extension auditable rather than invisible (§4.4).
	rule, err := lifecycle.NewRule(ruleInput(func(in *lifecycle.NewRuleInput) {
		in.RetainDays, in.Ceiling = ceiling+1, ceiling
		in.Justification = "Kept for the statutory period of the contract"
	}))
	if err != nil {
		t.Fatalf("a justified extension was refused: %v", err)
	}
	if !rule.ExceedsCeiling(ceiling) {
		t.Error("a rule past the ceiling does not report that it is")
	}
}

// Every data kind the catalogue knows carries a floor, because a kind without one is a kind whose
// rules cannot be checked against anything.
func TestPG5EveryDataKindHasALowerBound(t *testing.T) {
	kinds := lifecycle.Catalogue()
	if len(kinds) == 0 {
		t.Fatal("the retention catalogue is empty")
	}

	for _, kind := range kinds {
		if kind.MinDays < 0 {
			t.Errorf("%s has a lower bound of %d (PG-5)", kind.Name, kind.MinDays)
		}
	}
}
