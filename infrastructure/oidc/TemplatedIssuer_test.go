// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package oidc

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/identityprovider"
)

// A multi-directory endpoint, and the exact comparison that survives it (ADR-0071 §3).
//
// What these prove is the one thing that is easy to get wrong and impossible to see: that
// substituting the token's own `tid` into the published template does **not** turn the comparison
// into a pattern match. A token still has to name the directory its `iss` names, and both still
// have to be signed by the key set the endpoint published.

const directory = "72f988bf-86f1-41af-91ab-2d7cd011db47"

// The case the whole decision exists for: the endpoint is an address, the document publishes a
// template, and a token from one directory verifies against that directory's issuer.
func TestATemplatedIssuerIsSubstitutedAndThenCompared(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.published = idp.issuer() + "/{tenantid}/v2.0"
	idp.extraClaims = map[string]any{"tid": directory}
	idp.token = idp.wellFormed(now)
	idp.token.issuer = idp.issuer() + "/" + directory + "/v2.0"

	cfg := configFor(idp)
	cfg.DirectoryClaim = "tid"
	identity, err := relyingParty(idp, now).Exchange(t.Context(), cfg, exchange())
	if err != nil {
		t.Fatalf("exchanging against a templated issuer: %v", err)
	}
	if identity.Directory != directory {
		t.Errorf("the directory was read as %q", identity.Directory)
	}
}

// And the comparison is still exact. A token whose `iss` names one directory while `tid` names
// another is the forgery a regular expression would wave through, because both match the shape.
func TestATokenWhoseIssuerAndDirectoryDisagreeIsRefused(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.published = idp.issuer() + "/{tenantid}/v2.0"
	idp.extraClaims = map[string]any{"tid": "11111111-2222-3333-4444-555555555555"}
	idp.token = idp.wellFormed(now)
	idp.token.issuer = idp.issuer() + "/" + directory + "/v2.0"

	cfg := configFor(idp)
	cfg.DirectoryClaim = "tid"
	if _, err := relyingParty(idp, now).Exchange(t.Context(), cfg, exchange()); err == nil {
		t.Fatal("a token naming two different directories was accepted")
	}
}

// A token that names no directory at all cannot be checked against a template, so it is refused
// rather than checked against something else.
func TestATemplatedIssuerRefusesATokenWithNoDirectory(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.published = idp.issuer() + "/{tenantid}/v2.0"
	idp.token = idp.wellFormed(now)
	idp.token.issuer = idp.issuer() + "/" + directory + "/v2.0"

	cfg := configFor(idp)
	cfg.DirectoryClaim = "tid"
	if _, err := relyingParty(idp, now).Exchange(t.Context(), cfg, exchange()); err == nil {
		t.Fatal("a token with no directory verified against a template")
	}
}

// The bound on accepting a published issuer at all: the same host. A document that names somebody
// else's host is handing this installation's trust away, and that is the mismatch the check exists
// for — `TestAnIssuerThatDisagreesWithItsOwnMetadataIsRefused` is its other half.
func TestAPublishedIssuerAtAnotherHostIsStillRefused(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.published = "https://somebody.else.example/{tenantid}/v2.0"

	cfg := configFor(idp)
	cfg.DirectoryClaim = "tid"
	err := relyingParty(idp, now).Check(t.Context(), cfg.Issuer)
	if err == nil {
		t.Fatal("a document naming another host was accepted")
	}
	if code := shared.AsError(err).DetailCode; code != "auth.provider_unreachable" {
		t.Errorf("refused with %q", code)
	}
}

// The fixed-directory endpoint: the document names a directory the address did not, and it is that
// value a token carries. `/consumers` at Microsoft, which is where personal accounts live.
func TestAFixedPublishedIssuerBecomesTheOneCompared(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.published = idp.issuer() + "/" + directory + "/v2.0"
	idp.extraClaims = map[string]any{"tid": directory}
	idp.token = idp.wellFormed(now)
	idp.token.issuer = idp.published

	cfg := configFor(idp)
	cfg.DirectoryClaim = "tid"
	if _, err := relyingParty(idp, now).Exchange(t.Context(), cfg, exchange()); err != nil {
		t.Fatalf("exchanging against a fixed published issuer: %v", err)
	}
}

// Microsoft answers `email_verified`'s question under another name. Without this the address is
// unverified, the domain refuses, and no Microsoft account signs in at all — which is what it did.
func TestMicrosoftsDomainOwnerClaimIsReadAsVerification(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.extraClaims = map[string]any{"tid": directory, "xms_edov": true}
	idp.token = idp.wellFormed(now)

	cfg := configFor(idp)
	cfg.DirectoryClaim = "tid"
	cfg.Authority = port.Authority{Claim: "xms_edov"}
	identity, err := relyingParty(idp, now).Exchange(t.Context(), cfg, exchange())
	if err != nil {
		t.Fatalf("exchanging: %v", err)
	}
	if !identity.EmailVerified {
		t.Error("xms_edov was not read as verification")
	}
	if !identity.AddressAuthoritative {
		t.Error("xms_edov was not read as authority over the address's domain")
	}
}

// And it is read in the refusing direction too: `xms_edov: false` is a provider saying the domain
// is *not* verified, which outranks an `email_verified` that is not there.
func TestMicrosoftsDomainOwnerClaimRefusesWhenItSaysNo(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.extraClaims = map[string]any{"tid": directory, "xms_edov": false}
	idp.token = idp.wellFormed(now)

	cfg := configFor(idp)
	cfg.DirectoryClaim = "tid"
	cfg.Authority = port.Authority{Claim: "xms_edov"}
	identity, err := relyingParty(idp, now).Exchange(t.Context(), cfg, exchange())
	if err != nil {
		t.Fatalf("exchanging: %v", err)
	}
	if identity.EmailVerified || identity.AddressAuthoritative {
		t.Error("a provider saying the domain is unverified was read as saying it is")
	}
}

// Google's rule, in its own words: a verified address is not an organisation's. The directory has
// to be the address's own domain before the address is authoritative.
func TestAVerifiedAddressOutsideItsDirectoryIsNotAuthoritative(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.extraClaims = map[string]any{"hd": "acme.example"}
	idp.token = idp.wellFormed(now)

	cfg := configFor(idp)
	cfg.DirectoryClaim = "hd"
	cfg.Authority = port.Authority{DirectoryIsDomain: true}
	identity, err := relyingParty(idp, now).Exchange(t.Context(), cfg, exchange())
	if err != nil {
		t.Fatalf("exchanging: %v", err)
	}
	// The fake signs `ada@example.org`, and the directory says acme.example.
	if !identity.EmailVerified {
		t.Error("a verified address was read as unverified")
	}
	if identity.AddressAuthoritative {
		t.Error("an address outside its own directory was treated as the organisation's")
	}
}

// An issuer that signs with nothing this installation accepts is refused while somebody is still
// looking at the form, rather than by the first person who tries to sign in (ADR-0071 §5).
func TestAnIssuerThatSignsWithNothingWeAcceptIsRefusedAtConfiguration(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.algorithms = []string{"HS256", "none"}

	err := relyingParty(idp, now).Check(t.Context(), idp.issuer())
	if err == nil {
		t.Fatal("an issuer that signs with nothing acceptable was configured")
	}
	if code := shared.AsError(err).DetailCode; code != "identity_provider.signing_unsupported" {
		t.Errorf("refused with %q", code)
	}
	if !strings.Contains(shared.AsError(err).Params["algorithms"], "HS256") {
		t.Error("the refusal does not say what the provider offered")
	}
}

// A document that names no algorithms is the specification's "RS256 is required", not "anything
// goes" — and RS256 is on the allowlist, so it passes.
func TestAnIssuerThatNamesNoAlgorithmsIsAccepted(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	if err := relyingParty(idp, now).Check(t.Context(), idp.issuer()); err != nil {
		t.Fatalf("an issuer naming no algorithms was refused: %v", err)
	}
}

func exchange() port.Exchange {
	return port.Exchange{Code: "code", CodeVerifier: "verifier", Nonce: testNonce}
}
