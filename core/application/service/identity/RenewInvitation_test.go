// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

type renewFixture struct {
	accounts *accountStore
	auth     *authorizer
	sink     *auditSink
	queued   *notifier
	minter   *linkMinter
	handler  RenewInvitation
}

func newRenewFixture(t *testing.T, status domain.AccountStatus, withoutMail bool) *renewFixture {
	t.Helper()
	invited, err := domain.Invite(invitedID, tenant, "bert@example.org", "Bert", nil, nil)
	if err != nil {
		t.Fatalf("preparing: %v", err)
	}
	invited.Status = status
	f := &renewFixture{
		accounts: newAccounts(invited), auth: &authorizer{}, sink: &auditSink{},
		queued: &notifier{}, minter: &linkMinter{},
	}
	f.handler = RenewInvitation{
		Accounts: f.accounts, Authorizer: f.auth, Notifier: f.queued, Audit: f.sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
	if withoutMail {
		f.handler.Links = InvitationLinks{Minter: f.minter, BaseURL: homeBase}
	}
	return f
}

func (f *renewFixture) renew(t *testing.T, actor appshared.ActorContext) (usecase.Output, error) {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, f.handler.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	return registry.Invoke(t.Context(), RenewInvitationName, actor, usecase.Input{"account_id": invitedID.String()})
}

// A renewal goes out the way the invitation did: the mail queued again, or a new link answered
// once - and the trail says which, never the link.
func TestARenewalSendsTheMailAgainOrAnswersANewLink(t *testing.T) {
	for _, withoutMail := range []bool{false, true} {
		f := newRenewFixture(t, domain.AccountInvited, withoutMail)

		out, err := f.renew(t, admin())
		if err != nil {
			t.Fatalf("without mail %v: renewing: %v", withoutMail, err)
		}

		_, linked := out["invitation_link"]
		if linked != withoutMail {
			t.Errorf("without mail %v: link answered %v", withoutMail, linked)
		}
		if withoutMail {
			if len(f.queued.requests) != 0 || len(f.minter.minted) != 1 {
				t.Errorf("queued %v, minted %v; want one token and no mail", f.queued.requests, f.minter.minted)
			}
		} else if len(f.queued.requests) != 1 || f.queued.requests[0].Kind != queue.KindInvitationEmail ||
			f.queued.requests[0].DedupeKey != invitedID.String() {
			t.Errorf("queued %v, want the invitation mail for the account", f.queued.requests)
		}
		if len(f.sink.entries) != 1 || f.sink.entries[0].Action != InvitationRenewedAction ||
			f.sink.entries[0].TargetID != invitedID {
			t.Fatalf("trail %v, want one renewal of the account", f.sink.entries)
		}
		if stringOf(f.sink.entries[0].Changes["delivery"]) == "" {
			t.Error("the trail does not say how the invitation went out")
		}
	}
}

// Only an invitation still waiting is renewed; everything else is refused before anything goes out.
func TestARenewalIsRefusedWhereNoInvitationWaits(t *testing.T) {
	cases := []struct {
		name        string
		status      domain.AccountStatus
		withoutMail bool
		actor       func() appshared.ActorContext
		refuse      error
		want        string
	}{
		{"a person who arrived", domain.AccountActive, false, admin, nil, "accounts.not_invited"},
		{"a person who arrived, no mail", domain.AccountActive, true, admin, nil, "accounts.not_invited"},
		{"a disabled account", domain.AccountDisabled, true, admin, nil, "accounts.not_invited"},
		{"a rule's run without mail", domain.AccountInvited, true, ruleRun, nil, "accounts.invitation_needs_a_person"},
		{"no permission to manage people", domain.AccountInvited, true, admin,
			shared.ErrForbidden.WithDetail("access.denied"), "access.denied"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRenewFixture(t, c.status, c.withoutMail)
			f.auth.refuse = c.refuse

			_, err := f.renew(t, c.actor())

			if got := detailOf(err); got != c.want {
				t.Fatalf("answered %q (%v), want %s", got, err, c.want)
			}
			if len(f.queued.requests) != 0 || len(f.minter.minted) != 0 || len(f.sink.entries) != 0 {
				t.Error("something went out for a refused renewal")
			}
		})
	}
}

// With mail a rule may renew like anybody else: the mail carries the invitation, not the answer.
func TestARuleRenewsWhereTheMailCarriesIt(t *testing.T) {
	f := newRenewFixture(t, domain.AccountInvited, false)
	if _, err := f.renew(t, ruleRun()); err != nil {
		t.Fatalf("renewing: %v", err)
	}
}

// An account the workspace does not hold is not found - the answer an identifier from elsewhere
// gets everywhere - and the permission is asked for the workspace before anything is read.
func TestARenewalOfAnUnknownAccountIsNotFound(t *testing.T) {
	f := newRenewFixture(t, domain.AccountInvited, true)
	f.accounts = newAccounts()
	f.handler.Accounts = f.accounts

	_, err := f.renew(t, admin())

	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("answered %v, want not found", err)
	}
	if len(f.auth.requests) != 1 || f.auth.requests[0].Permission != "MANAGE_MEMBERS" {
		t.Errorf("asked for %v, want the member management permission", f.auth.requests)
	}
}

// Accepted between the read and the mint: the minter stores nothing on an account no longer
// invited, and the renewal says so rather than answering a link to nothing.
func TestARenewalRacingTheAcceptanceIsNotInvited(t *testing.T) {
	f := newRenewFixture(t, domain.AccountInvited, true)
	f.handler.Links.Minter = acceptedMeanwhile{}

	_, err := f.renew(t, admin())

	if got := detailOf(err); got != "accounts.not_invited" {
		t.Fatalf("answered %q (%v), want accounts.not_invited", got, err)
	}
}
