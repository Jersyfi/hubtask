// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Every way a sign-in ends reaches the TERMS step (UC-ID-19 checks 2 and 6): one test per site that
// opens a session, each with terms in force and nobody agreed, asserting that the site answers the
// step and opens no session - and that agreeing opens the session the site would have opened, with
// its own method and provider.

// withTermsOn hands the same terms and agreements to a writer.
func withTermsOn(writer *SessionWriter, reader TermsReader, store *termsStore) {
	writer.TermsIn, writer.Terms = reader, store
}

// mustBeTermsStep fails unless a site answered the TERMS step alone, and answers its token.
func mustBeTermsStep(t *testing.T, site string, result SignInResult, sessions int) secret.Secret {
	t.Helper()
	if result.Pair != nil {
		t.Fatalf("%s opened a session before the terms were agreed", site)
	}
	challenge := result.Challenge
	if challenge == nil || len(challenge.Methods) != 1 || challenge.Methods[0] != methodTerms ||
		challenge.Terms == nil || challenge.Terms.URL != hostTerms {
		t.Fatalf("%s answered %+v, want the TERMS step alone", site, challenge)
	}
	if sessions != 0 {
		t.Fatalf("%s wrote %d sessions before the terms were agreed", site, sessions)
	}
	return challenge.Token
}

// agreeOpens agrees with a writer and answers the session it opened.
func agreeOpens(t *testing.T, site string, writer SessionWriter, token secret.Secret) domain.Session {
	t.Helper()
	pair, err := AgreeToTerms{Writer: writer}.Execute(t.Context(), AgreeToTermsCommand{
		PendingToken: token, TermsURL: hostTerms, TermsVersion: "3",
	})
	if err != nil {
		t.Fatalf("%s: agreeing: %v", site, err)
	}
	return pair.Session
}

func TestTheCodeAndTheRecoveryCodeStepsEndInTheTerms(t *testing.T) {
	for name, recovery := range map[string]bool{"the code": false, "a recovery code": true} {
		t.Run(name, func(t *testing.T) {
			fixture := stepWithFactors(now)
			fixture.session.withAccount("bert@example.org", "a long enough passphrase")
			material := enrolled(t, fixture.session)
			first := fixture.signsIn(t, "a long enough passphrase").Challenge
			if first == nil || first.Methods[0] != methodTotp {
				t.Fatalf("the sign-in owes %+v, want the code", first)
			}

			// Terms appear only now, so the step before them is the factor's own.
			reader, store := &termsReaderFake{terms: installationTerms("3")}, newTermsStore()
			withTermsOn(&fixture.session.writer, reader, store)
			later := now.Add(domain.TotpStepSeconds * time.Second)
			completer := fixture.session.writer
			completer.Clock = clock.Fixed(later)
			command := CompleteSignInCommand{
				PendingToken: first.Token, Code: domain.TotpCode(material, domain.TotpStep(later)),
			}
			method := domain.SignedInWithPasswordTotp
			if recovery {
				fixture.session.writer.Recovery.(*recoveryStore).codes[domain.NormalizeRecoveryCode("ABCD-EFGH-JKMN-PQRS")] = true
				command = CompleteSignInCommand{
					PendingToken: first.Token, RecoveryCode: secret.New("ABCD-EFGH-JKMN-PQRS"),
				}
				method = domain.SignedInWithPasswordRecovery
			}
			result, _, err := CompleteSignIn{Writer: completer}.Execute(t.Context(), command)
			if err != nil {
				t.Fatalf("completing: %v", err)
			}
			token := mustBeTermsStep(t, name, result, len(fixture.session.sessions.inserted))
			if session := agreeOpens(t, name, completer, token); session.SignedInWith != method {
				t.Errorf("the session was opened by %q, want %q", session.SignedInWith, method)
			}
		})
	}
}

func TestTheDemandedEnrolmentEndsInTheTerms(t *testing.T) {
	fixture := stepWithFactors(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{
		MfaRequiredFor: requirementOf(domain.MfaForEveryone),
	}
	reader, store := &termsReaderFake{terms: installationTerms("3")}, newTermsStore()
	withTermsOn(&fixture.session.writer, reader, store)

	first := fixture.signsIn(t, "a long enough passphrase").Challenge
	if first == nil || first.Methods[0] != methodEnroll {
		t.Fatalf("the sign-in owes %+v, want the enrolment first", first)
	}
	anonymous := appshared.Anonymous("en", "UTC")
	if _, err := (EnrollTotp{Writer: fixture.session.writer}).Execute(t.Context(), anonymous,
		EnrollTotpCommand{PendingToken: first.Token}); err != nil {
		t.Fatalf("enrolling: %v", err)
	}
	material := fixture.session.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	confirmed, err := ConfirmTotp{Writer: fixture.session.writer}.Execute(t.Context(), anonymous,
		ConfirmTotpCommand{
			PendingToken: first.Token, Code: domain.TotpCode([]byte(material), domain.TotpStep(now)),
		})
	if err != nil {
		t.Fatalf("confirming: %v", err)
	}
	token := mustBeTermsStep(t, "the enrolment", SignInResult(confirmed),
		len(fixture.session.sessions.inserted))
	if session := agreeOpens(t, "the enrolment", fixture.session.writer, token); session.SignedInWith !=
		domain.SignedInWithPasswordTotp {
		t.Errorf("the session was opened by %q", session.SignedInWith)
	}
}

// The change step is the last proof, so the terms come after it - and a managed account, which has
// no invitation card, agrees there at its first sign-in (D7).
func TestTheChangeStepEndsInTheTermsAndAManagedAccountAgreesThere(t *testing.T) {
	for name, managed := range map[string]bool{"a password the rule refused": false, "a start password": true} {
		t.Run(name, func(t *testing.T) {
			fixture := newStepFixture(now)
			reader, store := &termsReaderFake{terms: installationTerms("3")}, newTermsStore()
			withTermsOn(&fixture.session.writer, reader, store)
			fixture.passwords.writer.Session = fixture.session.writer

			var first *SignInChallenge
			if managed {
				fixture.session.writer.Text = text.Composing{}
				fixture.passwords.writer.Session = fixture.session.writer
				fixture.withManaged(now.Add(-time.Hour))
				out, err := signInWith(t, signInRegistry(t, fixture.session.writer), "lena", startPassword)
				if err != nil {
					t.Fatalf("signing in with the name: %v", err)
				}
				first = &SignInChallenge{Token: secret.New(out.String("pending_token"))}
				if methods, _ := out["methods"].([]any); len(methods) != 1 || methods[0] != methodPasswordChange {
					t.Fatalf("the start password answered %v, want the change step first", out)
				}
			} else {
				fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(30)}
				first = fixture.signsIn(t, "correct horse battery").Challenge
				if first == nil || first.Methods[0] != methodPasswordChange {
					t.Fatalf("the sign-in owes %+v, want the change step first", first)
				}
			}

			result, err := SetPasswordAndSignIn{Writer: fixture.passwords.writer}.Execute(t.Context(),
				SetPasswordAndSignInCommand{
					PendingToken: first.Token,
					Password:     secret.New("seven blue lanterns above the quiet harbour"),
				})
			if err != nil {
				t.Fatalf("changing: %v", err)
			}
			token := mustBeTermsStep(t, name, result, len(fixture.session.sessions.inserted))
			if session := agreeOpens(t, name, fixture.session.writer, token); session.SignedInWith !=
				domain.SignedInWithPassword {
				t.Errorf("the session was opened by %q", session.SignedInWith)
			}
		})
	}
}

func TestAResetEndsInTheTermsAndNamesTheAccount(t *testing.T) {
	fixture := newResetFixture(now)
	reader, store := &termsReaderFake{terms: installationTerms("3")}, newTermsStore()
	withTermsOn(&fixture.writer.Session, reader, store)
	token := fixture.mintedFor(t, now, domain.PendingReset)

	result, err := ResetPassword{Writer: fixture.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("seven blue lanterns"),
	})
	if err != nil {
		t.Fatalf("the reset was refused: %v", err)
	}
	step := mustBeTermsStep(t, "the reset", result, len(fixture.sessions.inserted))
	if result.Challenge.Email != "bert@example.org" {
		t.Errorf("the step names %q, want the account's address", result.Challenge.Email)
	}
	if session := agreeOpens(t, "the reset", fixture.writer.Session, step); session.SignedInWith !=
		domain.SignedInWithReset {
		t.Errorf("the session was opened by %q", session.SignedInWith)
	}
}

// Check 6: a provider's return - an account made on arrival, and one found by its subject - ends in
// the step, and agreeing opens the provider's session. The person typed no address, so the step
// names the account, as the reset's does.
func TestAProviderReturnEndsInTheTerms(t *testing.T) {
	f := newOidcFixture(t, now)
	reader, store := &termsReaderFake{terms: installationTerms("3")}, newTermsStore()
	withTermsOn(&f.writer.Session, reader, store)
	ids := &distinctIDs{}
	f.session.writer.IDs, f.writer.Session.IDs = ids, ids

	for _, arrival := range []string{"provisioned on arrival", "found by its subject"} {
		result, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(),
			CompleteOidcSignInCommand{Code: arrival, State: start(t, f)})
		if err != nil {
			t.Fatalf("%s: %v", arrival, err)
		}
		token := mustBeTermsStep(t, arrival, result, len(f.session.sessions.inserted))
		if result.Challenge.Email != "ada@example.org" {
			t.Errorf("%s: the step names %q, want the account's address", arrival, result.Challenge.Email)
		}
		// The newcomer is made by the agreement, so the provider's half takes part in it.
		pair, err := AgreeToTerms{Writer: f.writer.Session, Arrivals: f.writer}.Execute(t.Context(),
			AgreeToTermsCommand{PendingToken: token, TermsURL: hostTerms, TermsVersion: "3"})
		if err != nil {
			t.Fatalf("%s: agreeing: %v", arrival, err)
		}
		session := pair.Session
		if session.SignedInWith != domain.SignedInWithOidc || session.SignedInVia != f.provider.ID {
			t.Errorf("%s: the session was opened by %q via %q, want the provider's", arrival,
				session.SignedInWith, session.SignedInVia)
		}
		// The second pass is the same subject, now an existing account: forget the agreement so
		// that it has to meet the step as well.
		store.agreements = map[shared.ID]domain.TermsAgreement{}
		f.session.sessions.inserted = nil
	}
}

func TestTheLinkStepAndTheConnectLinkEndInTheTerms(t *testing.T) {
	t.Run("the link step", func(t *testing.T) {
		f := linkFixture(t, false)
		reader, store := &termsReaderFake{terms: installationTerms("3")}, newTermsStore()
		withTermsOn(&f.writer.Session, reader, store)
		arrived, err := arrive(t, f)
		if err != nil || arrived.Challenge == nil || arrived.Challenge.Methods[0] != methodLink {
			t.Fatalf("arriving: (%+v, %v), want the LINK step first", arrived, err)
		}
		result, err := CompleteLink{Writer: f.writer}.Execute(t.Context(), CompleteLinkCommand{
			PendingToken: arrived.Challenge.Token, Password: secret.New("correct horse battery"),
		})
		if err != nil {
			t.Fatalf("linking: %v", err)
		}
		token := mustBeTermsStep(t, "the link step", result, len(f.session.sessions.inserted))
		if len(f.external.links) != 1 {
			t.Errorf("the provider was not connected by the account's proof: %v", f.external.links)
		}
		if session := agreeOpens(t, "the link step", f.writer.Session, token); session.SignedInWith !=
			domain.SignedInWithOidc {
			t.Errorf("the session was opened by %q", session.SignedInWith)
		}
	})

	t.Run("the connect link", func(t *testing.T) {
		f := connectFixture(t)
		reader, store := &termsReaderFake{terms: installationTerms("3")}, newTermsStore()
		withTermsOn(&f.writer.Session, reader, store)
		link, _ := connectLinkFor(t, f, account, connectAt.Add(domain.ResetLifetime), 1)
		result, err := completeConnect(t, f, startConnect(t, f, link))
		if err != nil {
			t.Fatalf("connecting: %v", err)
		}
		token := mustBeTermsStep(t, "the connect link", result, len(f.session.sessions.inserted))
		if result.Challenge.Email != "bert@example.org" {
			t.Errorf("the step names %q, want the account's address", result.Challenge.Email)
		}
		if session := agreeOpens(t, "the connect link", f.writer.Session, token); session.SignedInWith !=
			domain.SignedInWithOidc {
			t.Errorf("the session was opened by %q", session.SignedInWith)
		}
	})
}

// An elevation raises a session already open; it is no sign-in and meets no terms (D9).
func TestAnElevationMeetsNoTerms(t *testing.T) {
	fixture, register := newElevationFixture(t)
	withTermsOn(&fixture.writer, &termsReaderFake{terms: installationTerms("3")}, newTermsStore())

	if _, err := (ElevateSession{
		Writer: fixture.writer, Operators: register,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &idSequence{},
	}).Execute(t.Context(), recoveryActor(), fixture.proof); err != nil {
		t.Fatalf("the elevation was refused with terms unagreed: %v", err)
	}
}

// D8: the setup's owner is signed in by the setup, which asks for no terms; the owner meets the step
// at the next sign-in like anybody once terms exist - the password sign-in's own test.
func TestTheSetupSignsTheOwnerInWithoutTheTerms(t *testing.T) {
	f := newSetupFixture(env.TenancySingle)
	store := newTermsStore()
	withTermsOn(&f.session.writer, &termsReaderFake{terms: installationTerms("3")}, store)
	f.handler.Writer = f.session.writer

	out, err := f.invoke(t, setupInput(nil))
	if err != nil || out.String("access_token") == "" {
		t.Fatalf("the setup answered (%v, %v), want the pair", out, err)
	}
	if store.recorded != 0 {
		t.Error("the setup recorded an agreement nobody gave")
	}
}
