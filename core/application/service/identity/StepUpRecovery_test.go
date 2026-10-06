// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ADR-0075 §1, UC-ID-03 check 6: a recovery code proves a step-up, for somebody whose authenticator
// is not at hand - and it is consumed by the step-up exactly as by a sign-in, so it proves one act
// once.

// liveRecoveryCode answers one of the account's codes as the store holds it.
func liveRecoveryCode(t *testing.T, fixture *sessionFixture) string {
	t.Helper()
	for code, live := range fixture.writer.Recovery.(*recoveryStore).codes {
		if live {
			return code
		}
	}
	t.Fatal("the account holds no live recovery code")
	return ""
}

func TestARecoveryCodeProvesAStepUpOnceAndIsConsumed(t *testing.T) {
	fixture := stepUpFixture(now)
	enrolled(t, fixture)
	codes := fixture.writer.Recovery.(*recoveryStore)
	before, _ := codes.Remaining(t.Context(), account)
	code := liveRecoveryCode(t, fixture)

	grant, err := StepUp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{RecoveryCode: secret.New(code)})
	if err != nil {
		t.Fatalf("stepping up with a recovery code: %v", err)
	}
	if grant.Method != "RECOVERY" {
		t.Errorf("method %q, want RECOVERY", grant.Method)
	}
	if after, _ := codes.Remaining(t.Context(), account); after != before-1 {
		t.Errorf("%d codes left after the step-up, want %d - the code was not consumed", after, before-1)
	}
	// The trail says a code was spent, as a sign-in with one does.
	if !auditRecorded(fixture.audit, RecoveryCodeUsedAction) {
		t.Error("the consumed code is not in the trail")
	}

	// The same code a second time proves nothing, and counts against the ledger.
	_, err = StepUp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{RecoveryCode: secret.New(code)})
	if err == nil || !strings.Contains(err.Error(), "auth.mfa_code_invalid") {
		t.Fatalf("a spent recovery code answered %v", err)
	}
	if got := fixture.attempts.standing[stepUpSubject(account)].Failures; got != 1 {
		t.Errorf("the ledger stands at %d, want 1", got)
	}
}

// A stolen session alone proves nothing: without a proof the demand refuses, naming RECOVERY only
// while a code is left - a prompt for a code nobody holds is a prompt nobody can answer.
func TestTheDemandNamesRecoveryWhileACodeIsLeft(t *testing.T) {
	fixture := stepUpFixture(now)
	verifier := StepUpVerifier{Writer: fixture.writer}
	actor := signedInActor()
	enrolled(t, fixture)

	err := stepup.Demand(t.Context(), verifier, actor.TenantID, actor.AccountID, "")
	if methods := demandedMethods(t, err); methods != "PASSWORD TOTP RECOVERY" {
		t.Errorf("with codes left: %q, want PASSWORD TOTP RECOVERY", methods)
	}

	codes := fixture.writer.Recovery.(*recoveryStore)
	for code := range codes.codes {
		codes.codes[code] = false
	}
	err = stepup.Demand(t.Context(), verifier, actor.TenantID, actor.AccountID, "")
	if methods := demandedMethods(t, err); methods != "PASSWORD TOTP" {
		t.Errorf("with every code spent: %q, want PASSWORD TOTP", methods)
	}
}

// One method per request, the recovery code counted among them.
func TestARecoveryCodeBesideAnotherMethodIsRefused(t *testing.T) {
	fixture := stepUpFixture(now)
	_, err := StepUp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(), StepUpCommand{
		Code: "123456", RecoveryCode: secret.New("abcd-efgh"),
	})
	if err == nil || !strings.Contains(err.Error(), "auth.step_up_method_required") {
		t.Fatalf("two methods answered %v", err)
	}
}

// auditRecorded reports whether the trail holds an entry of this action.
func auditRecorded(sink *auditSink, action audit.Action) bool {
	for _, entry := range sink.entries {
		if entry.Action == action {
			return true
		}
	}
	return false
}
