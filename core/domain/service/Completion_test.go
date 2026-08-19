// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

func done() work.Completion { return work.Completion{IsCompleted: true} }
func open() work.Completion { return work.Completion{} }

func children(total, completed int) work.ChildCompletion {
	return work.ChildCompletion{Total: total, Completed: completed}
}

// The acceptance criterion of B-07: a table over every policy, with no infrastructure in it.
func TestTheRollUpDecision(t *testing.T) {
	cases := []struct {
		name     string
		policy   work.CompletionPolicy
		parent   work.Completion
		children work.ChildCompletion
		want     service.ParentEffect
	}{
		// MANUAL never touches the parent, whatever the children say. That is the whole of the policy.
		{"manual, the last child completed", work.CompletionManual, open(), children(3, 3), service.ParentUnchanged},
		{"manual, a child reopened", work.CompletionManual, done(), children(3, 2), service.ParentUnchanged},
		{"manual, no children", work.CompletionManual, open(), children(0, 0), service.ParentUnchanged},

		// ROLLUP completing: only the last open child does anything.
		{"rollup, the last of three completed", work.CompletionRollup, open(), children(3, 3), service.ParentComplete},
		{"rollup, one of one completed", work.CompletionRollup, open(), children(1, 1), service.ParentComplete},
		{"rollup, two of three completed", work.CompletionRollup, open(), children(3, 2), service.ParentUnchanged},
		{"rollup, none completed", work.CompletionRollup, open(), children(3, 0), service.ParentUnchanged},

		// ROLLUP reopening: any open child under a completed parent reopens it.
		{"rollup, one of three reopened", work.CompletionRollup, done(), children(3, 2), service.ParentReopen},
		{"rollup, all three reopened", work.CompletionRollup, done(), children(3, 0), service.ParentReopen},
		{"rollup, the only child reopened", work.CompletionRollup, done(), children(1, 0), service.ParentReopen},

		// Idempotence, which I-W5 asks for by name: the parent is compared against the state it is in,
		// so a second pass over unchanged numbers concludes nothing.
		{"rollup, done over done", work.CompletionRollup, done(), children(3, 3), service.ParentUnchanged},
		{"rollup, open over open", work.CompletionRollup, open(), children(3, 1), service.ParentUnchanged},

		// A parent with no children concludes nothing in either direction. A level becomes empty by
		// having its children trashed, and that is not a reason to call the work finished - nor, if the
		// parent was already done, a reason to reopen it.
		{"rollup, no children under an open parent", work.CompletionRollup, open(), children(0, 0), service.ParentUnchanged},
		{"rollup, no children under a done parent", work.CompletionRollup, done(), children(0, 0), service.ParentUnchanged},

		// A policy nobody recognises changes nothing. It cannot be stored - ParseCompletionPolicy refuses
		// it - and the safe reading of a value this version does not understand is to leave the tree alone.
		{"an unknown policy", work.CompletionPolicy("SOMETHING_ELSE"), open(), children(2, 2), service.ParentUnchanged},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := service.RollUp(c.policy, c.parent, c.children); got != c.want {
				t.Errorf("RollUp(%s, completed=%v, %d/%d) = %s, want %s",
					c.policy, c.parent.IsCompleted, c.children.Completed, c.children.Total, got, c.want)
			}
		})
	}
}

// Asking twice is what the roll-up does when two children are completed in the same second, and what a
// retried event delivery does. The second answer has to be UNCHANGED once the parent has moved.
func TestTheRollUpIsIdempotent(t *testing.T) {
	summary := children(2, 2)

	first := service.RollUp(work.CompletionRollup, open(), summary)
	if first != service.ParentComplete {
		t.Fatalf("the first pass answered %s", first)
	}
	// The parent is now what the first pass made it, and the children have not moved.
	if second := service.RollUp(work.CompletionRollup, done(), summary); second != service.ParentUnchanged {
		t.Errorf("the second pass answered %s, want UNCHANGED", second)
	}
}

// A summary of no children is not "everything is done". Completing a parent on that basis would complete
// it the moment its last child was trashed.
func TestAnEmptyLevelIsNotAllCompleted(t *testing.T) {
	if children(0, 0).AllCompleted() {
		t.Error("no children counted as all completed")
	}
	if children(0, 0).AnyOpen() {
		t.Error("no children counted as something open")
	}
	if !children(2, 2).AllCompleted() {
		t.Error("two of two is not all completed")
	}
	if !children(2, 1).AnyOpen() {
		t.Error("one of two open is not any open")
	}
}

// A count above the total cannot happen from the query that produces it, and if it ever did the answer
// must not become "something is still open" - which would reopen a parent for ever.
func TestASummaryThatCannotHappenStillDecidesSafely(t *testing.T) {
	impossible := children(2, 3)

	if !impossible.AllCompleted() {
		t.Error("more completed than total did not count as all completed")
	}
	if impossible.AnyOpen() {
		t.Error("more completed than total counted as something open")
	}
}
