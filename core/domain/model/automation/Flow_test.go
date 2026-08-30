// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	automation "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The three flow kinds are the one place the aggregate checks an action's *parameters* (G-09).
// Every other kind is a use case and the catalogue answers for it; these three are the engine's own
// control structures and have nobody else to be checked by.

func branchAction(params map[string]any) automation.Action {
	return automation.Action{Kind: automation.ActionBranch, Params: params}
}

func labelAction() any {
	return map[string]any{"kind": "ADD_LABEL", "params": map[string]any{"label_id": "x"}}
}

func TestTheFlowKindsAreNotUseCases(t *testing.T) {
	for _, kind := range []string{
		automation.ActionWait, automation.ActionBranch, automation.ActionStop,
	} {
		if !automation.IsFlowAction(kind) {
			t.Errorf("%s is not recognised as a flow action", kind)
		}
	}
	if automation.IsFlowAction("ADD_LABEL") {
		t.Error("a use case is recognised as a flow action")
	}
}

// A WAIT with no delay is a rule that would suspend for ever, and a negative one is a mistake read
// as an instruction.
func TestAWaitNeedsAPositiveBoundedDelay(t *testing.T) {
	good, err := automation.WaitFor(map[string]any{"duration": "PT1H"}, "/actions/0")
	if err != nil {
		t.Fatalf("reading an hour: %v", err)
	}
	if good != time.Hour {
		t.Errorf("read %v, want an hour", good)
	}

	for name, params := range map[string]map[string]any{
		"absent":      {},
		"empty":       {"duration": "  "},
		"negative":    {"duration": "-PT1H"},
		"signed":      {"duration": "+PT1H"},
		"zero":        {"duration": "PT0S"},
		"nonsense":    {"duration": "an hour"},
		"months":      {"duration": "P1M"},
		"past a year": {"duration": "P400D"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := automation.WaitFor(params, "/actions/0")
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error %v, want a validation refusal", err)
			}
			fields := shared.AsError(err).Fields
			if len(fields) != 1 || !strings.HasSuffix(fields[0].Path, "/params/duration") {
				t.Errorf("fields %v, want the delay's own field named", fields)
			}
		})
	}
}

// A branch is a nested condition and a nested list, and both halves are required to mean something:
// a branch with no condition decides nothing, and one with an empty `then` is a rule written the
// wrong way round.
func TestABranchNeedsAConditionAndSomethingToDo(t *testing.T) {
	good, err := automation.ReadBranch(map[string]any{
		"condition": "item.completed",
		"then":      []any{labelAction()},
		"else":      []any{labelAction(), labelAction()},
	}, "/actions/0", 0)
	if err != nil {
		t.Fatalf("reading a branch: %v", err)
	}
	if good.Condition != "item.completed" || len(good.Then) != 1 || len(good.Else) != 2 {
		t.Errorf("read %+v", good)
	}

	for name, params := range map[string]map[string]any{
		"no condition":              {"then": []any{labelAction()}},
		"empty condition":           {"condition": "   ", "then": []any{labelAction()}},
		"no then":                   {"condition": "true"},
		"empty then":                {"condition": "true", "then": []any{}},
		"a then that is not a list": {"condition": "true", "then": "ADD_LABEL"},
		"a row that is not an action": {
			"condition": "true", "then": []any{labelAction()}, "else": []any{"ADD_LABEL"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := automation.ReadBranch(params, "/actions/0", 0); !errors.Is(err, shared.ErrValidation) {
				t.Errorf("error %v, want a validation refusal", err)
			}
		})
	}

	// `else` is genuinely optional: "do this only when" is an ordinary shape.
	only, err := automation.ReadBranch(map[string]any{
		"condition": "true", "then": []any{labelAction()},
	}, "/actions/0", 0)
	if err != nil {
		t.Fatalf("reading a one-sided branch: %v", err)
	}
	if len(only.Else) != 0 {
		t.Errorf("an absent else read as %d actions", len(only.Else))
	}
}

// A branch inside a branch inside a branch is already a rule nobody can read back, and an unbounded
// nesting is a document somebody can post that costs the parser more than it costs them to write.
func TestBranchesAreBoundedInDepth(t *testing.T) {
	nested := func(depth int) map[string]any {
		params := map[string]any{"condition": "true", "then": []any{labelAction()}}
		for range depth {
			params = map[string]any{
				"condition": "true",
				"then": []any{map[string]any{
					"kind": automation.ActionBranch, "params": params,
				}},
			}
		}
		return params
	}

	if _, err := automation.ValidActionShape([]automation.Action{branchAction(nested(1))}); err != nil {
		t.Errorf("two levels of branch were refused: %v", err)
	}
	err := func() error {
		_, err := automation.ValidActionShape([]automation.Action{branchAction(nested(5))})
		return err
	}()
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a refusal past the depth bound", err)
	}
	if code := shared.AsError(err).DetailCode; code != "automation.branch_too_deep" {
		t.Errorf("code %q does not say what is wrong", code)
	}
}

// The bound on how many steps one rule may take is over the whole tree, all the way down. A rule
// that hid forty actions inside two branches would otherwise pass a count written about the top
// level, and one that hid them three levels down would pass a count written about the first.
func TestTheActionBoundCountsTheWholeTree(t *testing.T) {
	arm := make([]any, 0, automation.MaxActions)
	for range automation.MaxActions {
		arm = append(arm, labelAction())
	}

	_, err := automation.ValidActionShape([]automation.Action{
		branchAction(map[string]any{"condition": "true", "then": arm}),
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a refusal past the action bound", err)
	}
	if code := shared.AsError(err).DetailCode; code != "automation.too_many_actions" {
		t.Errorf("code %q does not say what is wrong", code)
	}
}

// A nested action is validated exactly as a top-level one, which is what stops a branch being a way
// to write an action the top level would refuse.
func TestANestedActionIsCheckedLikeAnyOther(t *testing.T) {
	_, err := automation.ValidActionShape([]automation.Action{
		branchAction(map[string]any{
			"condition": "true",
			"then": []any{map[string]any{
				"kind": automation.ActionWait, "params": map[string]any{"duration": "-PT1H"},
			}},
		}),
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want the nested WAIT refused", err)
	}
	fields := shared.AsError(err).Fields
	if len(fields) != 1 || !strings.Contains(fields[0].Path, "/then/0/params/duration") {
		t.Errorf("fields %v, want the nested field's own path", fields)
	}
}

// The path is what the run log and the idempotency key name an action by. An index alone would make
// two branches' first actions share a key, and the second would silently do nothing.
func TestAnActionPathNamesWhereTheActionSits(t *testing.T) {
	if got := automation.ActionPath("", 2); got != "2" {
		t.Errorf("top-level path %q, want %q", got, "2")
	}
	if got := automation.ActionPath("2/then", 0); got != "2/then/0" {
		t.Errorf("nested path %q, want %q", got, "2/then/0")
	}
}
