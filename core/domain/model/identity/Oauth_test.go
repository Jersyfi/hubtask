// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func validClientInput() NewOauthClientInput {
	return NewOauthClientInput{
		ID: shared.ID("0192f000-0000-7000-8000-0000000000c1"), TenantID: sessionTenant,
		Name: "Zapier", Confidential: true,
		RedirectURIs: []string{"https://zapier.example/callback", "https://zapier.example/callback"},
		Now:          time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestNewOauthClientValidatesAndDeduplicates(t *testing.T) {
	client, err := NewOauthClient(validClientInput())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	if len(client.RedirectURIs) != 1 {
		t.Errorf("uris %v, want the duplicate collapsed", client.RedirectURIs)
	}
	if !client.AllowsRedirect("https://zapier.example/callback") {
		t.Error("the registered URI is refused")
	}
	// Exact match, and nothing cleverer.
	for _, probe := range []string{
		"https://zapier.example/callback/", "https://zapier.example/callback?x=1",
		"https://zapier.example/", "https://zapier.example.evil/callback",
		"http://zapier.example/callback",
	} {
		if client.AllowsRedirect(probe) {
			t.Errorf("%q passed the exact match", probe)
		}
	}
}

func TestNewOauthClientRefusesTheBroken(t *testing.T) {
	cases := map[string]func(*NewOauthClientInput){
		"no name":       func(in *NewOauthClientInput) { in.Name = "  " },
		"no uris":       func(in *NewOauthClientInput) { in.RedirectURIs = nil },
		"a relative":    func(in *NewOauthClientInput) { in.RedirectURIs = []string{"/callback"} },
		"a fragment":    func(in *NewOauthClientInput) { in.RedirectURIs = []string{"https://a.example/cb#frag"} },
		"not a url":     func(in *NewOauthClientInput) { in.RedirectURIs = []string{"::"} },
		"too many uris": func(in *NewOauthClientInput) { in.RedirectURIs = manyURIs(MaxOauthRedirectURIs + 1) },
	}
	for name, wound := range cases {
		in := validClientInput()
		wound(&in)
		if _, err := NewOauthClient(in); err == nil {
			t.Errorf("%s registered", name)
		}
	}
}

func manyURIs(n int) []string {
	uris := make([]string, 0, n)
	for i := range n {
		uris = append(uris, "https://a.example/cb/"+strings.Repeat("x", i+1))
	}
	return uris
}

// RFC 7636 §4.6: the verifier proves the challenge, and nothing shorter, longer or wrong does.
func TestVerifyPKCE(t *testing.T) {
	verifier := strings.Repeat("v", 43)
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])

	if err := CheckPKCEChallenge(challenge); err != nil {
		t.Fatalf("a real challenge refused: %v", err)
	}
	if !VerifyPKCE(verifier, challenge) {
		t.Fatal("the right verifier was refused")
	}
	for name, probe := range map[string]string{
		"a wrong verifier": strings.Repeat("w", 43),
		"too short":        strings.Repeat("v", 42),
		"too long":         strings.Repeat("v", 129),
		"empty":            "",
	} {
		if VerifyPKCE(probe, challenge) {
			t.Errorf("%s verified", name)
		}
	}
	if err := CheckPKCEChallenge("not-a-challenge"); err == nil {
		t.Error("a malformed challenge passed")
	}
}

func TestOauthCodeAndSecretShapes(t *testing.T) {
	code, err := NewOauthCode(sessionTenant, sessionSecret())
	if err != nil || !strings.HasPrefix(code.Secret(), OauthCodePrefix) {
		t.Fatalf("code %q, %v", code.Secret(), err)
	}
	parsed, err := ParseOauthCode(code.Secret())
	if err != nil || parsed.TenantID() != sessionTenant {
		t.Fatalf("parsing: %v, %v", parsed.TenantID(), err)
	}
	if _, err := ParseOauthCode(strings.Replace(code.Secret(), OauthCodePrefix, RefreshTokenPrefix, 1)); err == nil {
		t.Error("a refresh-prefixed token parsed as a code")
	}
	secret, err := NewOauthClientSecret(sessionTenant, sessionSecret())
	if err != nil || !strings.HasPrefix(secret.Secret(), OauthClientSecretPrefix) {
		t.Fatalf("secret %q, %v", secret.Secret(), err)
	}
}

func TestAGrantVerifiesUntilRevoked(t *testing.T) {
	grant := OauthGrant{ID: shared.ID("0192f000-0000-7000-8000-0000000000c2")}
	if err := grant.Verify(); err != nil {
		t.Errorf("a live grant refused: %v", err)
	}
	grant.RevokedAt = time.Now()
	if err := grant.Verify(); err == nil {
		t.Error("a revoked grant verified")
	}
}
