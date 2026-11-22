// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// seedLocaliserFake records the language a redemption asked the seed to be named in, and answers
// it as admin.LocaliseSeed does: "" for a language no catalogue serves, or where the redeemer has
// no seed of their own - a member or a guest, who joins a workspace whose seed is somebody else's.
type seedLocaliserFake struct {
	served  map[string]bool
	unowned bool
	calls   int
	locale  string
	ownerID string
}

func (s *seedLocaliserFake) LocaliseSeed(
	_ context.Context, owner domain.Account, locale string, _ time.Time,
) (string, error) {
	s.calls++
	s.locale, s.ownerID = locale, owner.ID.String()
	if s.unowned || !s.served[locale] {
		return "", nil
	}
	return locale, nil
}

func (s *seedLocaliserFake) Serves(locale string) string {
	if !s.served[locale] {
		return ""
	}
	return locale
}

// UC-ID-07/8, UC-ID-07/9 and UC-ID-07/10, i18n-l10n.md §2: the owner's seed is named in the
// account's own language, else in the one the invitation card was read in. That one becomes the
// account's for every invited person without one - owner, member or guest - in the redemption's
// transaction. A language the account already has is kept; an empty, malformed or unserved one
// stores nothing and leaves the workspace's language.
func TestARedemptionKeepsTheCardsLanguageForEveryInvitedPerson(t *testing.T) {
	served := map[string]bool{"en": true, "de": true, "de-AT": true}
	for _, tc := range []struct {
		name          string
		own, sent     string
		unowned       bool
		wantNamedIn   string // "" for no call
		wantStored    string // the account's language afterwards
		wantRecorded  bool   // the trail names the language kept
		wantUnchanged bool   // no preference written
	}{
		{name: "no language of its own: the card's is named and kept",
			sent: "de", wantNamedIn: "de", wantStored: "de", wantRecorded: true},
		{name: "a region is kept as sent",
			sent: "de-AT", wantNamedIn: "de-AT", wantStored: "de-AT", wantRecorded: true},
		{name: "its own language wins over the card's",
			own: "en", sent: "de", wantNamedIn: "en", wantStored: "en", wantUnchanged: true},
		{name: "its own language names the seed without a card's",
			own: "en", wantNamedIn: "en", wantStored: "en", wantUnchanged: true},
		{name: "none sent: the workspace's language stays",
			wantStored: "", wantUnchanged: true},
		{name: "a language without a catalogue invents nothing",
			sent: "ja", wantNamedIn: "ja", wantStored: "", wantUnchanged: true},
		{name: "a malformed language stores nothing",
			sent: "de_DE!", wantNamedIn: "de_DE!", wantStored: "", wantUnchanged: true},
		{name: "a member, with no seed of their own, keeps the card's",
			sent: "de", unowned: true, wantNamedIn: "de", wantStored: "de", wantRecorded: true},
		{name: "a guest keeps the card's, region included",
			sent: "de-AT", unowned: true, wantNamedIn: "de-AT", wantStored: "de-AT", wantRecorded: true},
		{name: "a member with a language of their own keeps it",
			own: "en", sent: "de", unowned: true, wantNamedIn: "en", wantStored: "en", wantUnchanged: true},
		{name: "a member on a card no catalogue serves gets none",
			sent: "ja", unowned: true, wantNamedIn: "ja", wantStored: "", wantUnchanged: true},
		{name: "a member who sent none gets none",
			unowned: true, wantStored: "", wantUnchanged: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newStepFixture(now)
			token := redemptionToken(t)
			waiting := waitingRedemption()
			waiting.Account.Locale = tc.own
			waiting.Account.TimeZone = "Europe/Berlin"
			fixture.session.accounts.redemption[token.Secret()] = waiting
			accounts := newAccounts(waiting.Account)
			passwords := fixture.passwords.writer
			seed := &seedLocaliserFake{served: served, unowned: tc.unowned}

			_, err := RedeemInvitation{
				Writer: fixture.session.writer, Passwords: &passwords, Seed: seed, Accounts: accounts,
			}.Execute(t.Context(), RedeemInvitationCommand{
				Token: secret.New(token.Secret()), Password: secret.New("seven blue lanterns over the harbour"),
				Locale: tc.sent,
			})
			if err != nil {
				t.Fatalf("redeeming: %v", err)
			}

			if tc.wantNamedIn == "" {
				if seed.calls != 0 {
					t.Fatalf("the seed was named in %q, want it left alone", seed.locale)
				}
			} else if seed.calls != 1 || seed.locale != tc.wantNamedIn || seed.ownerID != waiting.Account.ID.String() {
				t.Fatalf("the seed was named %d times in %q for %s, want once in %q for the redeemer",
					seed.calls, seed.locale, seed.ownerID, tc.wantNamedIn)
			}

			stored := accounts.byID[waiting.Account.ID]
			if stored.Locale != tc.wantStored {
				t.Errorf("the account's language is %q, want %q", stored.Locale, tc.wantStored)
			}
			if tc.wantUnchanged && len(accounts.preference) != 0 {
				t.Errorf("%d preference writes, want none", len(accounts.preference))
			}
			if !tc.wantUnchanged && (len(accounts.preference) != 1 || stored.TimeZone != "Europe/Berlin") {
				t.Errorf("%d preference writes with the zone %q, want one keeping Europe/Berlin",
					len(accounts.preference), stored.TimeZone)
			}

			var redeemed int
			for _, entry := range fixture.session.audit.entries {
				if entry.Action != InvitationRedeemedAction {
					continue
				}
				redeemed++
				if _, recorded := entry.Changes["locale"]; recorded != tc.wantRecorded {
					t.Errorf("the trail names the language: %v, want %v", recorded, tc.wantRecorded)
				}
			}
			if redeemed != 1 {
				t.Errorf("%d redemption entries, want 1", redeemed)
			}
		})
	}
}
