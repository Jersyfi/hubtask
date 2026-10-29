// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// UC-INS-19 check 6: what a person keeps through a change of address is read as the step-up and
// the sign-in read it - the password, the app, and the rule's demand for a factor.
func TestLastingWaysReadTheAccountsWaysAndTheRule(t *testing.T) {
	fixture := armedUnder(t, domain.MfaForEveryone, domain.RoleMember)
	held, err := LastingWays{Writer: fixture.session.writer}.WaysOf(t.Context(), tenant, account)
	if err != nil {
		t.Fatalf("reading the ways: %v", err)
	}
	if !held.Password || !held.App || !held.FactorRequired || held.Lacking() != "" {
		t.Fatalf("ways %+v", held)
	}
}

// UC-INS-19 check 5: the proof answers the method that gave it, so a change of address can refuse a
// passkey and accept the password.
func TestTheVerifierAnswersTheMethodThatProvedIt(t *testing.T) {
	fixture := armedUnder(t, domain.MfaForNobody, domain.RoleMember)
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
	method, ok, err := StepUpVerifier{Writer: session.writer}.SatisfiedBy(t.Context(), account, grant.Token.Reveal())
	if err != nil || !ok || method != stepupport.MethodPassword {
		t.Fatalf("SatisfiedBy = %q, %v, %v", method, ok, err)
	}
}
