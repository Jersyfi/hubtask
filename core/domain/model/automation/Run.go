// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxCausationDepth is how far a chain of rules may reach from the act a person performed
// (automation.md §2).
//
// Five, and the number matters less than the fact that there is one. A rule that reacts to an event
// can cause an event that another rule reacts to, and two rules that each react to the other's
// output are a loop that costs a worker rather than a stack. The depth travels on the envelope
// (ADR-0007), so the bound is checked where the run begins rather than kept in a counter something
// has to reset.
//
// Aborting is not failing. A run at the limit did nothing and says so with its own status, because
// "this rule is misconfigured into a loop" and "this rule's action was refused" send their reader to
// two different places.
const MaxCausationDepth = 5

// RunStatus is how a run ended.
type RunStatus string

const (
	// RunRunning is a run in flight - or one whose process died. The engine writes it when the run
	// starts, so a row left in it is a crash rather than a state anything reaches deliberately.
	RunRunning RunStatus = "RUNNING"
	// RunSucceeded is a run that acted. Its actions may not all have worked: `on_error: CONTINUE`
	// finishes a run whose second action was refused, and the per-action results say so.
	RunSucceeded RunStatus = "SUCCEEDED"
	// RunSkipped is a run whose conditions said no. The ordinary answer of a rule that is working.
	RunSkipped RunStatus = "SKIPPED"
	// RunFailed is a run that could not do what its rule says.
	RunFailed RunStatus = "FAILED"
	// RunAbortedLoop is the loop protection: the run was at the depth limit and did nothing.
	RunAbortedLoop RunStatus = "ABORTED_LOOP"
	// RunThrottled is a run the rule's own bound held back. Not a failure and not a skip: the
	// conditions were never asked, because the rule has already run as often as it may this hour.
	RunThrottled RunStatus = "THROTTLED"
)

// Valid reports whether the status is one the column allows.
func (s RunStatus) Valid() bool {
	switch s {
	case RunRunning, RunSucceeded, RunSkipped, RunFailed, RunAbortedLoop, RunThrottled:
		return true
	default:
		return false
	}
}

// Finished reports whether the run is over. What the engine asks before writing a result, and what
// a reader asks before trusting `finished_at`.
func (s RunStatus) Finished() bool { return s != RunRunning }

// ActionStatus is one action's outcome.
type ActionStatus string

const (
	ActionSucceeded ActionStatus = "SUCCEEDED"
	ActionFailed    ActionStatus = "FAILED"
	// ActionSkipped is an action the run never reached, because `on_error: STOP` ended the run at
	// an earlier failure. Not the same as an action that ran and did nothing - which is
	// ActionSucceeded, because the use case decided there was nothing to do.
	ActionSkipped ActionStatus = "SKIPPED"
)

// ConditionResult is how one condition answered, in the order the rule declares them.
type ConditionResult struct {
	Index   int
	Matched bool
	// ErrorCode is present when the condition could not be evaluated at all - a timeout, or a value
	// the engine could not read. Distinct from Matched being false, which is the condition working.
	ErrorCode string
}

// ActionResult is one action's outcome, in the order the rule declares them.
type ActionResult struct {
	Index  int
	Kind   string
	Status ActionStatus
	// ErrorCode is the code the use case refused with, unchanged. A `run_as` account that may not
	// do what the action asks shows the authoriser's own refusal here, which is what makes the run
	// log answer "why did this not happen" rather than "something went wrong".
	ErrorCode string
	// IdempotencyKey is what made the action safe to attempt twice. Recorded because a person
	// comparing two runs of one event needs to see that they carried the same key.
	IdempotencyKey string
}

// Run is one rule's reaction to one event, and everything that happened in it.
//
// The record rather than a log line: `automation.md` §2 promises it is retrievable and filterable,
// and a person asking "why did this not happen" needs the conditions' answers and the actions'
// refusals rather than a sentence. Nothing here carries user content - an action's parameters are
// not on it, and an error is a code (rule 10).
type Run struct {
	ID       shared.ID
	TenantID shared.ID
	RuleID   shared.ID
	// EventID is the event that started the run, and zero for a run nothing published started.
	EventID          shared.ID
	Status           RunStatus
	ConditionResults []ConditionResult
	ActionResults    []ActionResult
	// ErrorCode is why the run as a whole ended badly, when it did.
	ErrorCode      string
	StartedAt      time.Time
	FinishedAt     *time.Time
	CausationDepth int
}

// NewRunInput is what starting a run needs.
type NewRunInput struct {
	ID             shared.ID
	TenantID       shared.ID
	RuleID         shared.ID
	EventID        shared.ID
	CausationDepth int
	Now            time.Time
}

// StartRun opens a run in the RUNNING state.
//
// Written before the conditions are evaluated, which is what makes a crashed run visible. The
// alternative - writing the row when the run ends - loses exactly the runs somebody needs to see.
func StartRun(in NewRunInput) (Run, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.RuleID.IsZero() {
		return Run{}, shared.ErrInternal.WithDetail("automation.run_incomplete")
	}
	if in.CausationDepth < 0 {
		return Run{}, shared.ErrInternal.WithDetail("automation.run_incomplete")
	}

	return Run{
		ID: in.ID, TenantID: in.TenantID, RuleID: in.RuleID, EventID: in.EventID,
		Status:           RunRunning,
		ConditionResults: []ConditionResult{},
		ActionResults:    []ActionResult{},
		StartedAt:        in.Now.UTC(),
		CausationDepth:   in.CausationDepth,
	}, nil
}

// TooDeep reports whether a run at this depth may act at all.
//
// Asked of the depth the *event* carried, before anything else about the run is decided: a run that
// may not act must not evaluate conditions either, because evaluating them is where the reads are
// and a loop that read on every hop would cost exactly what the bound exists to prevent.
func TooDeep(depth int) bool { return depth >= MaxCausationDepth }

// Abort ends the run as a loop that was stopped.
func (r Run) Abort(at time.Time) Run {
	return r.end(RunAbortedLoop, "automation.loop_depth_exceeded", at)
}

// Throttle ends the run as one the rule's own bound held back.
func (r Run) Throttle(at time.Time) Run {
	return r.end(RunThrottled, "automation.throttled", at)
}

// Skip ends the run as one whose conditions said no, carrying how each of them answered.
func (r Run) Skip(results []ConditionResult, at time.Time) Run {
	r.ConditionResults = results
	return r.end(RunSkipped, "", at)
}

// Fail ends the run as one that could not do what its rule says.
func (r Run) Fail(code string, at time.Time) Run { return r.end(RunFailed, code, at) }

// Complete ends the run with what its actions did.
//
// SUCCEEDED even when an action failed, as long as the run itself reached its end: that is what
// `on_error: CONTINUE` means, and the per-action results are where a reader sees which one refused.
// A run whose `on_error` is STOP and whose action failed does not reach here - the engine fails it.
func (r Run) Complete(conditions []ConditionResult, actions []ActionResult, at time.Time) Run {
	r.ConditionResults, r.ActionResults = conditions, actions
	return r.end(RunSucceeded, "", at)
}

func (r Run) end(status RunStatus, code string, at time.Time) Run {
	finished := at.UTC()
	r.Status, r.ErrorCode, r.FinishedAt = status, code, &finished
	if r.ConditionResults == nil {
		r.ConditionResults = []ConditionResult{}
	}
	if r.ActionResults == nil {
		r.ActionResults = []ActionResult{}
	}
	return r
}

// Succeeded and Failed count the actions, which is what the finished and failed events carry.
func (r Run) Succeeded() int { return r.count(ActionSucceeded) }

// Failed counts the actions that ran and refused.
func (r Run) Failed() int { return r.count(ActionFailed) }

func (r Run) count(status ActionStatus) int {
	total := 0
	for _, result := range r.ActionResults {
		if result.Status == status {
			total++
		}
	}
	return total
}
