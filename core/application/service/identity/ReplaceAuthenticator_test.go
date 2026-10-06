// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"encoding/base32"
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// SC-17, UC-ID-03 checks 4-6: the authenticator is replaced without a moment with no factor. The new
// secret waits beside the armed one; the old factor and its codes keep working until a code from the
// new app confirms the swap, and then the old ones stop at once.

// replacing is an armed person with a live session and a step-up in hand.
func replacing(t *testing.T) (*sessionFixture, []byte) {
	t.Helper()
	fixture := stepUpFixture(now)
	material := enrolled(t, fixture)
	// A new secret is new: the fixed entropy would draw the old one again. And several sign-ins in
	// one test are several pending credentials, which need identifiers of their own.
	fixture.writer.Entropy = &countingEntropy{drawn: 100}
	fixture.writer.IDs = &distinctIDs{}
	return fixture, material
}

// proof mints a step-up with the password, the proof every account in these fixtures holds.
func proof(t *testing.T, fixture *sessionFixture) string {
	t.Helper()
	grant, err := StepUp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{Password: secret.New("correct horse battery")})
	if err != nil {
		t.Fatalf("stepping up: %v", err)
	}
	return grant.Token.Reveal()
}

// newMaterial reads the replacement's secret back the way an authenticator does from the text.
func newMaterial(t *testing.T, started MintedReplacement) []byte {
	t.Helper()
	material, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(started.Secret.Reveal())
	if err != nil {
		t.Fatalf("reading the new secret: %v", err)
	}
	return material
}

// sessionCredentialOf is the credential a live session answers, for the session given or the
// credential's own.
func sessionCredentialOf(credential repository.RefreshCredential, session ...domain.Session) repository.SessionCredential {
	held := credential.Session
	if len(session) > 0 {
		held = session[0]
	}
	return repository.SessionCredential{
		TenantStatus: domain.TenantActive, Session: held, Account: credential.Account,
	}
}

func completeWith(t *testing.T, fixture *sessionFixture, at time.Time, material []byte) error {
	t.Helper()
	completer := fixture.writer
	completer.Clock = clock.Fixed(at)
	_, _, err := CompleteSignIn{Writer: completer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: signInChallenge(t, fixture),
		Code:         domain.TotpCode(material, domain.TotpStep(at)),
	})
	return err
}

func TestTheOldFactorSignsInUntilTheConfirmationAndNotAfter(t *testing.T) {
	fixture, old := replacing(t)

	started, err := StartAuthenticatorReplacement{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StartAuthenticatorReplacementCommand{StepUpToken: proof(t, fixture)})
	if err != nil {
		t.Fatalf("beginning the replacement: %v", err)
	}
	fresh := newMaterial(t, started)
	if started.URI.Reveal() == "" || started.ExpiresAt.IsZero() {
		t.Errorf("the replacement answered %+v", started)
	}

	// Before the confirmation the old authenticator signs in, and the new one does not.
	step1 := now.Add(1 * domain.TotpStepSeconds * time.Second)
	if err := completeWith(t, fixture, step1, old); err != nil {
		t.Fatalf("the old factor stopped signing in before the confirmation: %v", err)
	}
	step2 := now.Add(2 * domain.TotpStepSeconds * time.Second)
	if err := completeWith(t, fixture, step2, fresh); err == nil {
		t.Fatal("the new authenticator signed in before it was confirmed")
	}
	// The old recovery codes too: they are the old factor's escape hatch until the swap.
	oldCode := liveRecoveryCode(t, fixture)
	if _, _, err := (CompleteSignIn{Writer: fixture.writer}).Execute(t.Context(), CompleteSignInCommand{
		PendingToken: signInChallenge(t, fixture), RecoveryCode: secret.New(oldCode),
	}); err != nil {
		t.Fatalf("an old recovery code stopped signing in before the confirmation: %v", err)
	}
	// Another old code, still live, kept aside to try after the swap.
	staleCode := liveRecoveryCode(t, fixture)

	// The confirmation: a code from the new app, the swap, ten new codes.
	confirmer := fixture.writer
	at := now.Add(3 * domain.TotpStepSeconds * time.Second)
	confirmer.Clock = clock.Fixed(at)
	codes, err := ConfirmAuthenticatorReplacement{Writer: confirmer}.Execute(t.Context(), signedInActor(),
		ConfirmAuthenticatorReplacementCommand{Code: domain.TotpCode(fresh, domain.TotpStep(at))})
	if err != nil {
		t.Fatalf("confirming the replacement: %v", err)
	}
	if len(codes) != domain.RecoveryCodeCount {
		t.Errorf("%d recovery codes answered, want %d", len(codes), domain.RecoveryCodeCount)
	}
	if !auditRecorded(fixture.audit, MfaReplacedAction) {
		t.Error("the swap is not in the trail")
	}

	// After it, the new authenticator signs in and the old one does not.
	step4 := now.Add(4 * domain.TotpStepSeconds * time.Second)
	if err := completeWith(t, fixture, step4, old); err == nil {
		t.Fatal("the old factor still signs in after the confirmation")
	}
	step5 := now.Add(5 * domain.TotpStepSeconds * time.Second)
	if err := completeWith(t, fixture, step5, fresh); err != nil {
		t.Fatalf("the new authenticator does not sign in after the confirmation: %v", err)
	}
	// And the old codes went with the old factor: the one left from before no longer signs in, and
	// the store holds exactly the ten new ones.
	if _, _, err := (CompleteSignIn{Writer: fixture.writer}).Execute(t.Context(), CompleteSignInCommand{
		PendingToken: signInChallenge(t, fixture), RecoveryCode: secret.New(staleCode),
	}); err == nil {
		t.Fatal("an old recovery code still signs in after the confirmation")
	}
	left, _ := fixture.writer.Recovery.Remaining(t.Context(), account)
	if left != domain.RecoveryCodeCount {
		t.Errorf("%d codes left, want the ten new ones", left)
	}
	for _, code := range codes {
		if !fixture.writer.Recovery.(*recoveryStore).codes[domain.NormalizeRecoveryCode(code.Reveal())] {
			t.Errorf("an answered code is not live")
		}
	}
}

// A workspace that requires a factor of everyone is never without one: the enrolment stays armed
// from the start of the replacement to its end, and turning it off stays refused throughout.
func TestTheRuleRequiringEveryoneIsNeverBrokenDuringAReplacement(t *testing.T) {
	fixture := armedUnder(t, domain.MfaForEveryone, domain.RoleMember)
	session := fixture.session
	session.writer.StepUps = newStepUps()
	session.writer.Entropy = &countingEntropy{drawn: 100}
	credential, _ := refreshCredential(now)
	session.sessions.sessions[sessionRowID] = sessionCredentialOf(credential)

	armed := func(when string) {
		t.Helper()
		found, err := fixture.enroll.Find(t.Context(), account)
		if err != nil || found.ConfirmedAt.IsZero() {
			t.Fatalf("%s the factor is not armed: (%+v, %v)", when, found, err)
		}
		if err := turnOff(t, fixture); !errors.Is(err, shared.ErrForbidden) {
			t.Fatalf("%s turning the factor off answered %v", when, err)
		}
	}

	armed("before the replacement,")
	started, err := StartAuthenticatorReplacement{Writer: session.writer}.Execute(t.Context(), signedInActor(),
		StartAuthenticatorReplacementCommand{StepUpToken: proof(t, session)})
	if err != nil {
		t.Fatalf("beginning the replacement under the rule: %v", err)
	}
	armed("while the replacement waits,")

	at := now.Add(domain.TotpStepSeconds * time.Second)
	confirmer := session.writer
	confirmer.Clock = clock.Fixed(at)
	fresh := newMaterial(t, started)
	if _, err := (ConfirmAuthenticatorReplacement{Writer: confirmer}).Execute(t.Context(), signedInActor(),
		ConfirmAuthenticatorReplacementCommand{Code: domain.TotpCode(fresh, domain.TotpStep(at))}); err != nil {
		t.Fatalf("confirming under the rule: %v", err)
	}
	armed("after the swap,")
}

// The session alone does not begin a replacement - it is the same power as removing the factor.
func TestAReplacementBeginsOnlyBehindTheStepUp(t *testing.T) {
	fixture, _ := replacing(t)
	_, err := StartAuthenticatorReplacement{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StartAuthenticatorReplacementCommand{})
	if err == nil || !strings.Contains(err.Error(), stepup.CodeRequired) {
		t.Fatalf("a replacement began without a proof: %v", err)
	}
}

// Nothing armed is nothing to replace: setting one up is enrolment's.
func TestThereIsNothingToReplaceWithoutAFactor(t *testing.T) {
	fixture := stepUpFixture(now)
	_, err := StartAuthenticatorReplacement{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StartAuthenticatorReplacementCommand{StepUpToken: proof(t, fixture)})
	if err == nil || !strings.Contains(err.Error(), "auth.mfa_not_enrolled") {
		t.Fatalf("a replacement began without an armed factor: %v", err)
	}
}

// Only the session that began it confirms it, only inside its window, and a wrong code changes
// nothing.
func TestOnlyItsSessionConfirmsAReplacementInTime(t *testing.T) {
	fixture, old := replacing(t)
	started, err := StartAuthenticatorReplacement{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		StartAuthenticatorReplacementCommand{StepUpToken: proof(t, fixture)})
	if err != nil {
		t.Fatalf("beginning: %v", err)
	}
	fresh := newMaterial(t, started)
	at := now.Add(domain.TotpStepSeconds * time.Second)
	confirmer := fixture.writer
	confirmer.Clock = clock.Fixed(at)
	code := domain.TotpCode(fresh, domain.TotpStep(at))

	credential, _ := refreshCredential(now)
	otherSession := credential.Session
	otherSession.ID = refreshRowID
	fixture.sessions.sessions[refreshRowID] = sessionCredentialOf(credential, otherSession)
	thief := signedInActor()
	thief.TokenID = refreshRowID
	if _, err := (ConfirmAuthenticatorReplacement{Writer: confirmer}).Execute(t.Context(), thief,
		ConfirmAuthenticatorReplacementCommand{Code: code}); err == nil ||
		!strings.Contains(err.Error(), "auth.mfa_replacement_unknown") {
		t.Errorf("another session confirmed the replacement: %v", err)
	}

	if _, err := (ConfirmAuthenticatorReplacement{Writer: confirmer}).Execute(t.Context(), signedInActor(),
		ConfirmAuthenticatorReplacementCommand{Code: domain.TotpCode(old, domain.TotpStep(at))}); err == nil ||
		!strings.Contains(err.Error(), "auth.mfa_code_invalid") {
		t.Errorf("the old authenticator's code confirmed the new one: %v", err)
	}

	late := fixture.writer
	lateAt := now.Add(domain.MfaReplacementLifetime + time.Minute)
	late.Clock = clock.Fixed(lateAt)
	if _, err := (ConfirmAuthenticatorReplacement{Writer: late}).Execute(t.Context(), signedInActor(),
		ConfirmAuthenticatorReplacementCommand{Code: domain.TotpCode(fresh, domain.TotpStep(lateAt))}); err == nil ||
		!strings.Contains(err.Error(), "auth.mfa_replacement_unknown") {
		t.Errorf("a lapsed replacement was confirmed: %v", err)
	}

	found, _ := fixture.writer.Enrollments.Find(t.Context(), account)
	if found.Replacement == nil {
		t.Error("a refused confirmation dropped the waiting replacement")
	}
}
