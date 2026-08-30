// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The flow kinds (automation.md §1.3, G-09). Three actions that are not use cases at all: they are
// the engine's own control structures, and they mean nothing outside a run.
//
// That is why they are absent from the catalogue and why the parity gate does not name them. The
// gate's rule is that every *use case* is reachable as an action; the converse - that every action
// is a use case - was never true, because "wait a day" is not something a person can ask the API to
// do to their workspace.
const (
	// ActionWait suspends the run and resumes it later. A job with a `run_at`, never a sleeping
	// worker: a rule that waited a day by holding a worker would hold it through every restart,
	// every deploy and every other rule's turn.
	ActionWait = "WAIT"
	// ActionBranch takes one of two paths depending on a condition, evaluated in the same
	// environment and with the same limits as the rule's own conditions (ADR-0009).
	ActionBranch = "BRANCH"
	// ActionStop ends the run where it stands. The actions after it are SKIPPED, and the run
	// succeeded: stopping early is what the rule said to do.
	ActionStop = "STOP"
)

// MaxBranchDepth is how deeply a rule may nest branches.
//
// Three, and the number matters less than the fact that there is one. A branch inside a branch
// inside a branch is already a rule nobody can read back, and an unbounded nesting is a document
// somebody can post that costs the parser more than it costs them to write.
const MaxBranchDepth = 3

// MaxWait is how long one WAIT may hold a run.
//
// A year, the bound the relative-date offset already uses, and for the same reason: it is past any
// delay somebody writes deliberately and short enough that a typo of one digit cannot resume a run
// after everybody who wrote it has left.
const MaxWait = MaxOffset

// IsFlowAction reports whether a kind is one of the engine's own control structures rather than a
// use case. Asked where the catalogue is consulted, so that a flow action is not looked up in a
// register it was never in.
func IsFlowAction(kind string) bool {
	switch kind {
	case ActionWait, ActionBranch, ActionStop:
		return true
	default:
		return false
	}
}

// Branch is what a BRANCH action carries, read out of its parameters.
//
// A nested list rather than a jump target: "skip the next two actions" is a rule whose meaning
// changes when somebody inserts an action above it, and a rule that quietly means something else
// after an edit is the failure this whole aggregate is written to avoid.
type Branch struct {
	Condition string
	Then      []Action
	Else      []Action
}

// ReadBranch reads a BRANCH's parameters, or says which field is wrong.
//
// The path is the branch's own, so that a refusal three levels down points at the line an editor
// has to put the cursor on rather than at the rule.
func ReadBranch(params map[string]any, path string, depth int) (Branch, error) {
	if depth >= MaxBranchDepth {
		return Branch{}, shared.ErrValidation.
			WithDetail("automation.branch_too_deep").
			WithParams(map[string]string{"maximum": itoa(MaxBranchDepth)}).
			WithFields(shared.FieldError{Path: path, Code: "automation.branch_too_deep"})
	}

	condition, _ := params["condition"].(string)
	if strings.TrimSpace(condition) == "" {
		return Branch{}, fieldError(path+"/params/condition", "automation.branch_condition_required")
	}

	then, err := branchArm(params, "then", path, depth)
	if err != nil {
		return Branch{}, err
	}
	if len(then) == 0 {
		// An empty `then` is a branch that does nothing when its condition holds, which is a rule
		// somebody wrote the wrong way round. `else` may be empty: "do this only when" is an
		// ordinary shape.
		return Branch{}, fieldError(path+"/params/then", "automation.branch_then_required")
	}
	otherwise, err := branchArm(params, "else", path, depth)
	if err != nil {
		return Branch{}, err
	}

	return Branch{Condition: strings.TrimSpace(condition), Then: then, Else: otherwise}, nil
}

// branchArm reads one side of a branch as a list of actions, validated exactly as a rule's own
// list is - which is what stops a branch being a way to write an action the top level refuses.
func branchArm(params map[string]any, name, path string, depth int) ([]Action, error) {
	raw, present := params[name]
	if !present || raw == nil {
		return nil, nil
	}

	rows, ok := raw.([]any)
	if !ok {
		return nil, fieldError(path+"/params/"+name, "automation.branch_arm_invalid")
	}

	actions := make([]Action, 0, len(rows))
	for i, row := range rows {
		document, ok := row.(map[string]any)
		if !ok {
			return nil, fieldError(
				path+"/params/"+name+"/"+itoa(i), "automation.branch_arm_invalid")
		}
		kind, _ := document["kind"].(string)
		params, _ := document["params"].(map[string]any)
		if params == nil {
			params = map[string]any{}
		}
		actions = append(actions, Action{Kind: strings.TrimSpace(kind), Params: params})
	}
	return validActionsAt(actions, path+"/params/"+name, depth+1)
}

// WaitFor reads a WAIT's delay, or says why it is not one.
//
// An unsigned ISO 8601 duration: "wait minus an hour" is not a thing a run can do, and reading a
// sign here would be reading a mistake as an instruction.
func WaitFor(params map[string]any, path string) (time.Duration, error) {
	raw, _ := params["duration"].(string)
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, fieldError(path+"/params/duration", "automation.wait_duration_required")
	}
	if strings.HasPrefix(text, "-") || strings.HasPrefix(text, "+") {
		return 0, fieldError(path+"/params/duration", "automation.wait_duration_invalid")
	}

	delay, err := parseOffset(text)
	if err != nil || delay <= 0 || delay > MaxWait {
		return 0, shared.ErrValidation.
			WithDetail("automation.wait_duration_invalid").
			WithParams(map[string]string{"duration": text}).
			WithFields(shared.FieldError{
				Path: path + "/params/duration", Code: "automation.wait_duration_invalid",
			})
	}
	return delay, nil
}

// ActionPath is where an action sits in a rule, as the run log and the idempotency key name it.
//
// `"2"` is the third action of the rule; `"2/then/0"` is the first action of that branch's `then`.
// A path rather than an index, because G-07's key is `(rule, occasion, action index)` and a nested
// action has no index at the top level - two branches' first actions would otherwise share a key
// and the second would silently do nothing.
func ActionPath(parent string, index int) string {
	if parent == "" {
		return itoa(index)
	}
	return parent + "/" + itoa(index)
}
