// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package oidc

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	port "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The T-13 suite (security.md §4). Every tampering the threat row names is built here and shown
// to be refused - against a provider that really signs, because a fake that only pretends to
// sign proves nothing about a verifier.
//
// The tokens are assembled by hand rather than by a library. That is the point: `alg: none` and
// a signature over somebody else's key are exactly the documents a well-behaved library declines
// to produce, and they are the documents that have to be refused.

const (
	testClientID = "hubtask"
	testNonce    = "the-nonce-this-flow-minted"
	testSubject  = "provider-subject-1"
)

// idToken is the set of claims a fake provider signs, with every knob a tampering needs.
type idToken struct {
	issuer   string
	audience string
	subject  string
	nonce    string
	issuedAt time.Time
	expires  time.Time
	// algorithm is what the header claims. "none" produces an unsigned token.
	algorithm string
	// signWith is the key the signature is made with, which is not always the provider's.
	signWith *rsa.PrivateKey
	// omitNonce leaves the claim out entirely rather than blanking it.
	omitNonce bool
}

// fakeIDP is a provider: discovery, a key set, and a token endpoint that answers whatever the
// test told it to sign.
type fakeIDP struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	keyID  string
	// token is what the next exchange answers. Set per test.
	token idToken
	// omitIDToken answers an OAuth2 token with no identity token in it at all.
	omitIDToken bool
	// discoveries counts how often the metadata was fetched, for the caching test.
	discoveries int
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating the provider's key: %v", err)
	}

	idp := &fakeIDP{key: key, keyID: "k1"}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		idp.discoveries++
		writeJSON(w, map[string]any{
			"issuer":                 idp.issuer(),
			"authorization_endpoint": idp.issuer() + "/authorize",
			"token_endpoint":         idp.issuer() + "/token",
			"jwks_uri":               idp.issuer() + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": idp.keyID,
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		answer := map[string]any{"access_token": "at", "token_type": "Bearer"}
		if !idp.omitIDToken {
			answer["id_token"] = idp.sign(t, idp.token)
		}
		writeJSON(w, answer)
	})

	idp.server = httptest.NewServer(mux)
	t.Cleanup(idp.server.Close)
	idp.token = idp.wellFormed(time.Now())
	return idp
}

func (f *fakeIDP) issuer() string { return f.server.URL }

// wellFormed is the token every tampering below is a variation of.
func (f *fakeIDP) wellFormed(now time.Time) idToken {
	return idToken{
		issuer: f.issuer(), audience: testClientID, subject: testSubject, nonce: testNonce,
		issuedAt: now, expires: now.Add(time.Hour), algorithm: "RS256", signWith: f.key,
	}
}

// sign assembles the JWT by hand: header, claims, and whatever signature the case calls for.
func (f *fakeIDP) sign(t *testing.T, token idToken) string {
	t.Helper()

	header := map[string]any{"alg": token.algorithm, "typ": "JWT"}
	if token.algorithm != "none" {
		header["kid"] = f.keyID
	}
	claims := map[string]any{
		"iss": token.issuer, "aud": token.audience, "sub": token.subject,
		"iat": token.issuedAt.Unix(), "exp": token.expires.Unix(),
		"email": "ada@example.org", "email_verified": true, "name": "Ada",
	}
	if !token.omitNonce {
		claims["nonce"] = token.nonce
	}

	signing := segment(t, header) + "." + segment(t, claims)
	if token.algorithm == "none" {
		// An unsigned token, which is the whole of the `alg: none` attack: the document says it
		// needs no signature, and a verifier that believes it accepts anything.
		return signing + "."
	}

	digest := sha256.Sum256([]byte(signing))
	signature, err := rsa.SignPKCS1v15(rand.Reader, token.signWith, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("signing the token: %v", err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func segment(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding a token segment: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func configFor(idp *fakeIDP) port.Config {
	return port.Config{
		Issuer: idp.issuer(), ClientID: testClientID,
		ClientSecret: secret.New("s3cr3t"),
		RedirectURL:  "https://hubtask.example/auth/callback",
	}
}

func relyingParty(idp *fakeIDP, now time.Time) *Provider {
	return New(idp.server.Client(), clockport.Fixed(now))
}

// The happy path first, because a suite of refusals proves nothing if nothing is ever accepted.
func TestAWellFormedTokenIsAccepted(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.token = idp.wellFormed(now)

	identity, err := relyingParty(idp, now).Exchange(t.Context(), configFor(idp), port.Exchange{
		Code: "the-code", CodeVerifier: strings.Repeat("v", 43), Nonce: testNonce,
	})
	if err != nil {
		t.Fatalf("a well-formed token was refused: %v", err)
	}
	if identity.Subject != testSubject {
		t.Errorf("the subject is %q", identity.Subject)
	}
	if identity.Email != "ada@example.org" || !identity.EmailVerified {
		t.Errorf("the address came back as %q (verified: %v)", identity.Email, identity.EmailVerified)
	}
	if identity.DisplayName != "Ada" {
		t.Errorf("the name is %q", identity.DisplayName)
	}
}

// T-13, one case per tampering the threat row names. Each is refused, and each is refused with
// the same code: which check caught it helps whoever forged the token and nobody else.
func TestEveryTamperingIsRefused(t *testing.T) {
	now := time.Now()
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating a second key: %v", err)
	}

	cases := []struct {
		name   string
		tamper func(idp *fakeIDP, token *idToken)
		nonce  string
	}{
		{
			name:   "a wrong issuer",
			tamper: func(_ *fakeIDP, token *idToken) { token.issuer = "https://elsewhere.example" },
		},
		{
			name:   "a wrong audience - a token minted for another client",
			tamper: func(_ *fakeIDP, token *idToken) { token.audience = "somebody-else" },
		},
		{
			name: "an expired token",
			tamper: func(_ *fakeIDP, token *idToken) {
				token.issuedAt = now.Add(-2 * time.Hour)
				token.expires = now.Add(-time.Hour)
			},
		},
		{
			name:   "a signature by a key that is not the provider's",
			tamper: func(_ *fakeIDP, token *idToken) { token.signWith = otherKey },
		},
		{
			name:   "a stripped nonce",
			tamper: func(_ *fakeIDP, token *idToken) { token.omitNonce = true },
		},
		{
			name:   "a nonce belonging to another flow",
			tamper: func(_ *fakeIDP, token *idToken) { token.nonce = "somebody-elses-nonce" },
		},
		{
			name:   "alg: none - a token that says it needs no signature",
			tamper: func(_ *fakeIDP, token *idToken) { token.algorithm = "none" },
		},
		{
			name: "issued further ahead than a clock explains",
			tamper: func(_ *fakeIDP, token *idToken) {
				token.issuedAt = now.Add(10 * time.Minute)
				token.expires = now.Add(2 * time.Hour)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			idp := newFakeIDP(t)
			token := idp.wellFormed(now)
			c.tamper(idp, &token)
			idp.token = token

			_, err := relyingParty(idp, now).Exchange(t.Context(), configFor(idp), port.Exchange{
				Code: "the-code", CodeVerifier: strings.Repeat("v", 43), Nonce: testNonce,
			})
			if err == nil {
				t.Fatal("the tampered token was accepted")
			}
			if code := shared.AsError(err).DetailCode; code != "auth.identity_token_invalid" {
				t.Errorf("the refusal is %q, want the one indistinguishable code", code)
			}
		})
	}
}

// A provider that speaks OAuth2 and answers no identity token is not an OpenID provider, however
// well it spoke discovery.
func TestAnAnswerWithoutAnIdentityTokenIsRefused(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.omitIDToken = true

	_, err := relyingParty(idp, now).Exchange(t.Context(), configFor(idp), port.Exchange{
		Code: "the-code", CodeVerifier: strings.Repeat("v", 43), Nonce: testNonce,
	})
	if err == nil {
		t.Fatal("an answer with no identity token was accepted")
	}
	if code := shared.AsError(err).DetailCode; code != "auth.identity_token_missing" {
		t.Errorf("the refusal is %q", code)
	}
}

// The degradation observability-reliability.md §7 promises, proved rather than asserted: with the
// provider stopped, both halves of the flow refuse with the code that says it is the provider -
// which is what lets everything else, and every local account, carry on.
func TestAStoppedProviderDegradesWithItsOwnCode(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	config := configFor(idp)
	party := relyingParty(idp, now)

	// The container stops.
	idp.server.Close()

	if err := party.Check(t.Context(), config.Issuer); err == nil {
		t.Error("a stopped provider passed the configuration check")
	} else if code := shared.AsError(err).DetailCode; code != "auth.provider_unreachable" {
		t.Errorf("the check refused with %q", code)
	}

	_, err := party.AuthorizationURL(t.Context(), config, port.Authorization{
		State: "s", Nonce: testNonce, CodeVerifier: strings.Repeat("v", 43),
	})
	if err == nil {
		t.Fatal("a stopped provider produced an authorization URL")
	}
	if code := shared.AsError(err).DetailCode; code != "auth.provider_unreachable" {
		t.Errorf("the start refused with %q, want the provider's own code", code)
	}
}

// Discovery is fetched once and reused. What is cached is the metadata - the endpoints - and not
// the keys: the key set behind it refetches on its own when a token arrives signed by a key it
// does not know, which is how a rotation survives without anybody restarting anything.
func TestDiscoveryIsFetchedOnceAndReused(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	party := relyingParty(idp, now)

	for range 3 {
		if _, err := party.AuthorizationURL(t.Context(), configFor(idp), port.Authorization{
			State: "s", Nonce: testNonce, CodeVerifier: strings.Repeat("v", 43),
		}); err != nil {
			t.Fatalf("building the authorization URL: %v", err)
		}
	}
	if idp.discoveries != 1 {
		t.Errorf("discovery was fetched %d times, want once", idp.discoveries)
	}
}

// An issuer whose metadata names a different issuer is refused - RFC 8414's own check, and the
// reason discovery is a library's job rather than a document fetch.
func TestAnIssuerThatDisagreesWithItsOwnMetadataIsRefused(t *testing.T) {
	liar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "openid-configuration") {
			writeJSON(w, map[string]any{
				"issuer":                 "https://somebody.else.example",
				"authorization_endpoint": "https://somebody.else.example/authorize",
				"token_endpoint":         "https://somebody.else.example/token",
				"jwks_uri":               "https://somebody.else.example/jwks",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer liar.Close()

	err := New(liar.Client(), clockport.Fixed(time.Now())).Check(t.Context(), liar.URL)
	if err == nil {
		t.Fatal("a provider that renamed itself in its own metadata was accepted")
	}
	if code := shared.AsError(err).DetailCode; code != "auth.provider_unreachable" {
		t.Errorf("the refusal is %q", code)
	}
}

// The authorization request carries what the flow needs, and the verifier is not among it: what
// travels is the challenge, and the verifier stays here until the exchange.
func TestTheAuthorizationRequestCarriesTheChallengeAndNotTheVerifier(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	verifier := strings.Repeat("v", 43)

	url, err := relyingParty(idp, now).AuthorizationURL(t.Context(), configFor(idp),
		port.Authorization{State: "the-state", Nonce: testNonce, CodeVerifier: verifier,
			LoginHint: "ada@example.org"})
	if err != nil {
		t.Fatalf("building the authorization URL: %v", err)
	}

	for _, want := range []string{
		"state=the-state", "nonce=" + testNonce, "code_challenge_method=S256",
		"login_hint=ada%40example.org", "client_id=" + testClientID,
	} {
		if !strings.Contains(url, want) {
			t.Errorf("the authorization URL does not carry %s:\n%s", want, url)
		}
	}
	if strings.Contains(url, verifier) {
		t.Error("the code verifier travelled to the provider - only its challenge may")
	}
}

// The adapter is the port, checked here as well as in the package itself: a test that builds
// against the interface is the one that notices a method disappearing.
var _ port.Port = (*Provider)(nil)
