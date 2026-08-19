// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "github.com/Jersyfi/hubtask/core/domain/model/shared"

// CompletionPolicy is a collection's answer to one question: when a child's completion changes, does
// anything happen to the item above it (domain-model.md §3.3, invariant I-W5).
//
// A policy of the collection rather than of the item, because it is a way of working rather than a
// property of one task: a team that tracks a work package as done when its activities are done wants
// that for the whole collection, and a team that closes a work package deliberately wants that
// everywhere too.
//
// Two values. `ROLLUP` is the one domain-model.md names; `MANUAL` is the absence of it, and it is the
// default because it is the behaviour that surprises nobody - an item changes when somebody changes it.
// No third value for "roll up completing but not reopening": nothing asks for it, and a policy that
// completed a parent automatically and then required a person to reopen it by hand would leave the
// parent wrong in exactly the case the roll-up exists to handle.
type CompletionPolicy string

const (
	// CompletionManual leaves the parent alone. Completing the last activity of a work package
	// completes that activity, and nothing else.
	CompletionManual CompletionPolicy = "MANUAL"
	// CompletionRollup completes the parent when its last open child is completed, and reopens it
	// when any child is reopened (I-W5).
	CompletionRollup CompletionPolicy = "ROLLUP"
)

// completionPolicies is the closed set, in the order of the constants above.
var completionPolicies = [...]CompletionPolicy{CompletionManual, CompletionRollup}

// CompletionPolicies returns every defined policy. /meta/capabilities answers from it, so a client
// configures its own form from the installation rather than from a copy of this list.
func CompletionPolicies() []CompletionPolicy { return completionPolicies[:] }

// DefaultCompletionPolicy is what a collection that has never been configured behaves as.
const DefaultCompletionPolicy = CompletionManual

// Valid reports whether the policy is one of the defined ones.
func (p CompletionPolicy) Valid() bool {
	for _, known := range completionPolicies {
		if known == p {
			return true
		}
	}
	return false
}

// OrDefault reads the absent policy as the default: a collection that has never been configured
// behaves as MANUAL, and so does a hub, which has no items for a policy to decide about.
//
// One rule in one place. The column starts as `{}` and a Container built anywhere but by the adapter
// carries the zero value, so every reader would otherwise have to know that "" and MANUAL are the
// same thing - and one of them would eventually not.
func (p CompletionPolicy) OrDefault() CompletionPolicy {
	if p == "" {
		return DefaultCompletionPolicy
	}
	return p
}

// ParseCompletionPolicy reads a stored or submitted policy.
//
// The empty value is the default rather than an error: the `policies` column starts as `{}`, so every
// collection that predates this policy has no value for it, and a read that failed on those would make
// the column impossible to introduce. Anything else that is not a known policy *is* an error - a value
// nobody recognises is not a reason to silently pick a behaviour.
func ParseCompletionPolicy(value string) (CompletionPolicy, error) {
	if value == "" {
		return CompletionPolicy("").OrDefault(), nil
	}
	policy := CompletionPolicy(value)
	if !policy.Valid() {
		return "", shared.ErrValidation.
			WithDetail("containers.completion_policy_unknown").
			WithParams(map[string]string{"value": value}).
			WithFields(shared.FieldError{
				Path: "/policies/completion_policy", Code: "containers.completion_policy_unknown",
			})
	}
	return policy, nil
}

// RollsUp reports whether this policy propagates a child's change upwards. The one question the
// completion service asks of it, kept here so that adding a policy that also rolls up is a change to
// this line rather than to every comparison against ROLLUP.
func (p CompletionPolicy) RollsUp() bool { return p == CompletionRollup }

// ChildCompletion summarises the children of one item: how many there are, and how many are done.
//
// A summary rather than the children themselves, and that is a deliberate shape. The roll-up asks one
// question - "is anything still open down there" - and a repository can answer it with a count, where
// handing up every child would mean reading a subtree to decide one boolean. It also keeps the decision
// a pure function of two numbers, which is what makes every policy table-testable without a database.
//
// Trashed children are not counted at all: they are deletions waiting out their retention period, and a
// work package whose last activity was deleted should not become done because of it. Archived children
// are counted as they stand - archiving is a decision to keep something quietly, not to disown it.
type ChildCompletion struct {
	Total     int
	Completed int
}

// AllCompleted reports whether nothing is left open. False for no children at all: "every child is
// done" of an empty set is vacuously true, and completing a parent on that basis would complete it the
// moment its last child was trashed.
func (c ChildCompletion) AllCompleted() bool { return c.Total > 0 && c.Completed >= c.Total }

// AnyOpen reports whether at least one child is still open.
func (c ChildCompletion) AnyOpen() bool { return c.Total > c.Completed }
