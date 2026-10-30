// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
)

// KeepSAMLAnswer on the fixture's flows: once, on a live sign-in's flow.
func (s *flowStore) KeepSAMLAnswer(
	_ context.Context, presented domain.Token, answer string, hash []byte, _ time.Time,
) (bool, error) {
	flow, found := s.byState[presented.Secret()]
	if !found || s.spent[presented.Secret()] || flow.SAMLResponse != "" || !flow.SessionID.IsZero() {
		return false, nil
	}
	flow.SAMLResponse, flow.SAMLHandoffHash = answer, hash
	s.byState[presented.Secret()] = flow
	return true, nil
}

// samlDouble records what it was asked to verify.
type samlDouble struct {
	arrivingDouble
	verified provider.Exchange
	config   provider.Config
}

func (d *samlDouble) Exchange(_ context.Context, cfg provider.Config, ex provider.Exchange) (provider.Identity, error) {
	d.verified, d.config = ex, cfg
	return d.identity, d.err
}

func newSAMLFixture(t *testing.T, at time.Time) (*oidcFixture, *samlDouble) {
	t.Helper()
	f := newOidcFixture(t, at)
	double := &samlDouble{arrivingDouble: arrivingDouble{identity: provider.Identity{
		Subject: "00u1ada", Email: "ada@example.org", EmailVerified: true, DisplayName: "Ada",
	}}}
	f.writer.SAML, f.writer.SAMLACSURL = double, "https://hubtask.example/api/v1/auth/saml:acs"
	cert := samlTestCertificate(t, at)
	configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: f.provider.ID, TenantID: tenant, Issuer: "http://www.example.org/exk1", ClientID: "https://hubtask.example/sp",
		Kind: "SAML", SAMLSSOURL: "https://idp.example.org/sso", SAMLCertificate: cert,
		AllowedEmailDomains: []string{"example.org"}, Enabled: true, Now: at,
	})
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	for i := range f.store.rows {
		if f.store.rows[i].ID == configured.ID {
			configured.CreatedAt, configured.Version = f.store.rows[i].CreatedAt, f.store.rows[i].Version
			f.store.rows[i] = configured
		}
	}
	return f, double
}

// UC-ID-30 check 3: the answer is kept once, verified only at the callback with the handoff the
// service minted, and the sign-in ends in a session as any provider's does.
func TestASAMLAnswerIsKeptOnceAndVerifiedAtTheCallback(t *testing.T) {
	at := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	f, double := newSAMLFixture(t, at)
	state := start(t, f)
	acs := ReceiveSAMLAnswer{Session: f.session.writer, Answers: f.flows}

	code, err := acs.Execute(t.Context(), state.Reveal(), "PHNhbWxwOlJlc3BvbnNlLz4=")
	if err != nil {
		t.Fatalf("keeping the answer: %v", err)
	}
	if _, err := acs.Execute(t.Context(), state.Reveal(), "PGV2aWwvPg=="); shared.AsError(err).DetailCode != "auth.oidc_flow_not_found" {
		t.Fatalf("a second answer: %v", err)
	}
	if _, err := acs.Execute(t.Context(), "hbt_osf_nonsense", "x"); shared.AsError(err).DetailCode != "auth.oidc_flow_not_found" {
		t.Fatalf("an unknown state: %v", err)
	}

	result, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: code.Reveal(), State: state, UserAgent: "Firefox",
	})
	if err != nil {
		t.Fatalf("completing: %v", err)
	}
	if pairOf(result).Session.ID.IsZero() {
		t.Fatal("no session was opened")
	}
	if double.verified.Code != "PHNhbWxwOlJlc3BvbnNlLz4=" || double.verified.Nonce == "" ||
		double.config.RedirectURL != "https://hubtask.example/api/v1/auth/saml:acs" ||
		double.config.Issuer != "http://www.example.org/exk1" || double.config.Certificate == "" {
		t.Fatalf("verified %+v with %+v", double.verified, double.config)
	}
}

// A callback without the minted handoff verifies nothing and spends the flow.
func TestASAMLCallbackWithoutTheHandoffIsRefused(t *testing.T) {
	at := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	f, double := newSAMLFixture(t, at)
	state := start(t, f)
	if _, err := (ReceiveSAMLAnswer{Session: f.session.writer, Answers: f.flows}).Execute(t.Context(), state.Reveal(), "eA=="); err != nil {
		t.Fatal(err)
	}
	_, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: domain.SAMLHandoffPrefix + "guessed", State: state,
	})
	if shared.AsError(err).DetailCode != "auth.oidc_failed" || double.verified.Code != "" {
		t.Fatalf("want the one refusal before any verification, got %v", err)
	}
}

func samlTestCertificate(t *testing.T, at time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: at.AddDate(-1, 0, 0), NotAfter: at.AddDate(1, 0, 0)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
