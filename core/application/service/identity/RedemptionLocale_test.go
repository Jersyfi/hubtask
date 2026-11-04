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

// seedLocaliserFake records the language a redemption asked the seed to be named in.
type seedLocaliserFake struct {
	calls   int
	locale  string
	ownerID string
}

func (s *seedLocaliserFake) LocaliseSeed(_ context.Context, owner domain.Account, locale string, _ time.Time) error {
	s.calls++
	s.locale, s.ownerID = locale, owner.ID.String()
	return nil
}

// user-experience.md §4: the language the invitation card was read in reaches the seed, inside the
// redemption; a redemption that sends none leaves the workspace's language alone.
func TestARedemptionNamesTheSeedInTheLanguageTheCardWasReadIn(t *testing.T) {
	for _, tc := range []struct {
		name, locale string
		wantCalls    int
	}{
		{"a language sent", "en", 1},
		{"none sent", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newStepFixture(now)
			token := redemptionToken(t)
			waiting := waitingRedemption()
			fixture.session.accounts.redemption[token.Secret()] = waiting
			passwords := fixture.passwords.writer
			seed := &seedLocaliserFake{}

			_, err := RedeemInvitation{Writer: fixture.session.writer, Passwords: &passwords, Seed: seed}.
				Execute(t.Context(), RedeemInvitationCommand{
					Token: secret.New(token.Secret()), Password: secret.New("seven blue lanterns over the harbour"),
					Locale: tc.locale,
				})
			if err != nil {
				t.Fatalf("redeeming: %v", err)
			}
			if seed.calls != tc.wantCalls {
				t.Fatalf("the seed was named %d times, want %d", seed.calls, tc.wantCalls)
			}
			if tc.wantCalls == 1 && (seed.locale != tc.locale || seed.ownerID != waiting.Account.ID.String()) {
				t.Errorf("named in %q for %s, want %q for the redeemer %s",
					seed.locale, seed.ownerID, tc.locale, waiting.Account.ID)
			}
		})
	}
}
