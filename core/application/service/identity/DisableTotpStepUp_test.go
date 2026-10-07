// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ADR-0075 §3, UC-ID-05 check 5, UC-ID-03 check 6: turning the second factor off takes the step-up
// like every privileged action, so an account that holds no password proves it with what it holds.
// A route that asked for the password in its own body would leave a provider-only person with a
// factor no way to turn it off at all.

func TestTurningTheFactorOffTakesTheStepUp(t *testing.T) {
	fixture := providerOnly(t)
	material := enrolled(t, fixture)

	// The session alone proves nothing: the demand, naming what this account holds - and the
	// factor stays.
	err := DisableTotp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(), DisableTotpCommand{})
	if methods := demandedMethods(t, err); methods != "TOTP RECOVERY" {
		t.Errorf("turning the factor off without a proof named %q, want TOTP RECOVERY", methods)
	}
	if _, err := fixture.writer.Enrollments.Find(t.Context(), account); err != nil {
		t.Fatalf("the factor is gone without a proof: %v", err)
	}

	// A code proves it through the step-up, and the factor goes.
	later := now.Add(domain.TotpStepSeconds * time.Second)
	writer := fixture.writer
	writer.Clock = clock.Fixed(later)
	grant, err := StepUp{Writer: writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{Code: domain.TotpCode(material, domain.TotpStep(later))})
	if err != nil {
		t.Fatalf("stepping up: %v", err)
	}
	if err := (DisableTotp{Writer: writer}).Execute(t.Context(), signedInActor(),
		DisableTotpCommand{StepUpToken: grant.Token.Reveal()}); err != nil {
		t.Fatalf("turning the factor off with the proof: %v", err)
	}
	if _, err := writer.Enrollments.Find(t.Context(), account); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the factor is still there: %v", err)
	}

	// The proof was spent by that one act.
	enrolled(t, fixture)
	err = DisableTotp{Writer: writer}.Execute(t.Context(), signedInActor(),
		DisableTotpCommand{StepUpToken: grant.Token.Reveal()})
	if err == nil || !strings.Contains(err.Error(), stepup.CodeRequired) {
		t.Errorf("a spent proof turned the factor off again: %v", err)
	}
}

// Somebody without the authenticator but with a recovery code turns it off with the code.
func TestARecoveryCodeTurnsTheFactorOff(t *testing.T) {
	fixture := providerOnly(t)
	enrolled(t, fixture)

	grant, err := StepUp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{RecoveryCode: secret.New(liveRecoveryCode(t, fixture))})
	if err != nil {
		t.Fatalf("stepping up with a recovery code: %v", err)
	}
	if err := (DisableTotp{Writer: fixture.writer}).Execute(t.Context(), signedInActor(),
		DisableTotpCommand{StepUpToken: grant.Token.Reveal()}); err != nil {
		t.Fatalf("turning the factor off: %v", err)
	}
}

// A workspace that requires the factor refuses before the proof is spent: the person keeps the
// proof for whatever they meant to do next, and learns the rule rather than that a proof failed.
func TestTheRuleRefusesBeforeTheProofIsSpent(t *testing.T) {
	fixture := armedUnder(t, domain.MfaForEveryone, domain.RoleMember)
	session := fixture.session
	session.writer.StepUps = newStepUps()
	credential, _ := refreshCredential(now)
	session.sessions.sessions[sessionRowID] = repository.SessionCredential{
		TenantStatus: domain.TenantActive, Session: credential.Session, Account: credential.Account,
	}
	grant, err := StepUp{Writer: session.writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{Password: secret.New("correct horse battery")})
	if err != nil {
		t.Fatalf("stepping up: %v", err)
	}

	err = DisableTotp{Writer: session.writer}.Execute(t.Context(), signedInActor(),
		DisableTotpCommand{StepUpToken: grant.Token.Reveal()})
	if !errors.Is(err, shared.ErrForbidden) || !strings.Contains(err.Error(), "auth.mfa_required_by_tenant") {
		t.Fatalf("under Everyone the proof turned the factor off: %v", err)
	}
	if ok, err := (StepUpVerifier{Writer: session.writer}).Satisfied(
		t.Context(), account, grant.Token.Reveal()); err != nil || !ok {
		t.Errorf("the refusal spent the proof (%v, %v)", ok, err)
	}
}

// The body's password is the proof this route took before; it is accepted for one release.
func TestTheDeprecatedPasswordStillTurnsTheFactorOff(t *testing.T) {
	fixture := stepUpFixture(now)
	enrolled(t, fixture)
	if err := (DisableTotp{Writer: fixture.writer}).Execute(t.Context(), signedInActor(),
		DisableTotpCommand{Password: secret.New("correct horse battery")}); err != nil {
		t.Fatalf("the deprecated password was refused: %v", err)
	}
}
