// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package oidc

import (
	"strings"
	"testing"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/identityprovider"
)

// ADR-0075 §2: a step-up at the provider asks for a fresh sign-in there - `prompt=login` and
// `max_age=0` - and reads back when the person last authenticated, `auth_time`, so the core can
// refuse a provider that answered from its own session instead.

func TestAFreshAuthorizationAsksTheProviderToSignInAgain(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)

	fresh, err := relyingParty(idp, now).AuthorizationURL(t.Context(), configFor(idp),
		port.Authorization{State: "s", Nonce: testNonce, CodeVerifier: strings.Repeat("v", 43), Fresh: true})
	if err != nil {
		t.Fatalf("building the authorization URL: %v", err)
	}
	for _, want := range []string{"prompt=login", "max_age=0"} {
		if !strings.Contains(fresh, want) {
			t.Errorf("a fresh authorization does not carry %s:\n%s", want, fresh)
		}
	}

	// A sign-in asks for nothing of the kind: the person's own session at the provider is welcome.
	signIn, err := relyingParty(idp, now).AuthorizationURL(t.Context(), configFor(idp),
		port.Authorization{State: "s", Nonce: testNonce, CodeVerifier: strings.Repeat("v", 43)})
	if err != nil {
		t.Fatalf("building the authorization URL: %v", err)
	}
	if strings.Contains(signIn, "prompt=") || strings.Contains(signIn, "max_age=") {
		t.Errorf("a sign-in asked the provider to sign the person in again:\n%s", signIn)
	}
}

func TestTheIdentityCarriesWhenThePersonLastSignedIn(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	idp := newFakeIDP(t)
	idp.token = idp.wellFormed(now)
	idp.extraClaims = map[string]any{"auth_time": now.Add(-time.Minute).Unix()}

	identity, err := relyingParty(idp, now).Exchange(t.Context(), configFor(idp), port.Exchange{
		Code: "the-code", CodeVerifier: strings.Repeat("v", 43), Nonce: testNonce,
	})
	if err != nil {
		t.Fatalf("exchanging: %v", err)
	}
	if want := now.Add(-time.Minute); !identity.AuthTime.Equal(want) {
		t.Errorf("auth_time came back as %v, want %v", identity.AuthTime, want)
	}

	// A token without the claim answers zero - which the core reads as "nothing fresh was proven".
	idp.extraClaims = nil
	identity, err = relyingParty(idp, now).Exchange(t.Context(), configFor(idp), port.Exchange{
		Code: "the-code", CodeVerifier: strings.Repeat("v", 43), Nonce: testNonce,
	})
	if err != nil {
		t.Fatalf("exchanging: %v", err)
	}
	if !identity.AuthTime.IsZero() {
		t.Errorf("a token without auth_time answered %v", identity.AuthTime)
	}
}

// RFC 7519 lets a NumericDate carry a fraction. A provider that sends one must not lose every
// sign-in for a claim only a step-up reads.
func TestAFractionalAuthTimeIsRead(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	idp := newFakeIDP(t)
	idp.token = idp.wellFormed(now)
	idp.extraClaims = map[string]any{"auth_time": float64(now.Add(-time.Minute).Unix()) + 0.5}

	identity, err := relyingParty(idp, now).Exchange(t.Context(), configFor(idp), port.Exchange{
		Code: "the-code", CodeVerifier: strings.Repeat("v", 43), Nonce: testNonce,
	})
	if err != nil {
		t.Fatalf("a fractional auth_time refused the sign-in: %v", err)
	}
	if want := now.Add(-time.Minute); !identity.AuthTime.Equal(want) {
		t.Errorf("auth_time came back as %v, want %v", identity.AuthTime, want)
	}
}
