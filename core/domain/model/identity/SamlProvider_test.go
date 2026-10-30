// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var samlNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func samlCertificate(t *testing.T, from, until time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "idp"},
		NotBefore: from, NotAfter: until,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func samlInput(t *testing.T) identity.NewIdentityProviderInput {
	return identity.NewIdentityProviderInput{
		ID: shared.ID("11111111-1111-1111-1111-111111111111"), TenantID: shared.ID("22222222-2222-2222-2222-222222222222"),
		Issuer: "http://www.example.org/exk1/", ClientID: "https://hubtask.example/api/v1/auth/saml:metadata",
		Kind: "SAML", Now: samlNow, AllowedEmailDomains: []string{"example.org"},
		SAMLSSOURL:      "https://idp.example.org/sso?tenant=1",
		SAMLCertificate: samlCertificate(t, samlNow.AddDate(-1, 0, 0), samlNow.AddDate(1, 0, 0)),
	}
}

// UC-ID-30 check 1: a SAML provider is configured from its entity ID, single sign-on address and
// certificate; what is not one is refused with a code naming the field.
func TestASAMLProviderIsConfiguredWhole(t *testing.T) {
	valid := samlCertificate(t, samlNow.AddDate(-1, 0, 0), samlNow.AddDate(1, 0, 0))
	expired := samlCertificate(t, samlNow.AddDate(-2, 0, 0), samlNow.AddDate(0, 0, -1))
	tests := []struct {
		name   string
		change func(*identity.NewIdentityProviderInput)
		code   string
	}{
		{name: "whole", change: func(*identity.NewIdentityProviderInput) {}},
		{name: "a urn entity id", change: func(in *identity.NewIdentityProviderInput) { in.Issuer = "urn:example:idp" }},
		{name: "two certificates for a rollover", change: func(in *identity.NewIdentityProviderInput) { in.SAMLCertificate = valid + valid }},
		{name: "an entity id that is no URI", change: func(in *identity.NewIdentityProviderInput) { in.Issuer = "idp example" }, code: "identity_provider.issuer_invalid"},
		{name: "a plain-http sign-on address", change: func(in *identity.NewIdentityProviderInput) { in.SAMLSSOURL = "http://idp.example.org/sso" }, code: "identity_provider.saml_sso_url_invalid"},
		{name: "no certificate", change: func(in *identity.NewIdentityProviderInput) { in.SAMLCertificate = "" }, code: "identity_provider.saml_certificate_invalid"},
		{name: "not a certificate", change: func(in *identity.NewIdentityProviderInput) { in.SAMLCertificate = "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n" }, code: "identity_provider.saml_certificate_invalid"},
		{name: "three certificates", change: func(in *identity.NewIdentityProviderInput) { in.SAMLCertificate = valid + valid + valid }, code: "identity_provider.saml_certificate_invalid"},
		{name: "an expired certificate", change: func(in *identity.NewIdentityProviderInput) { in.SAMLCertificate = expired }, code: "identity_provider.saml_certificate_expired"},
		{name: "offered by the installation", change: func(in *identity.NewIdentityProviderInput) { in.TenantID = ""; in.Provisioning = "INVITED_ONLY" }, code: "identity_provider.saml_workspace_only"},
		{name: "SAML fields on an OpenID Connect provider", change: func(in *identity.NewIdentityProviderInput) {
			in.Kind = "GENERIC"
			in.Issuer = "https://idp.example.org"
		}, code: "identity_provider.saml_only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := samlInput(t)
			tt.change(&in)
			got, err := identity.NewIdentityProvider(in)
			if tt.code == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				if !got.IsSAML() || got.Issuer != strings.TrimSpace(in.Issuer) || got.SAMLSSOURL == "" ||
					!strings.HasPrefix(got.SAMLCertificate, "-----BEGIN CERTIFICATE-----") {
					t.Fatalf("configured as %+v", got)
				}
				// The default admission of a workspace's provider without a directory claim.
				if got.Provisioning != identity.ProvisionDomains {
					t.Fatalf("provisioning %s", got.Provisioning)
				}
				return
			}
			var refused *shared.Error
			if !errors.As(err, &refused) || refused.DetailCode != tt.code {
				t.Fatalf("want %s, got %v", tt.code, err)
			}
		})
	}
}

// The entity ID is a name: its trailing slash stays, and no issuer host makes a provider SAML.
func TestASAMLEntityIDIsKeptExactly(t *testing.T) {
	in := samlInput(t)
	in.Issuer = "https://sts.windows.net/0000/"
	got, err := identity.NewIdentityProvider(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Issuer != "https://sts.windows.net/0000/" {
		t.Fatalf("issuer %q", got.Issuer)
	}
	if identity.KindOfIssuer("idp.example.org") == identity.KindSAML {
		t.Fatal("an issuer host was read as SAML")
	}
}
