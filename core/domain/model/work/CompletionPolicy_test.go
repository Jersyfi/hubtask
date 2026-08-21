// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The empty value is the default rather than an error. The `policies` column starts as `{}`, so every
// collection that predates this policy has no value for it - a read that failed on those would make the
// column impossible to introduce at all.
func TestAnUnsetPolicyReadsAsTheDefault(t *testing.T) {
	policy, err := work.ParseCompletionPolicy("")
	if err != nil {
		t.Fatalf("an unset policy answered %v", err)
	}
	if policy != work.DefaultCompletionPolicy {
		t.Errorf("an unset policy read as %q, want %q", policy, work.DefaultCompletionPolicy)
	}
	// The default is the behaviour that surprises nobody: an item changes when somebody changes it.
	if work.DefaultCompletionPolicy != work.CompletionManual {
		t.Errorf("the default policy is %q", work.DefaultCompletionPolicy)
	}
}

func TestAKnownPolicyReadsBack(t *testing.T) {
	for _, want := range work.CompletionPolicies() {
		t.Run(string(want), func(t *testing.T) {
			policy, err := work.ParseCompletionPolicy(string(want))
			if err != nil {
				t.Fatalf("reading %q: %v", want, err)
			}
			if policy != want {
				t.Errorf("read back as %q", policy)
			}
		})
	}
}

// A value nobody recognises is not a reason to silently pick a behaviour: the collection would then be
// working one way while its configuration said another.
func TestAnUnknownPolicyIsRefused(t *testing.T) {
	_, err := work.ParseCompletionPolicy("ROLL_UP_SOMETIMES")
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("an unknown policy answered %v", err)
	}
	if got := shared.AsError(err).DetailCode; got != "containers.completion_policy_unknown" {
		t.Errorf("the detail code is %q", got)
	}
}

// RollsUp is the one question the completion service asks, so adding a policy that also rolls up is a
// change to one line rather than to every comparison against ROLLUP.
func TestOnlyRollupRollsUp(t *testing.T) {
	if !work.CompletionRollup.RollsUp() {
		t.Error("ROLLUP does not roll up")
	}
	if work.CompletionManual.RollsUp() {
		t.Error("MANUAL rolls up")
	}
	// An unknown policy never rolls up. It cannot be stored - ParseCompletionPolicy refuses it - and
	// the safe reading of a value nobody understands is to change nothing.
	if work.CompletionPolicy("SOMETHING_ELSE").RollsUp() {
		t.Error("an unknown policy rolls up")
	}
}

// A new collection behaves as the default rather than as the zero value, so that nothing above the
// constructor has to know that "" and MANUAL are the same thing.
func TestANewContainerCarriesTheDefaultPolicy(t *testing.T) {
	container, err := work.NewContainer(baseColl)
	if err != nil {
		t.Fatalf("building the container: %v", err)
	}
	if container.CompletionPolicy != work.DefaultCompletionPolicy {
		t.Errorf("a new container carries the policy %q", container.CompletionPolicy)
	}
}
