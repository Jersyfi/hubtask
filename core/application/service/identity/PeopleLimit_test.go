// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-10 check 5 (multi-tenancy.md §4.2): at the people limit, every door that creates a person
// refuses with the limit named and writes nothing - an invitation, its renewal, a managed account,
// and a provider's newcomer, at once or after the terms.

// peopleFake is the people limit: full refuses as the quota guard does; asked records each call.
type peopleFake struct {
	full  bool
	asked []int64
}

func (p *peopleFake) Renewal(ctx context.Context, _, _ string) error {
	return p.Accounts(ctx, "", -1)
}

func (p *peopleFake) Accounts(_ context.Context, _ string, adding int64) error {
	p.asked = append(p.asked, adding)
	if p.full {
		return shared.ErrValidation.WithDetail("capacity.accounts").
			WithParams(map[string]string{"quota": "accounts", "limit": "6", "used": "6"})
	}
	return nil
}

func TestAnInvitationAtThePeopleLimitIsRefused(t *testing.T) {
	accounts, sink, queued := newAccounts(), &auditSink{}, &notifier{}
	handler := inviteHandler(accounts, &authorizer{}, sink, queued)
	people := &peopleFake{full: true}
	handler.People = people

	_, err := handler.Execute(t.Context(), admin(), InviteAccountCommand{Email: "bert@example.org"})
	if detailOf(err) != "capacity.accounts" {
		t.Fatalf("an invitation at the limit answered %v, want capacity.accounts", err)
	}
	if len(accounts.inserted) != 0 || len(queued.requests) != 0 || len(sink.entries) != 0 {
		t.Error("an invitation refused at the limit wrote something")
	}

	people.full = false
	if _, err := handler.Execute(t.Context(), admin(), InviteAccountCommand{Email: "bert@example.org"}); err != nil {
		t.Fatalf("an invitation with room answered %v", err)
	}
	if len(people.asked) != 2 || people.asked[1] != 1 {
		t.Errorf("the limit was asked %v, want one more person each time", people.asked)
	}
}

func TestARenewalAtThePeopleLimitIsRefused(t *testing.T) {
	f := newRenewFixture(t, domain.AccountInvited, false)
	people := &peopleFake{full: true}
	f.handler.People = people

	if _, err := f.renew(t, admin()); detailOf(err) != "capacity.accounts" {
		t.Fatalf("a renewal at the limit answered %v, want capacity.accounts", err)
	}
	// Asked as a renewal - whether it adds anybody is the limit's to tell from the link - and before
	// the new link went out.
	if len(people.asked) != 1 || people.asked[0] != -1 {
		t.Errorf("the limit was asked %v, want one renewal", people.asked)
	}
	if len(f.queued.requests) != 0 {
		t.Error("the refused renewal queued its mail")
	}
}

func TestAManagedAccountAtThePeopleLimitIsRefused(t *testing.T) {
	f := newManagedFixture(t)
	f.people.full = true

	if _, err := f.create(t, admin(), lenaInput()); detailOf(err) != "capacity.accounts" {
		t.Fatalf("a managed account at the limit answered %v, want capacity.accounts", err)
	}
	if len(f.accounts.inserted) != 0 || len(f.grants.granted) != 0 {
		t.Error("a managed account refused at the limit wrote something")
	}
}

func TestAProvidersNewcomerAtThePeopleLimitIsTurnedAway(t *testing.T) {
	f := newOidcFixture(t, now)
	f.writer.People = &peopleFake{full: true}

	_, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(),
		CompleteOidcSignInCommand{Code: "the-code", State: start(t, f)})
	if detailOf(err) != "capacity.accounts" {
		t.Fatalf("a newcomer at the limit answered %v, want capacity.accounts", err)
	}
	if len(f.accounts.inserted) != 0 || len(f.external.links) != 0 {
		t.Error("a newcomer refused at the limit left an account or a link")
	}
	if !containsAction(auditActions(f.session.audit.entries), OidcRefusedAction) {
		t.Error("the newcomer turned away at the limit is not recorded")
	}
}

// Where terms are in force, the limit is asked before the terms are put - nobody agrees for a place
// there is not - and again when the account is made after the agreement.
func TestANewcomerAfterTheTermsIsHeldToThePeopleLimitTwice(t *testing.T) {
	f, _, _ := newcomerFixture(t)
	people := &peopleFake{full: true}
	f.writer.People = people
	_, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(),
		CompleteOidcSignInCommand{Code: "the-code", State: start(t, f)})
	if detailOf(err) != "capacity.accounts" {
		t.Fatalf("a newcomer at the limit was put the terms: %v", err)
	}

	people.full = false
	token := arrives(t, f)
	people.full = true
	if _, err := agreesAsNewcomer(t, f, token, "3"); detailOf(err) != "capacity.accounts" {
		t.Fatalf("an agreement after the last place went answered %v, want capacity.accounts", err)
	}
	if len(f.accounts.inserted) != 0 {
		t.Error("the agreement at the limit made an account")
	}
}
