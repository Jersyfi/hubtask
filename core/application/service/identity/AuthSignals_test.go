// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// A step a sign-in is handed is counted by its step, and only the step: the label set is the
// closed set of purposes, never a subject (rule 10, observability-reliability.md §3.2).
func TestAStepASignInIsHandedIsCountedByItsStep(t *testing.T) {
	fixture := stepWithFactors(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{
		MfaRequiredFor: requirementOf(domain.MfaForEveryone),
	}
	signals := &signalsFake{}
	fixture.session.writer.Signals = signals

	if challenge := fixture.signsIn(t, "correct horse battery").Challenge; challenge == nil {
		t.Fatal("the rule demanded a factor and the sign-in owed no step")
	}
	if !slices.Equal(signals.steps, []string{"enroll"}) {
		t.Errorf("the steps counted are %q, want one enroll", signals.steps)
	}
}

// Every forgotten-password request is counted alike: a counter that moved only for an address
// with an account would answer the question the request is built not to answer (T-02).
func TestEveryForgottenPasswordRequestIsCountedAlike(t *testing.T) {
	for name, arrange := range map[string]func(*resetRequestFixture){
		"an account": func(*resetRequestFixture) {}, "no account": unheld,
	} {
		fixture := newResetRequestFixture()
		arrange(fixture)
		signals := &signalsFake{}
		fixture.writer.Session.Signals = signals

		if err := fixture.forget().Execute(t.Context(), ForgetPasswordCommand{
			Email: "bert@example.org", TenantSlug: "acme",
		}); err != nil {
			t.Fatalf("%s: refused: %v", name, err)
		}
		if signals.resets != 1 || len(signals.steps) != 0 {
			t.Errorf("%s: counted %d requests and steps %q, want one request and no step", name, signals.resets, signals.steps)
		}
	}
}
