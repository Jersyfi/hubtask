// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The terms of use at sign-in (UC-ID-19): the TERMS step after every proof, and its completion.

const (
	hostTerms = "https://host.example/terms"
	acmeTerms = "https://acme.example/terms"
)

type termsReaderFake struct{ terms domain.TermsInForce }

func (r *termsReaderFake) TermsInForce(context.Context, shared.ID) (domain.TermsInForce, error) {
	return r.terms, nil
}

type termsStore struct {
	agreements map[shared.ID]domain.TermsAgreement
	recorded   int
}

func newTermsStore() *termsStore {
	return &termsStore{agreements: map[shared.ID]domain.TermsAgreement{}}
}

func (s *termsStore) TermsAgreementOf(_ context.Context, accountID shared.ID) (domain.TermsAgreement, error) {
	return s.agreements[accountID], nil
}

func (s *termsStore) RecordTermsAgreement(
	_ context.Context, accountID shared.ID, agreement domain.TermsAgreement,
) (bool, error) {
	s.agreements[accountID] = agreement
	s.recorded++
	return true, nil
}

// termsFixture is the second factor's fixture with terms in force and nobody agreed.
func termsFixture(at time.Time, terms domain.TermsInForce) (*sessionFixture, *termsReaderFake, *termsStore) {
	fixture := mfaFixture(at)
	reader, store := &termsReaderFake{terms: terms}, newTermsStore()
	fixture.writer.TermsIn = reader
	fixture.writer.Terms = store
	return fixture, reader, store
}

func installationTerms(version string) domain.TermsInForce {
	return domain.TermsInForce{URL: hostTerms, Version: version, SetBy: domain.TermsByInstallation}
}

func termsRegistry(t *testing.T, writer SessionWriter) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil,
		SignIn{Writer: writer}.Descriptor(),
		AgreeToTerms{Writer: writer}.Descriptor(),
		RefreshSession{Writer: writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return registry
}

// signInThroughTheRegistry signs Bert in by password and answers the output.
func signInThroughTheRegistry(t *testing.T, registry *usecase.Registry) usecase.Output {
	t.Helper()
	out, err := registry.Invoke(t.Context(), SignInName, appshared.Anonymous("en", "UTC"), usecase.Input{
		"email": "bert@example.org", "password": "correct horse battery",
		"user_agent": "browser/1", "remote_addr": "203.0.113.7:51234",
	})
	if err != nil {
		t.Fatalf("signing in: %v", err)
	}
	return out
}

// Check 2: an account that never agreed - or agreed to other terms - meets the TERMS step and
// no session opens; agreeing opens the session the password proved, records the agreement on the
// account and in the trail (check 4) with the version and the link.
func TestAPasswordSignInEndsInTheTermsStepUntilAgreed(t *testing.T) {
	fixture, _, store := termsFixture(now, installationTerms("3"))
	fixture.withAccount("bert@example.org", "correct horse battery")
	registry := termsRegistry(t, fixture.writer)

	out := signInThroughTheRegistry(t, registry)
	methods, _ := out["methods"].([]any)
	if len(methods) != 1 || methods[0] != methodTerms {
		t.Fatalf("the sign-in answered %v, want the TERMS step alone", out)
	}
	terms, _ := out["terms"].(usecase.Output)
	if terms["url"] != hostTerms || terms["version"] != "3" || terms["set_by"] != "INSTALLATION" {
		t.Errorf("the step names the terms as %v", terms)
	}
	if len(fixture.sessions.inserted) != 0 {
		t.Fatalf("%d sessions opened before the agreement", len(fixture.sessions.inserted))
	}
	if len(fixture.attempts.cleared) == 0 {
		t.Error("the proven password left the ledger standing")
	}

	pair, err := registry.Invoke(t.Context(), AgreeToTermsName, appshared.Anonymous("en", "UTC"), usecase.Input{
		"pending_token": out["pending_token"], "terms_url": hostTerms, "terms_version": "3",
	})
	if err != nil {
		t.Fatalf("agreeing: %v", err)
	}
	if pair.String("access_token") == "" || len(fixture.sessions.inserted) != 1 {
		t.Fatalf("agreeing answered %v with %d sessions", pair, len(fixture.sessions.inserted))
	}
	session := fixture.sessions.inserted[0]
	if session.SignedInWith != domain.SignedInWithPassword ||
		session.UserAgent != "browser/1" {
		t.Errorf("the session is %+v, want the password sign-in's", session)
	}
	agreement := store.agreements[account]
	if agreement.URL != hostTerms || agreement.Version != "3" || !agreement.AgreedAt.Equal(now) {
		t.Errorf("the account keeps %+v", agreement)
	}
	var agreed, signedIn bool
	for _, entry := range fixture.audit.entries {
		switch entry.Action {
		case TermsAgreedAction:
			agreed = entry.Changes["terms_version"] != nil && entry.TargetID == account
		case SignedInAction:
			signedIn = true
		}
	}
	if !agreed || !signedIn {
		t.Errorf("the trail reads %v, want the agreement with its version and the sign-in", fixture.audit.entries)
	}

	// Agreed: the next sign-in is a plain one.
	again := signInThroughTheRegistry(t, registry)
	if again.String("access_token") == "" {
		t.Errorf("an account that agreed met a step again: %v", again)
	}
}

// Check 2 again: a new version is put once more; the same version is not.
func TestANewVersionAsksAgainAndTheSameDoesNot(t *testing.T) {
	fixture, reader, store := termsFixture(now, installationTerms("4"))
	fixture.withAccount("bert@example.org", "correct horse battery")
	store.agreements[account] = domain.TermsAgreement{URL: hostTerms, Version: "3", AgreedAt: now.Add(-time.Hour)}
	registry := termsRegistry(t, fixture.writer)

	if out := signInThroughTheRegistry(t, registry); out["terms"] == nil {
		t.Errorf("version 4 was not put to somebody who agreed to version 3: %v", out)
	}
	reader.terms = installationTerms("3")
	if out := signInThroughTheRegistry(t, registry); out.String("access_token") == "" {
		t.Errorf("version 3 was put again to somebody who agreed to it: %v", out)
	}
}

// Check 5: where no terms are set, no step anywhere.
func TestNoTermsNoStep(t *testing.T) {
	fixture, _, _ := termsFixture(now, domain.TermsInForce{})
	fixture.withAccount("bert@example.org", "correct horse battery")

	if out := signInThroughTheRegistry(t, termsRegistry(t, fixture.writer)); out.String("access_token") == "" {
		t.Errorf("a workspace without terms asked for them: %v", out)
	}
}

// Terms that changed while the step was open are refused without spending the credential, so the
// person agrees to the new ones with the same step; the trail records nothing for the refusal.
func TestTermsThatChangedMeanwhileAreRefusedAndTheStepStaysOpen(t *testing.T) {
	fixture, reader, store := termsFixture(now, installationTerms("3"))
	fixture.withAccount("bert@example.org", "correct horse battery")
	registry := termsRegistry(t, fixture.writer)
	out := signInThroughTheRegistry(t, registry)

	reader.terms = installationTerms("4")
	_, err := registry.Invoke(t.Context(), AgreeToTermsName, appshared.Anonymous("en", "UTC"), usecase.Input{
		"pending_token": out["pending_token"], "terms_url": hostTerms, "terms_version": "3",
	})
	var problem *shared.Error
	if !errors.Is(err, shared.ErrConflict) || !errors.As(err, &problem) ||
		problem.DetailCode != "auth.terms_changed" || problem.Params["version"] != "4" {
		t.Fatalf("an agreement to old terms answered %v", err)
	}
	if store.recorded != 0 || len(fixture.sessions.inserted) != 0 {
		t.Fatal("an agreement to old terms was recorded or opened a session")
	}

	if _, err := registry.Invoke(t.Context(), AgreeToTermsName, appshared.Anonymous("en", "UTC"), usecase.Input{
		"pending_token": out["pending_token"], "terms_url": hostTerms, "terms_version": "4",
	}); err != nil {
		t.Fatalf("agreeing to the terms now in force with the same step: %v", err)
	}
	if store.agreements[account].Version != "4" {
		t.Errorf("the account keeps %+v", store.agreements[account])
	}
}

// The step's credential completes this step and nothing else, once; and nothing else completes it.
func TestTheTermsCredentialIsOneUseAndOnlyForTheTerms(t *testing.T) {
	fixture, _, _ := termsFixture(now, installationTerms(""))
	fixture.withAccount("bert@example.org", "correct horse battery")
	registry := termsRegistry(t, fixture.writer)
	anonymous := appshared.Anonymous("en", "UTC")
	out := signInThroughTheRegistry(t, registry)

	// The second factor's step cannot complete a TERMS credential.
	if _, _, err := (CompleteSignIn{Writer: fixture.writer}).Execute(t.Context(), CompleteSignInCommand{
		PendingToken: secret.New(out.String("pending_token")), Code: "000000",
	}); err == nil {
		t.Error("a code completed the terms step")
	}

	agree := usecase.Input{"pending_token": out["pending_token"], "terms_url": hostTerms}
	if _, err := registry.Invoke(t.Context(), AgreeToTermsName, anonymous, agree); err != nil {
		t.Fatalf("agreeing to terms without a version: %v", err)
	}
	if _, err := registry.Invoke(t.Context(), AgreeToTermsName, anonymous, agree); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("a spent step answered %v, want the steps' one refusal", err)
	}
	if _, err := registry.Invoke(t.Context(), AgreeToTermsName, anonymous, usecase.Input{
		"pending_token": "hbt_mfa_not-a-token", "terms_url": hostTerms,
	}); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("an unknown step answered %v, want the steps' one refusal", err)
	}
	if _, err := registry.Invoke(t.Context(), AgreeToTermsName, anonymous, usecase.Input{
		"pending_token": out["pending_token"],
	}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an agreement naming no terms answered %v", err)
	}
}

// Terms removed while the step was open need no agreement: the session opens, nothing is recorded.
func TestTermsRemovedMeanwhileOpenTheSessionWithoutAnAgreement(t *testing.T) {
	fixture, reader, store := termsFixture(now, installationTerms("3"))
	fixture.withAccount("bert@example.org", "correct horse battery")
	registry := termsRegistry(t, fixture.writer)
	out := signInThroughTheRegistry(t, registry)

	reader.terms = domain.TermsInForce{}
	if _, err := registry.Invoke(t.Context(), AgreeToTermsName, appshared.Anonymous("en", "UTC"), usecase.Input{
		"pending_token": out["pending_token"], "terms_url": hostTerms, "terms_version": "3",
	}); err != nil {
		t.Fatalf("completing: %v", err)
	}
	if store.recorded != 0 || len(fixture.sessions.inserted) != 1 {
		t.Errorf("recorded %d agreements and opened %d sessions", store.recorded, len(fixture.sessions.inserted))
	}
}

// No person, no agreement: a service account never meets the step; and a refresh is no sign-in.
func TestServiceAccountsAndRefreshesNeverMeetTheTerms(t *testing.T) {
	fixture, _, _ := termsFixture(now, installationTerms("3"))
	fixture.withAccount("robot@example.org", "correct horse battery")
	robot := fixture.accounts.byEmail["robot@example.org"]
	robot.Account.Kind = domain.AccountServiceAccount
	fixture.accounts.byEmail["robot@example.org"] = robot

	result, err := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "robot@example.org", Password: secret.New("correct horse battery"),
	})
	if err != nil || result.Pair == nil {
		t.Fatalf("a service account answered (%+v, %v), want the pair", result, err)
	}

	// A person's session refreshed with the terms still unagreed: a rotation, not a step.
	credential, presented := refreshCredential(now)
	fixture.refresh.byToken[presented.Secret()] = credential
	refreshed, err := RefreshSession{Writer: fixture.writer}.Execute(t.Context(), RefreshSessionCommand{
		RefreshToken: secret.New(presented.Secret()),
	})
	if err != nil || refreshed.AccessToken.IsEmpty() {
		t.Errorf("a refresh answered (%+v, %v), want a rotation", refreshed, err)
	}
}
