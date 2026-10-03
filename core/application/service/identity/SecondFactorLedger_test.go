// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// A wrong second-factor proof advances the attempt ledger at every door (T-02, #1117, SC-22), so
// that guessing six digits meets the lockout curve. The fixture's unit of work rolls the ledger back
// when the work fails, as the database does - a failure recorded inside the refusing transaction is
// a failure that never lands, and every test here would have shown it.

// ledger reads the failures on one subject.
func ledger(fixture *sessionFixture, subject string) int {
	return fixture.attempts.standing[subject].Failures
}

func TestAWrongCodeAtTheSignInsSecondStepCounts(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	enrolled(t, fixture)

	_, _, err := CompleteSignIn{Writer: fixture.writer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: signInChallenge(t, fixture), Code: "000000",
	})
	if err == nil || !strings.Contains(err.Error(), "auth.mfa_code_invalid") {
		t.Fatalf("a wrong code answered %v", err)
	}
	if got := ledger(fixture, mfaSubject(account)); got != 1 {
		t.Errorf("after a wrong code the ledger stands at %d, want 1", got)
	}

	_, _, err = CompleteSignIn{Writer: fixture.writer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: signInChallenge(t, fixture), RecoveryCode: secret.New("AAAA-BBBB-CCCC-DDDD"),
	})
	if err == nil {
		t.Fatal("a wrong recovery code completed a sign-in")
	}
	if got := ledger(fixture, mfaSubject(account)); got != 2 {
		t.Errorf("after a wrong recovery code the ledger stands at %d, want 2", got)
	}
}

func TestAWrongCodeConfirmingAnEnrolmentCounts(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	if _, err := (EnrollTotp{Writer: fixture.writer}).Execute(t.Context(), signedInActor(), EnrollTotpCommand{}); err != nil {
		t.Fatalf("enrolling: %v", err)
	}

	_, err := ConfirmTotp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(), ConfirmTotpCommand{Code: "000000"})
	if err == nil || !strings.Contains(err.Error(), "auth.mfa_code_invalid") {
		t.Fatalf("a wrong code answered %v", err)
	}
	if got := ledger(fixture, mfaSubject(account)); got != 1 {
		t.Errorf("the ledger stands at %d, want 1", got)
	}
}

func TestAWrongPasswordAtTheLinkStepCounts(t *testing.T) {
	f := linkFixture(t, false)
	result, err := arrive(t, f)
	if err != nil || result.Challenge == nil {
		t.Fatalf("arriving: (%+v, %v)", result, err)
	}

	_, err = CompleteLink{Writer: f.writer}.Execute(t.Context(), CompleteLinkCommand{
		PendingToken: result.Challenge.Token, Password: secret.New("not the password at all"),
	})
	if err == nil {
		t.Fatal("a wrong password connected the provider")
	}
	if got := ledger(f.session, linkSubject(account)); got != 1 {
		t.Errorf("the ledger stands at %d, want 1", got)
	}
}

func TestAWrongCodeConfirmingAReplacementCounts(t *testing.T) {
	fixture, old := replacing(t)
	if _, err := (StartAuthenticatorReplacement{Writer: fixture.writer}).Execute(t.Context(), signedInActor(),
		StartAuthenticatorReplacementCommand{StepUpToken: proof(t, fixture)}); err != nil {
		t.Fatalf("beginning: %v", err)
	}
	at := now.Add(domain.TotpStepSeconds * time.Second)
	confirmer := fixture.writer
	confirmer.Clock = clock.Fixed(at)
	before := ledger(fixture, mfaSubject(account))

	if _, err := (ConfirmAuthenticatorReplacement{Writer: confirmer}).Execute(t.Context(), signedInActor(),
		ConfirmAuthenticatorReplacementCommand{Code: domain.TotpCode(old, domain.TotpStep(at))}); err == nil {
		t.Fatal("the old authenticator's code confirmed the new one")
	}
	if got := ledger(fixture, mfaSubject(account)); got != before+1 {
		t.Errorf("the ledger stands at %d, want %d", got, before+1)
	}
}

func TestAWrongCodeAtTheStepUpCounts(t *testing.T) {
	fixture := stepUpFixture(now)
	material := enrolled(t, fixture)

	_, err := StepUp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{Code: domain.TotpCode(material, domain.TotpStep(now)+1000)})
	if err == nil {
		t.Fatal("a wrong code proved a step-up")
	}
	if got := ledger(fixture, stepUpSubject(account)); got != 1 {
		t.Errorf("the ledger stands at %d, want 1", got)
	}
}
