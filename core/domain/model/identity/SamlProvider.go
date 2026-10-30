// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/pem"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A SAML 2.0 identity provider is a way in beside OpenID Connect (identity.md §26). What differs is
// only how the identity is proven - a signed assertion instead of an ID token - so it is a kind of
// identity provider with the same admission, the same arrival and the same session; what is its own
// is held here: the provider's entity ID as the issuer, compared exactly, the single sign-on address
// and the certificates its assertions are signed with.

// KindSAML is a SAML 2.0 identity provider. It draws the letter tile, is stated rather than read
// from an issuer, and is a workspace's own provider only.
const KindSAML ProviderKind = "SAML"

// The bounds of a SAML provider's configuration.
const (
	MaxSAMLEntityID      = 500
	MaxSAMLSSOURL        = 2000
	MaxSAMLCertificate   = 20000
	MaxSAMLCertificates  = 2
	samlCertificateBlock = "CERTIFICATE"
)

// IsSAML reports whether the provider speaks SAML rather than OpenID Connect.
func (p IdentityProvider) IsSAML() bool { return p.Kind == KindSAML }

// samlEntityID holds an entity ID to what SAML says one is: an absolute URI, compared exactly. Not
// an issuer URL - an entity ID is a name and is never fetched, so `http:` and `urn:` are what
// providers publish and a trailing slash is part of it.
func samlEntityID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", shared.ErrValidation.WithDetail("identity_provider.issuer_required")
	}
	parsed, err := url.Parse(id)
	if err != nil || parsed.Scheme == "" || len(id) > MaxSAMLEntityID || strings.ContainsFunc(id, unicode.IsSpace) {
		return "", shared.ErrValidation.WithDetail("identity_provider.issuer_invalid").
			WithParams(map[string]string{"issuer": id})
	}
	return id, nil
}

// samlSSOURL holds the single sign-on address to https: the browser carries the request there, and
// a plain-HTTP address is one anybody on the path answers for.
func samlSSOURL(raw string) (string, error) {
	address := strings.TrimSpace(raw)
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.Fragment != "" || len(address) > MaxSAMLSSOURL {
		return "", shared.ErrValidation.WithDetail("identity_provider.saml_sso_url_invalid").
			WithFields(shared.FieldError{Path: "/saml_sso_url", Code: "identity_provider.saml_sso_url_invalid"})
	}
	return address, nil
}

// SAMLCertificates reads the PEM a SAML provider signs with: one or two certificates (a rollover),
// each valid at now, nothing else in the text. It answers them re-encoded, so what is stored is
// exactly what was checked.
func SAMLCertificates(raw string, now time.Time) (string, []*x509.Certificate, error) {
	refused := func(code string) error {
		return shared.ErrValidation.WithDetail(code).
			WithFields(shared.FieldError{Path: "/saml_certificate", Code: code})
	}
	if len(raw) > MaxSAMLCertificate {
		return "", nil, refused("identity_provider.saml_certificate_invalid")
	}
	rest := []byte(strings.TrimSpace(raw))
	var (
		certs []*x509.Certificate
		out   strings.Builder
	)
	for len(rest) > 0 {
		block, remainder := pem.Decode(rest)
		if block == nil || block.Type != samlCertificateBlock {
			return "", nil, refused("identity_provider.saml_certificate_invalid")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return "", nil, refused("identity_provider.saml_certificate_invalid")
		}
		if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
			return "", nil, refused("identity_provider.saml_certificate_expired")
		}
		certs = append(certs, cert)
		out.Write(pem.EncodeToMemory(&pem.Block{Type: samlCertificateBlock, Bytes: block.Bytes}))
		rest = []byte(strings.TrimSpace(string(remainder)))
	}
	if len(certs) == 0 || len(certs) > MaxSAMLCertificates {
		return "", nil, refused("identity_provider.saml_certificate_invalid")
	}
	return out.String(), certs, nil
}

// samlFields checks what only a SAML provider holds: required for one, refused for every other
// kind, because a field nothing reads is somebody's belief that it did something.
func samlFields(in NewIdentityProviderInput, kind ProviderKind) (string, string, error) {
	if kind != KindSAML {
		if strings.TrimSpace(in.SAMLSSOURL) != "" || strings.TrimSpace(in.SAMLCertificate) != "" {
			return "", "", shared.ErrValidation.WithDetail("identity_provider.saml_only")
		}
		return "", "", nil
	}
	if in.TenantID.IsZero() {
		return "", "", shared.ErrValidation.WithDetail("identity_provider.saml_workspace_only")
	}
	sso, err := samlSSOURL(in.SAMLSSOURL)
	if err != nil {
		return "", "", err
	}
	certificate, _, err := SAMLCertificates(in.SAMLCertificate, in.Now)
	if err != nil {
		return "", "", err
	}
	return sso, certificate, nil
}

// SAMLHandoffPrefix labels the single-use code the assertion consumer service hands the callback.
const SAMLHandoffPrefix = "hbt_sam_"

// MaxSAMLResponse bounds the answer the assertion consumer service keeps (base64, as posted).
const MaxSAMLResponse = 256 << 10

// SAMLHandoffHash is what the flow keeps of the handoff code: its digest, as every presented token.
func SAMLHandoffHash(code string) []byte {
	sum := sha256.Sum256([]byte(code))
	return sum[:]
}

// SAMLHandoffMatches reports whether a presented code is the one the flow's answer was handed off
// with; a flow holding none matches nothing.
func SAMLHandoffMatches(held []byte, presented string) bool {
	return len(held) == sha256.Size && subtle.ConstantTimeCompare(held, SAMLHandoffHash(presented)) == 1
}
