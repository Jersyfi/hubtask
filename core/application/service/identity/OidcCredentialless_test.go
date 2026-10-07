// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
)

// An ACTIVE account that holds no credential at all - no password, no second factor, no provider
// identity, as after its provider was removed and its identity went with it - has nothing to prove
// on the card. Were the provider's word to connect it and open a session in every mode, an
// administrator's own issuer minting a member's address would sign in as that member (P-02,
// UC-ID-10 checks 5 and 6). It is connected only by a provider authoritative for the address -
// the mailbox's host vouching is a mailbox proof (ADR-0078 §1, §5) - and otherwise refused with the
// sentence that names the way back: *Forgot your password?*.

func credentiallessMember() domain.Account {
	return domain.Account{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e3"), TenantID: tenant,
		Kind: domain.AccountUser, Email: "cleo@example.org", DisplayName: "Cleo",
		Status: domain.AccountActive,
	}
}

func TestACredentiallessAccountIsConnectedOnlyByAnAuthoritativeProvider(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	for _, mode := range []domain.Provisioning{
		domain.ProvisionAny, domain.ProvisionDomains, domain.ProvisionInvitedOnly,
	} {
		for _, authoritative := range []bool{true, false} {
			name := string(mode) + "/not authoritative"
			if authoritative {
				name = string(mode) + "/authoritative"
			}
			t.Run(name, func(t *testing.T) {
				member := credentiallessMember()
				f := newOidcFixture(t, at, member)
				f.store.rows[0].Provisioning = mode
				f.relying.identity = provider.Identity{
					Subject: "an-issuer-subject", Email: member.Email, EmailVerified: true,
					AddressAuthoritative: authoritative, DisplayName: "Cleo",
				}

				result, err := complete(t, f, start(t, f))
				if authoritative {
					if err != nil {
						t.Fatalf("the authoritative arrival answered %v", err)
					}
					if pairOf(result).Session.AccountID != member.ID {
						t.Errorf("the session belongs to %q, want the member", pairOf(result).Session.AccountID)
					}
					if len(f.external.links) != 1 {
						t.Errorf("the member is connected as %v", f.external.links)
					}
					return
				}

				if detailOf(err) != "identity_provider.link_needs_mailbox" {
					t.Fatalf("the arrival answered %v, want the way back by mail", err)
				}
				if result.Pair != nil || result.Challenge != nil {
					t.Errorf("a refused arrival answered %+v", result)
				}
				if len(f.external.links) != 0 || len(f.accounts.inserted) != 0 {
					t.Errorf("a refused arrival linked %v or created %d accounts",
						f.external.links, len(f.accounts.inserted))
				}
				// Stored although the arrival's transaction rolled back: the fixture's unit of work
				// rolls the trail back with it.
				if !containsAction(auditActions(f.session.audit.entries), OidcRefusedAction) {
					t.Error("the refusal is not in the trail")
				}
			})
		}
	}
}
