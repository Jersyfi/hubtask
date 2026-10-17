// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/stepup"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// The purchase platform's scope is the control plane's in how it is handed out (UC-INS-06 check 6):
// minted behind a fresh proof by somebody the register names, and dropped from a token whose holder
// the register no longer names.

func TestTheProvisioningScopeIsMintedOnlyByARegisteredOperatorBehindAProof(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)
	writer.KnownScopes = append(slices.Clone(buildScopes), "admin:provisioning")
	proof := &stepUpFake{}
	writer.StepUp = proof
	command := validCommand()
	command.Scopes = []string{"admin:provisioning"}

	if _, err := (CreateAccessToken{Writer: writer}).Execute(t.Context(), holder(), command); detailOf(err) != stepup.CodeRequired {
		t.Errorf("a mint without a proof answered %v, want the step-up demand", err)
	}

	proof.satisfied = true
	command.StepUpToken = "hbt_stu_fresh"
	writer.Operators = &operatorRegister{accounts: map[shared.ID]bool{}}
	if _, err := (CreateAccessToken{Writer: writer}).Execute(t.Context(), holder(), command); detailOf(err) != "admin.operator_required" {
		t.Errorf("a mint outside the register answered %v, want admin.operator_required", err)
	}

	writer.Operators = &operatorRegister{accounts: map[shared.ID]bool{holder().AccountID: true}}
	minted, err := CreateAccessToken{Writer: writer}.Execute(t.Context(), holder(), command)
	if err != nil {
		t.Fatalf("minting with the proof, in the register: %v", err)
	}
	if !slices.Equal(minted.Token.Scopes, []string{"admin:provisioning"}) {
		t.Errorf("the token carries %v", minted.Token.Scopes)
	}
}

func TestTheRegisterBoundsTheProvisioningScope(t *testing.T) {
	raw, credential := mintCredential(t)
	credential.Token.Scopes = []string{"items:read", "admin:provisioning"}
	credential.Account.Kind = domain.AccountServiceAccount
	handler := handlerFor(&tokens{credential: credential}, &unitOfWork{})
	handler.Operators = &operatorRegister{accounts: map[shared.ID]bool{}}

	actor, err := handler.Execute(t.Context(), AuthenticateTokenCommand{
		Credential: raw, FallbackLocale: "en", FallbackTimeZone: "UTC",
	})
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}
	if actor.HasScope("admin:provisioning") || !actor.HasScope("items:read") {
		t.Fatalf("the actor carries %v, want items:read alone", actor.Scopes)
	}
	var domainErr *shared.Error
	err = actor.RequireAnyScope("admin:tenants", "admin:provisioning")
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "access.operator_register_required" ||
		domainErr.Params["scope"] != "admin:provisioning" {
		t.Fatalf("refusal = %v, want the register naming admin:provisioning", err)
	}
}

// A raised session gains the whole control plane and not its subset beside it: the hour is a person
// changing a switch, and the narrow scope would add nothing.
func TestARaisedSessionGainsTheWholeControlPlaneAlone(t *testing.T) {
	sessions := newSessionsStore()
	credential, _ := refreshCredential(now)
	credential.Session.ElevatedUntil = now.Add(30 * time.Minute)
	sessions.sessions[sessionRowID] = repository.SessionCredential{
		TenantStatus: domain.TenantActive, Session: credential.Session, Account: credential.Account,
	}
	signer := validatingSigner{token: "hbt_sat_x", claims: cryptoport.SessionClaims{
		TenantID: tenant, SessionID: sessionRowID, AccountID: account,
		ExpiresAt: now.Add(10 * time.Minute),
	}}
	authenticate := sessionAuthenticator(sessions, signer, now)
	authenticate.Operators = &operatorRegister{accounts: map[shared.ID]bool{account: true}}

	actor, err := authenticate.Execute(t.Context(), AuthenticateTokenCommand{Credential: "hbt_sat_x"})
	if err != nil {
		t.Fatalf("authenticating: %v", err)
	}
	if !actor.HasScope("admin:tenants") || actor.HasScope("admin:provisioning") {
		t.Errorf("the raised session carries %v, want admin:tenants and not its subset", actor.Scopes)
	}
}
