// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// UC-ID-08 check 4: a session a provider opened answers to the workspace's session rules - the
// maximum age and the idle time - like every other session. Opened with no bounds at all, it would
// let a workspace that ends idle sessions after thirty minutes keep a provider's open for a month.

// boundedRule is a workspace rule with both session bounds set, as the rule reader answers it.
func boundedRule(at time.Time) PasswordWriter {
	passwords := newPasswordFixture(at)
	passwords.instance.level.Policy.Patch = domain.PolicyPatch{
		SessionMaxDays: intOf(7), SessionIdleMinutes: intOf(30),
	}
	return passwords.writer
}

// assertBounded checks the two bounds the row was opened with.
func assertBounded(t *testing.T, opened domain.Session, at time.Time) {
	t.Helper()
	if opened.IdleMinutes != 30 {
		t.Errorf("the idle bound is %d, want the workspace's 30", opened.IdleMinutes)
	}
	if want := at.Add(7 * 24 * time.Hour); !opened.HardExpiresAt.Equal(want) {
		t.Errorf("the hard expiry is %v, want %v", opened.HardExpiresAt, want)
	}
}

func TestAProviderSessionPastItsIdleTimeIsRefused(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)
	f.writer.Session.Rule = boundedRule(at)

	result, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "the-code", State: start(t, f),
	})
	if err != nil || result.Pair == nil {
		t.Fatalf("the arrival answered (%+v, %v), want a session", result, err)
	}
	opened := result.Pair.Session
	assertBounded(t, opened, at)
	if opened.SignedInWith != domain.SignedInWithOidc {
		t.Errorf("the session records %q", opened.SignedInWith)
	}

	// The refresh the client makes after walking away, through the use case that refuses it. The
	// row as the database holds it after its first use: used once, at the moment it opened.
	used := opened
	used.LastSeenAt = at
	store := f.session.refresh
	store.byToken[store.presented[len(store.presented)-1].Secret()] = repository.RefreshCredential{
		TenantStatus: domain.TenantActive,
		Token:        store.inserted[len(store.inserted)-1],
		Session:      used,
		Account:      f.accounts.byEmail["ada@example.org"],
	}
	later := f.writer.Session
	later.Clock = clock.Fixed(at.Add(31 * time.Minute))
	_, err = RefreshSession{Writer: later}.Execute(t.Context(), RefreshSessionCommand{
		RefreshToken: result.Pair.RefreshToken,
	})
	if err == nil || !strings.Contains(err.Error(), "auth.session_idle") {
		t.Errorf("thirty-one idle minutes later the refresh answered %v, want auth.session_idle", err)
	}
}

// The LINK step ends in a provider session too, and it is bounded the same way.
func TestAProviderSessionOpenedAfterTheLinkStepIsBounded(t *testing.T) {
	f := linkFixture(t, false)
	f.writer.Session.Rule = boundedRule(now)

	result, err := arrive(t, f)
	if err != nil || result.Challenge == nil {
		t.Fatalf("arriving: (%+v, %v)", result, err)
	}
	done, err := CompleteLink{Writer: f.writer}.Execute(t.Context(), CompleteLinkCommand{
		PendingToken: result.Challenge.Token, Password: secret.New("correct horse battery"),
	})
	if err != nil || done.Pair == nil {
		t.Fatalf("the LINK step answered (%+v, %v), want a session", done, err)
	}
	assertBounded(t, done.Pair.Session, now)
}

// The session a forced setup opens during sign-in is a password sign-in with a code, and it answers
// to the same rule the ordinary second step does.
func TestTheSessionAForcedSetupOpensIsBounded(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{
		MfaRequiredFor: requirementOf(domain.MfaForEveryone),
		SessionMaxDays: intOf(7), SessionIdleMinutes: intOf(30),
	}
	fixture.session.writer.Encryptor = newEncryptor()
	challenge := fixture.signsIn(t, "a long enough passphrase").Challenge
	if challenge == nil || challenge.Methods[0] != methodEnroll {
		t.Fatalf("no setup was demanded: %+v", challenge)
	}

	anonymous := appshared.Anonymous("en", "UTC")
	if _, err := (EnrollTotp{Writer: fixture.session.writer}).Execute(t.Context(), anonymous,
		EnrollTotpCommand{PendingToken: challenge.Token}); err != nil {
		t.Fatalf("setting up: %v", err)
	}
	material := fixture.session.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	confirmed, err := ConfirmTotp{Writer: fixture.session.writer}.Execute(t.Context(), anonymous,
		ConfirmTotpCommand{
			PendingToken: challenge.Token,
			Code:         domain.TotpCode([]byte(material), domain.TotpStep(now)),
		})
	if err != nil || confirmed.Pair == nil {
		t.Fatalf("confirming answered (%+v, %v), want a session", confirmed, err)
	}
	assertBounded(t, confirmed.Pair.Session, now)
}
