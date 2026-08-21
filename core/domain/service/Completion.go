// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service

import "github.com/Jersyfi/hubtask/core/domain/model/work"

// The roll-up of invariant I-W5: with the collection's completion policy set to ROLLUP, a parent item
// follows its children - completed when the last open one is done, open again when any is reopened.
//
// A pure function of a policy and two numbers, which is the whole point. Every branch below is a table
// test with no database, no clock and no repository in it; what the numbers were is the adapter's
// problem, and what to do about the answer is the use case's.

// ParentEffect is what a child's changed completion means for the item above it.
type ParentEffect string

const (
	// ParentUnchanged is the answer most of the time: the policy does not roll up, or the parent is
	// already in the state the children imply.
	ParentUnchanged ParentEffect = "UNCHANGED"
	// ParentComplete means the last open child was just completed.
	ParentComplete ParentEffect = "COMPLETE"
	// ParentReopen means a child is open again and the parent is not.
	ParentReopen ParentEffect = "REOPEN"
)

// RollUp decides what happens to a parent, given the collection's policy, the parent's own state, and
// the summary of its children.
//
// It compares against the state the parent is *in*, not against the change that was just made, and that
// is what makes it idempotent as I-W5 requires: asked twice with the same numbers, the second answer is
// UNCHANGED. Two children completed in the same second therefore complete the parent once, and a
// retried delivery of the same event writes nothing.
//
// It is also why the direction is derived rather than passed in. A caller that said "a child was
// completed, roll up" would be trusted about which way to go, and the case where that is wrong is
// exactly the interesting one: completing one child while another is still open must complete nothing,
// and completing the last one must complete the parent even if the caller thought otherwise.
func RollUp(policy work.CompletionPolicy, parent work.Completion, children work.ChildCompletion) ParentEffect {
	if !policy.RollsUp() {
		return ParentUnchanged
	}

	switch {
	case children.AllCompleted() && !parent.IsCompleted:
		return ParentComplete
	case children.AnyOpen() && parent.IsCompleted:
		return ParentReopen
	default:
		// Includes the two states that need no explanation - a done parent over done children, an open
		// parent over open ones - and the one that does: a parent with no children at all. Nothing is
		// concluded from an empty level, because a level becomes empty by having its children trashed,
		// and that is not a reason to call the work finished.
		return ParentUnchanged
	}
}
