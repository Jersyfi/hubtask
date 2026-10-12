// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// linkMinter is MintRedemptionToken in memory: it answers a token named after the account and
// counts what it minted.
type linkMinter struct {
	minted []shared.ID
}

func (m *linkMinter) MintRedemptionToken(_ context.Context, _, accountID shared.ID) (secret.Secret, error) {
	m.minted = append(m.minted, accountID)
	return secret.New("hbt_inv_" + accountID.String()), nil
}

const homeBase = "https://home.example/"

func withoutMail(h InviteAccount, minter *linkMinter) InviteAccount {
	h.Links = InvitationLinks{Minter: minter, BaseURL: homeBase}
	return h
}

func ruleRun() appshared.ActorContext {
	actor := admin()
	actor.Kind, actor.RunsForRule = shared.ActorServiceAccount, "01936f2a-7c1e-7000-8000-0000000000e1"
	return actor
}

// UC-ID-14 check 5: the invitation goes out by mail exactly where mail is configured, and as a link
// answered once exactly where it is not - never both, because the mail's delivery mints a token of
// its own on every attempt and would kill the link at its first retry.
func TestAnInvitationGoesOutByMailOrByLinkNeverBoth(t *testing.T) {
	cases := []struct {
		name         string
		withoutMail  bool
		wantLink     bool
		wantDelivery string
	}{
		{"mail configured", false, false, DeliveryMail},
		{"no mail", true, true, DeliveryLink},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			accounts, sink, queued, minter := newAccounts(), &auditSink{}, &notifier{}, &linkMinter{}
			handler := inviteHandler(accounts, &authorizer{}, sink, queued)
			if c.withoutMail {
				handler = withoutMail(handler, minter)
			}

			invitation, err := handler.Execute(t.Context(), admin(), InviteAccountCommand{Email: "bert@example.org"})
			if err != nil {
				t.Fatalf("inviting: %v", err)
			}

			if got := !invitation.Link.IsEmpty(); got != c.wantLink {
				t.Errorf("link answered %v, want %v", got, c.wantLink)
			}
			if c.wantLink {
				want := "https://home.example/redeem#token=hbt_inv_" + invitedID.String()
				if invitation.Link.Reveal() != want {
					t.Errorf("link %q, want the mail's own composition %q", invitation.Link.Reveal(), want)
				}
				if len(queued.requests) != 0 {
					t.Errorf("queued %v beside the link", queued.requests)
				}
				if len(minter.minted) != 1 || minter.minted[0] != invitedID {
					t.Errorf("minted %v, want one token for the invited account", minter.minted)
				}
			} else {
				if len(queued.requests) != 1 || queued.requests[0].Kind != queue.KindInvitationEmail {
					t.Errorf("queued %v, want the invitation mail", queued.requests)
				}
				if len(minter.minted) != 0 {
					t.Error("a token was minted where the mail mints its own")
				}
			}

			delivery, _ := sink.entries[0].Changes["delivery"].(map[string]any)
			if delivery["to"] != c.wantDelivery {
				t.Errorf("the trail records delivery %v, want %s", delivery, c.wantDelivery)
			}
			for field, change := range sink.entries[0].Changes {
				if strings.Contains(stringOf(change), "hbt_inv_") {
					t.Errorf("the trail carries the link in %q", field)
				}
			}
		})
	}
}

// A rule's run reads no answer: without mail the link would reach nobody, so it is refused before
// anything is written. With mail the run invites like anybody else.
func TestARuleRunInvitesOnlyWhereTheMailCarriesIt(t *testing.T) {
	accounts, queued, minter := newAccounts(), &notifier{}, &linkMinter{}

	_, err := withoutMail(inviteHandler(accounts, &authorizer{}, &auditSink{}, queued), minter).
		Execute(t.Context(), ruleRun(), InviteAccountCommand{Email: "bert@example.org"})

	if got := detailOf(err); got != "accounts.invitation_needs_a_person" {
		t.Fatalf("answered %q (%v), want accounts.invitation_needs_a_person", got, err)
	}
	if len(accounts.inserted) != 0 || len(minter.minted) != 0 || len(queued.requests) != 0 {
		t.Error("a refused run wrote something")
	}

	if _, err := inviteHandler(newAccounts(), &authorizer{}, &auditSink{}, &notifier{}).
		Execute(t.Context(), ruleRun(), InviteAccountCommand{Email: "bert@example.org"}); err != nil {
		t.Errorf("a run with mail was refused: %v", err)
	}
}

// A token minted with no base address to carry it would be an invitation nobody can accept; the
// invitation fails rather than answering a dead link.
func TestNoLinkWithoutABaseAddress(t *testing.T) {
	handler := withoutMail(inviteHandler(newAccounts(), &authorizer{}, &auditSink{}, &notifier{}), &linkMinter{})
	handler.Links.BaseURL = ""

	_, err := handler.Execute(t.Context(), admin(), InviteAccountCommand{Email: "bert@example.org"})

	if got := detailOf(err); got != "accounts.invitation_link_unavailable" {
		t.Fatalf("answered %q (%v), want accounts.invitation_link_unavailable", got, err)
	}
}

// Through the registry, the way every door reaches it: the link is in the answer where it is
// handed over, and absent - not empty - where the mail carries it.
func TestTheInvitationsAnswerCarriesTheLinkOnlyWithoutMail(t *testing.T) {
	for _, withLinks := range []bool{false, true} {
		handler := inviteHandler(newAccounts(), &authorizer{}, &auditSink{}, &notifier{})
		if withLinks {
			handler = withoutMail(handler, &linkMinter{})
		}
		registry, err := usecase.NewRegistry(nil, handler.Descriptor())
		if err != nil {
			t.Fatalf("registering: %v", err)
		}

		out, err := registry.Invoke(t.Context(), InviteAccountName, admin(), usecase.Input{"email": "bert@example.org"})
		if err != nil {
			t.Fatalf("inviting: %v", err)
		}

		link, present := out["invitation_link"]
		if present != withLinks {
			t.Errorf("without mail %v: invitation_link present %v", withLinks, present)
		}
		if withLinks && !strings.HasPrefix(link.(string), "https://home.example/redeem#token=") {
			t.Errorf("link %v, want the redeem link", link)
		}
	}
}

func stringOf(change any) string {
	values, _ := change.(map[string]any)
	var b strings.Builder
	for _, v := range values {
		if s, ok := v.(string); ok {
			b.WriteString(s)
		}
	}
	return b.String()
}
