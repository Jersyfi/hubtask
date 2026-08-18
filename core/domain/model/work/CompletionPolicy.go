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

// ParseCompletionPolicy reads a stored or submitted policy.
//
// The empty value is the default rather than an error: the `policies` column starts as `{}`, so every
// collection that predates this policy has no value for it, and a read that failed on those would make
// the column impossible to introduce. Anything else that is not a known policy *is* an error - a value
// nobody recognises is not a reason to silently pick a behaviour.
func ParseCompletionPolicy(value string) (CompletionPolicy, error) {
	if value == "" {
		return DefaultCompletionPolicy, nil
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
