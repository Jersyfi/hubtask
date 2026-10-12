// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// A password the rule refuses meets the change step at the next sign-in (identity.md §4.2) - also
// when a factor step comes first. Before, the factor's step opened the session and the owed change
// was forgotten: an account with a second factor kept a password the rule had refused, and a start
// password survived a workspace that demands a factor (UC-ID-20 check 3).

// stepWithFactors is the change-step fixture with the second factor's machinery behind it.
func stepWithFactors(at time.Time) *stepFixture {
	fixture := newStepFixture(at)
	fixture.session.writer.Encryptor = newEncryptor()
	fixture.session.writer.Policy = policyFake{}
	fixture.session.writer.People = newAccounts(domain.Account{
		ID: account, TenantID: tenant, Kind: domain.AccountUser,
		Email: "bert@example.org", DisplayName: "Bert", Status: domain.AccountActive,
	})
	fixture.passwords.writer.Session = fixture.session.writer
	return fixture
}

// The enrolment a workspace demands comes first; its confirmation then answers the change step
// rather than the session, and the change completes the sign-in.
func TestAnOwedChangeSurvivesTheDemandedEnrolment(t *testing.T) {
	fixture := stepWithFactors(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{
		MfaRequiredFor: requirementOf(domain.MfaForEveryone), MinLength: intOf(30),
	}

	first := fixture.signsIn(t, "correct horse battery").Challenge
	if first == nil || first.Methods[0] != methodEnroll {
		t.Fatalf("the first step owes %+v, want the enrolment", first)
	}

	anonymous := appshared.Anonymous("en", "UTC")
	if _, err := (EnrollTotp{Writer: fixture.session.writer}).Execute(t.Context(), anonymous,
		EnrollTotpCommand{PendingToken: first.Token}); err != nil {
		t.Fatalf("enrolling: %v", err)
	}
	material := fixture.session.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	confirmed, err := ConfirmTotp{Writer: fixture.session.writer}.Execute(t.Context(), anonymous,
		ConfirmTotpCommand{
			PendingToken: first.Token,
			Code:         domain.TotpCode([]byte(material), domain.TotpStep(now)),
		})
	if err != nil {
		t.Fatalf("confirming: %v", err)
	}
	if confirmed.Pair != nil {
		t.Fatal("the enrolment opened a session with a password the rule refuses")
	}
	if confirmed.Challenge == nil || len(confirmed.Challenge.Methods) != 1 ||
		confirmed.Challenge.Methods[0] != methodPasswordChange || confirmed.Challenge.PasswordRules == nil {
		t.Fatalf("the confirmation owes %+v, want the change step with its rules", confirmed.Challenge)
	}

	pair, err := SetPasswordAndSignIn{Writer: fixture.passwords.writer}.Execute(t.Context(),
		SetPasswordAndSignInCommand{
			PendingToken: confirmed.Challenge.Token,
			Password:     secret.New("seven blue lanterns above the quiet harbour"),
		})
	if err != nil || pair.Session.ID.IsZero() {
		t.Fatalf("the change step did not finish the sign-in: %v", err)
	}
}

// An armed factor comes first too; its code answers the change step rather than the session.
func TestAnOwedChangeSurvivesTheFactorsCode(t *testing.T) {
	fixture := stepWithFactors(now)
	fixture.session.withAccount("bert@example.org", "correct horse battery")
	material := enrolled(t, fixture.session)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(30)}

	first := fixture.signsIn(t, "correct horse battery").Challenge
	if first == nil || first.Methods[0] != methodTotp {
		t.Fatalf("the first step owes %+v, want the code", first)
	}

	later := now.Add(domain.TotpStepSeconds * time.Second)
	completer := fixture.session.writer
	completer.Clock = clock.Fixed(later)
	result, _, err := CompleteSignIn{Writer: completer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: first.Token,
		Code:         domain.TotpCode(material, domain.TotpStep(later)),
	})
	if err != nil {
		t.Fatalf("completing: %v", err)
	}
	if result.Pair != nil {
		t.Fatal("the code opened a session with a password the rule refuses")
	}
	if result.Challenge == nil || result.Challenge.Methods[0] != methodPasswordChange {
		t.Fatalf("the code's step owes %+v, want the change step", result.Challenge)
	}
}

// A password the rule accepts goes through the factor's step to the session, as before.
func TestNoChangeIsOwedForAPasswordTheRuleAccepts(t *testing.T) {
	fixture := stepWithFactors(now)
	fixture.session.withAccount("bert@example.org", "a long enough passphrase")
	material := enrolled(t, fixture.session)

	first := fixture.signsIn(t, "a long enough passphrase").Challenge
	later := now.Add(domain.TotpStepSeconds * time.Second)
	completer := fixture.session.writer
	completer.Clock = clock.Fixed(later)
	result, _, err := CompleteSignIn{Writer: completer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: first.Token,
		Code:         domain.TotpCode(material, domain.TotpStep(later)),
	})
	if err != nil || result.Pair == nil || result.Challenge != nil {
		t.Fatalf("a conforming password did not sign in after its code: %+v %v", result, err)
	}
}
