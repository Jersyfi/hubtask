// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// Whoever holds a service account's token acts with every role the account holds, so a mint for
// one is held to the ceiling (identity.md §13): an owner mints for any service account, an
// administrator only for one holding no role above their own - before the proof an administrative
// scope demands is looked at, and with nothing stored on a refusal.
func TestAServiceAccountsTokenIsHeldToTheCeiling(t *testing.T) {
	for _, c := range ceilingCases {
		t.Run(c.name, func(t *testing.T) {
			machine := domain.Account{
				ID: serviceAccountID, TenantID: tenant, Kind: domain.AccountServiceAccount,
				DisplayName: "the nightly export", Status: domain.AccountActive,
			}
			accounts, auth, sink := newAccounts(machine), &authorizer{own: c.own}, &auditSink{}
			writer := tokenWriter(t, accounts, auth, sink)
			writer.Held = heldRoles{serviceAccountID: c.held}
			writer.KnownScopes = append(slices.Clone(buildScopes), "admin:tenants")
			proof := &stepUpFake{satisfied: true}
			writer.StepUp = proof
			command := validCommand()
			command.AccountID = serviceAccountID
			command.Scopes = []string{"items:read", "admin:tenants"}
			command.StepUpToken = "hbt_stu_fresh"
			store := writer.Tokens.(*tokens)

			_, err := CreateAccessToken{Writer: writer}.Execute(t.Context(), holder(), command)
			if c.issued {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				if len(store.minted) != 1 || len(proof.presented) != 1 {
					t.Error("minted without a row stored or the proof judged")
				}
				return
			}
			if got := detailOf(err); got != "access.role_above_own" {
				t.Fatalf("answered %q (%v), want access.role_above_own", got, err)
			}
			if len(proof.presented) != 0 {
				t.Error("the fresh proof was looked at before the ceiling refused")
			}
			if len(store.minted)+len(sink.entries) != 0 {
				t.Error("something was stored for a refused mint")
			}
			asked := auth.requests[len(auth.requests)-1]
			if asked.Grants != domain.RoleOwner || asked.Action != TokenCreatedAction || asked.TargetID != serviceAccountID {
				t.Errorf("the ceiling's question was %+v", asked)
			}
		})
	}
}

// One's own token hands nobody anything: no ceiling, whatever one holds.
func TestOnesOwnTokenIsNotHeldToTheCeiling(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{own: domain.RoleMember}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)
	writer.Held = heldRoles{adminID: {domain.RoleOwner}}

	if _, err := (CreateAccessToken{Writer: writer}).Execute(t.Context(), holder(), validCommand()); err != nil {
		t.Fatalf("minting one's own: %v", err)
	}
	if len(auth.requests) != 0 {
		t.Errorf("one's own token asked %+v", auth.requests)
	}
}

// Listing and revoking a service account's tokens hand out nothing, and revoking is never harder
// than minting: neither asks the ceiling.
func TestListingAServiceAccountsTokensIsNotHeldToTheCeiling(t *testing.T) {
	machine := domain.Account{
		ID: serviceAccountID, TenantID: tenant, Kind: domain.AccountServiceAccount,
		DisplayName: "the nightly export", Status: domain.AccountActive,
	}
	accounts, auth, sink := newAccounts(machine), &authorizer{own: domain.RoleAdmin}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)
	writer.Held = heldRoles{serviceAccountID: {domain.RoleOwner}}

	if _, err := (ListAccessTokens{Writer: writer}).Execute(t.Context(), holder(), serviceAccountID); err != nil {
		t.Fatalf("listing: %v", err)
	}
	for _, asked := range auth.requests {
		if asked.Grants != "" {
			t.Errorf("the listing asked the ceiling: %+v", asked)
		}
	}
}
