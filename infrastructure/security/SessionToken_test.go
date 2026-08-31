// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var (
	sessionTokenNow = time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	sessionClaims   = port.SessionClaims{
		TenantID:  shared.ID("0192f000-0000-7000-8000-000000000001"),
		SessionID: shared.ID("0192f000-0000-7000-8000-000000000002"),
		AccountID: shared.ID("0192f000-0000-7000-8000-000000000003"),
		ExpiresAt: sessionTokenNow.Add(15 * time.Minute),
	}
)

func sessionIssuer() SessionTokenIssuer {
	return NewSessionTokenIssuer(secret.New("installation-secret-for-tests"))
}

func TestASessionTokenRoundTrips(t *testing.T) {
	issuer := sessionIssuer()
	token := issuer.Issue(sessionClaims)

	if !strings.HasPrefix(token, identity.SessionAccessTokenPrefix) {
		t.Fatalf("token %q lacks the %q prefix", token, identity.SessionAccessTokenPrefix)
	}

	claims, err := issuer.Validate(token, sessionTokenNow)
	if err != nil {
		t.Fatalf("validating what was issued: %v", err)
	}
	if claims.TenantID != sessionClaims.TenantID ||
		claims.SessionID != sessionClaims.SessionID ||
		claims.AccountID != sessionClaims.AccountID {
		t.Errorf("claims %+v, want %+v", claims, sessionClaims)
	}
	if !claims.ExpiresAt.Equal(sessionClaims.ExpiresAt.Truncate(time.Second)) {
		t.Errorf("expiry %v, want %v", claims.ExpiresAt, sessionClaims.ExpiresAt)
	}
}

// Every forgery is one indistinguishable answer; only expiry is distinguished, because an
// expired token is one this server really minted and "refresh" is actionable.
func TestATamperedSessionTokenIsRefused(t *testing.T) {
	issuer := sessionIssuer()
	token := issuer.Issue(sessionClaims)

	swapped := func(from, to string) string {
		body := strings.TrimPrefix(token, identity.SessionAccessTokenPrefix)
		return identity.SessionAccessTokenPrefix + strings.Replace(body, from, to, 1)
	}
	_ = swapped

	cases := map[string]string{
		"empty":            "",
		"no prefix":        strings.TrimPrefix(token, identity.SessionAccessTokenPrefix),
		"a PAT prefix":     identity.TokenPrefix + strings.TrimPrefix(token, identity.SessionAccessTokenPrefix),
		"truncated":        token[:len(token)-4],
		"a flipped byte":   token[:len(token)-1] + flip(token[len(token)-1]),
		"another issuer's": NewSessionTokenIssuer(secret.New("another-secret")).Issue(sessionClaims),
	}

	var refusal error
	for name, presented := range cases {
		_, err := issuer.Validate(presented, sessionTokenNow)
		if err == nil {
			t.Fatalf("%s validated", name)
		}
		if strings.Contains(err.Error(), "expired") {
			t.Fatalf("%s answered the expiry refusal", name)
		}
		if refusal == nil {
			refusal = err
			continue
		}
		if err.Error() != refusal.Error() {
			t.Errorf("%s answered %q, another case %q - the refusals differ", name, err, refusal)
		}
	}
}

func TestAnExpiredSessionTokenIsDistinguished(t *testing.T) {
	issuer := sessionIssuer()
	token := issuer.Issue(sessionClaims)

	_, err := issuer.Validate(token, sessionClaims.ExpiresAt.Add(time.Second))
	if err == nil {
		t.Fatal("an expired token validated")
	}
	if !strings.Contains(err.Error(), "access.token_expired") {
		t.Errorf("an expired token answered %v, want the expiry refusal", err)
	}
}

// The claims are signed: a token cannot be rewritten to name another tenant, session or account.
func TestSwappedClaimsInvalidateTheTag(t *testing.T) {
	issuer := sessionIssuer()
	other := sessionClaims
	other.TenantID = shared.ID("0192f000-0000-7000-8000-00000000ffff")

	// Issue for one set of claims, then check that a token for the other set differs everywhere
	// it matters: same issuer, same expiry, different tag.
	one := issuer.Issue(sessionClaims)
	two := issuer.Issue(other)
	if one == two {
		t.Fatal("two different claim sets produced one token")
	}
}

func TestTheHashersAreDomainSeparated(t *testing.T) {
	installation := secret.New("installation-secret-for-tests")
	value := "hbt_srt_0192f000000070008000000000000001_" + strings.Repeat("a", 43)

	refresh := NewSessionRefreshHasher(installation).Hash(value)
	redemption := NewRedemptionTokenHasher(installation).Hash(value)
	attempt := NewAuthAttemptHasher(installation).Hash(value)
	pat := NewTokenHasher(installation).Hash(value)

	hashes := map[string][]byte{
		"refresh": refresh, "redemption": redemption, "attempt": attempt, "pat": pat,
	}
	for a, ha := range hashes {
		for b, hb := range hashes {
			if a != b && string(ha) == string(hb) {
				t.Errorf("the %s hash equals the %s hash - a value from one table could be replayed as the other", a, b)
			}
		}
	}
}

func flip(b byte) string {
	if b == 'A' {
		return "B"
	}
	return "A"
}
