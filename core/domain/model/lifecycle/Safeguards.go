// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import "slices"

// The reasons a removal can be refused, as `retention_run.blocked_reasons` records them, as the
// metric labels them, and as `RetentionState.blocked_by` answers them.
//
// A small closed set, written by hand: an unbounded label is an unbounded metric
// (observability-reliability.md §3.2). They live in the domain rather than beside the engine
// because data-retention.md §4 makes them a precedence order rather than a set - and an order is a
// rule about the data, which is exactly what the domain is for.
const (
	// BlockedByLegalHold is §4.1, and it outranks everything: a hold on a tenant, a container or an
	// item means no deletion and no anonymisation, and lifting it is auditable.
	BlockedByLegalHold = "legal_hold"
	// BlockedByRestriction is §4.2: an ongoing data subject request under GDPR Art. 18 restricts
	// processing, and a restricted object is neither deleted nor changed. E-10 builds the request;
	// what is here is its place in the order, so that the task which fills the seam does not also
	// have to decide where it sits.
	BlockedByRestriction = "restriction"
	// BlockedByTombstoneWindow is §4.5: an object may only disappear for good once every known
	// device has had the chance to learn of the deletion (offline-sync.md §7). It bounds the
	// automatic runs; an explicit purge is a decision somebody took and proceeds, leaving the
	// tombstone to stop the resurrection.
	BlockedByTombstoneWindow = "tombstone_window"
	// BlockedByDescendant is §4.6, the referential safeguard: a work package is not removed while
	// activities hang off it whose own period has not run out. The chain is worked from the bottom
	// up, so the child goes first and the parent goes on the pass after that.
	BlockedByDescendant = "descendant_retained"
)

// blockOrder is §4's precedence, and the first match wins.
//
// An order rather than a set, and that is the difference the document insists on: an object under a
// legal hold *and* past its tombstone window is blocked by the hold, and reporting the window would
// send an operator to look at the wrong thing. The upper and lower bounds of §4.3 and §4.4 are not
// in this list because they are not reasons an object is kept - they bound the rule, and they are
// decided where a rule is written rather than where an object is judged.
var blockOrder = [...]string{
	BlockedByLegalHold,
	BlockedByRestriction,
	BlockedByTombstoneWindow,
	BlockedByDescendant,
}

// BlockOrder is the precedence of §4, outermost first.
func BlockOrder() []string { return slices.Clone(blockOrder[:]) }

// FirstBlock answers which of the reasons that apply is the one to report.
//
// The caller hands in what it found; this decides which one the object is blocked *by*. Deciding it
// here rather than at each call site is what stops two paths reporting the same object under two
// different reasons - which would make the metric's series disagree with the object's own
// `blocked_by`.
func FirstBlock(found map[string]bool) (string, bool) {
	for _, reason := range blockOrder {
		if found[reason] {
			return reason, true
		}
	}
	return "", false
}
