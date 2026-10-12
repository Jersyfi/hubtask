// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-ID-20 check 7, its second half: whoever holds the mailbox follows the link, the account takes
// the address and is an ordinary one from then on - a new start password no longer exists for it.

func (f *addressFixture) confirmRegistry(t *testing.T) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, ConfirmAddress{
		Requests: f.store, Audit: f.audit, UnitOfWork: &unitOfWork{}, Clock: f.clock,
	}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	return registry
}

// mailedLink asks for an address as Lena and mints the link its mail would carry.
func (f *addressFixture) mailedLink(t *testing.T, email string) string {
	t.Helper()
	if _, err := f.request(t, lena(), lenaID, email); err != nil {
		t.Fatalf("asking: %v", err)
	}
	request := f.store.requests[len(f.store.requests)-1]
	link, err := MintAddressLink{
		Requests: f.store, Entropy: &countingEntropy{drawn: byte(len(f.store.requests))},
		UnitOfWork: &unitOfWork{}, Clock: f.clock,
	}.MintAddressLink(t.Context(), tenant, request.credential.ID)
	if err != nil || link.Address != email {
		t.Fatalf("minting answered %+v, %v", link, err)
	}
	return link.Token.Reveal()
}

func confirmWith(t *testing.T, registry *usecase.Registry, token string) error {
	t.Helper()
	_, err := registry.Invoke(t.Context(), ConfirmAddressName, appshared.Anonymous("en", "UTC"),
		usecase.Input{"token": token})
	return err
}

func TestFollowingTheLinkMakesTheAccountAnOrdinaryOne(t *testing.T) {
	f := newAddressFixture(t)
	token := f.mailedLink(t, "lena@example.org")

	if err := confirmWith(t, f.confirmRegistry(t), token); err != nil {
		t.Fatalf("confirming: %v", err)
	}
	account := f.accounts.byID[lenaID]
	if account.Email != "lena@example.org" || account.SignInName != "lena" {
		t.Errorf("the account reads %+v, want the address and the name kept", account)
	}
	last := f.audit.entries[len(f.audit.entries)-1]
	if last.Action != AddressConfirmedAction || last.TargetID != lenaID {
		t.Errorf("the trail's last entry is %+v", last)
	}

	// The action of check 5 no longer exists for her.
	renewal := newRenewalFixture(t)
	renewal.accounts.byID[lenaID] = account
	if _, err := renewal.renew(t, admin(), renewalOf(lenaID)); detailOf(err) != "accounts.not_managed" {
		t.Errorf("a new start password after the upgrade answered %v", err)
	}
}

// Unknown, spent, replaced, expired and malformed links are one refusal.
func TestALinkThatCannotConfirmIsOneRefusal(t *testing.T) {
	cases := map[string]func(t *testing.T, f *addressFixture) string{
		"malformed": func(*testing.T, *addressFixture) string { return "not-a-token" },
		"spent": func(t *testing.T, f *addressFixture) string {
			token := f.mailedLink(t, "lena@example.org")
			if err := confirmWith(t, f.confirmRegistry(t), token); err != nil {
				t.Fatalf("the first use: %v", err)
			}
			return token
		},
		"expired": func(t *testing.T, f *addressFixture) string {
			token := f.mailedLink(t, "lena@example.org")
			f.clock.at = now.Add(24 * time.Hour)
			return token
		},
		"replaced by a newer one": func(t *testing.T, f *addressFixture) string {
			token := f.mailedLink(t, "lena@example.org")
			f.clock.at = now.Add(time.Hour)
			f.mailedLink(t, "lena@elsewhere.example")
			return token
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			f := newAddressFixture(t)
			token := arrange(t, f)
			err := confirmWith(t, f.confirmRegistry(t), token)
			if detailOf(err) != "auth.address_link_invalid" || shared.AsError(err).Category != shared.CategoryUnauthenticated {
				t.Errorf("answered %v", err)
			}
		})
	}
}

// A link one minute before its day ends still confirms.
func TestALinkConfirmsUntilItsDayEnds(t *testing.T) {
	f := newAddressFixture(t)
	token := f.mailedLink(t, "lena@example.org")
	f.clock.at = now.Add(24*time.Hour - time.Minute)
	if err := confirmWith(t, f.confirmRegistry(t), token); err != nil {
		t.Errorf("a link within its day answered %v", err)
	}
}

// An address somebody here holds by now is told to whoever holds the mailbox.
func TestAnAddressTakenMeanwhileIsAConflict(t *testing.T) {
	f := newAddressFixture(t)
	token := f.mailedLink(t, "bert@example.org")
	f.store.taken["bert@example.org"] = true
	if err := confirmWith(t, f.confirmRegistry(t), token); detailOf(err) != "accounts.address_taken" {
		t.Errorf("answered %v", err)
	}
	if f.accounts.byID[lenaID].Email != "" {
		t.Error("the account took an address another holds")
	}
}

// The tenant header may confirm the link's workspace, never overrule it.
func TestTheLinksWorkspaceIsNotOverruled(t *testing.T) {
	f := newAddressFixture(t)
	token := f.mailedLink(t, "lena@example.org")
	_, err := f.confirmRegistry(t).Invoke(t.Context(), ConfirmAddressName, appshared.Anonymous("en", "UTC"),
		usecase.Input{"token": token, "tenant_header": "01936f2a-7c1e-7000-8000-0000000000ee"})
	if detailOf(err) != "access.tenant_mismatch" {
		t.Errorf("answered %v", err)
	}
}
