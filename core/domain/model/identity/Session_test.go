// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	sessionNow    = time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	sessionTenant = shared.ID("0192f000-0000-7000-8000-000000000001")
	sessionActor  = shared.ID("0192f000-0000-7000-8000-000000000002")
)

func sessionSecret() []byte { return bytes.Repeat([]byte{0xAB}, TokenSecretBytes) }

func TestARefreshTokenRoundTrips(t *testing.T) {
	minted, err := NewRefreshToken(sessionTenant, sessionSecret())
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if !strings.HasPrefix(minted.Secret(), RefreshTokenPrefix) {
		t.Fatalf("minted %q, want the %q prefix", minted.Secret(), RefreshTokenPrefix)
	}

	parsed, err := ParseRefreshToken(minted.Secret())
	if err != nil {
		t.Fatalf("parsing what was minted: %v", err)
	}
	if parsed.TenantID() != sessionTenant {
		t.Errorf("tenant %q, want %q", parsed.TenantID(), sessionTenant)
	}
	if parsed.Secret() != minted.Secret() {
		t.Errorf("the parsed credential is not the minted one")
	}
}

func TestARedemptionTokenRoundTrips(t *testing.T) {
	minted, err := NewRedemptionToken(sessionTenant, sessionSecret())
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	parsed, err := ParseRedemptionToken(minted.Secret())
	if err != nil {
		t.Fatalf("parsing what was minted: %v", err)
	}
	if parsed.TenantID() != sessionTenant {
		t.Errorf("tenant %q, want %q", parsed.TenantID(), sessionTenant)
	}
}

// Every malformed credential is one indistinguishable answer, ParseToken's discipline.
func TestAMalformedPrefixedTokenIsOneAnswer(t *testing.T) {
	minted, _ := NewRefreshToken(sessionTenant, sessionSecret())

	cases := map[string]string{
		"empty":              "",
		"the wrong prefix":   strings.Replace(minted.Secret(), RefreshTokenPrefix, TokenPrefix, 1),
		"a PAT":              "hbt_pat_0192f000000070008000000000000001_" + strings.Repeat("a", 43),
		"a short secret":     RefreshTokenPrefix + "0192f000000070008000000000000001_" + strings.Repeat("a", 20),
		"a bad tenant":       RefreshTokenPrefix + strings.Repeat("z", 32) + "_" + strings.Repeat("a", 43),
		"no separator":       RefreshTokenPrefix + strings.Repeat("a", 76),
		"padding characters": RefreshTokenPrefix + "0192f000000070008000000000000001_" + strings.Repeat("a", 42) + "=",
	}

	var refusal error
	for name, raw := range cases {
		_, err := ParseRefreshToken(raw)
		if err == nil {
			t.Fatalf("%s parsed", name)
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

func TestNewSessionCoarsensTheClientHint(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		want   string
	}{
		{"an IPv4 peer with a port", "203.0.113.7:51234", "203.0.113.0/24"},
		{"a bare IPv4", "198.51.100.200", "198.51.100.0/24"},
		{"an IPv6 peer", "[2001:db8:aaaa:bbbb::7]:443", "2001:db8:aaaa::/48"},
		{"a mapped IPv4", "::ffff:203.0.113.7", "203.0.113.0/24"},
		{"garbage", "not-an-address", ""},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			session, err := NewSession(NewSessionInput{
				ID: shared.ID("0192f000-0000-7000-8000-00000000000a"), TenantID: sessionTenant,
				AccountID: sessionActor, RemoteAddr: c.remote, Now: sessionNow,
			})
			if err != nil {
				t.Fatalf("NewSession: %v", err)
			}
			if session.IPClass != c.want {
				t.Errorf("IPClass %q, want %q", session.IPClass, c.want)
			}
		})
	}
}

func TestNewSessionBoundsTheUserAgent(t *testing.T) {
	session, err := NewSession(NewSessionInput{
		ID: shared.ID("0192f000-0000-7000-8000-00000000000a"), TenantID: sessionTenant,
		AccountID: sessionActor, UserAgent: "  " + strings.Repeat("x", MaxUserAgentLength+50),
		Now: sessionNow,
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if got := len([]rune(session.UserAgent)); got != MaxUserAgentLength {
		t.Errorf("user agent kept %d runes, want %d", got, MaxUserAgentLength)
	}
	if session.ExpiresAt != sessionNow.Add(RefreshTokenLifetime) {
		t.Errorf("expiry %v, want the refresh horizon", session.ExpiresAt)
	}
}

func TestNewSessionRefusesAnIncompleteIdentity(t *testing.T) {
	_, err := NewSession(NewSessionInput{TenantID: sessionTenant, AccountID: sessionActor, Now: sessionNow})
	if !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("err %v, want the internal refusal", err)
	}
}

func TestSessionVerify(t *testing.T) {
	live := Session{
		ID:        shared.ID("0192f000-0000-7000-8000-00000000000a"),
		CreatedAt: sessionNow, ExpiresAt: sessionNow.Add(time.Hour),
	}

	if err := live.Verify(sessionNow); err != nil {
		t.Errorf("a live session refused: %v", err)
	}
	if err := live.Verify(sessionNow.Add(2 * time.Hour)); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("an expired session answered %v", err)
	}

	revoked := live.Revoked(sessionNow)
	err := revoked.Verify(sessionNow.Add(2 * time.Hour))
	if !strings.Contains(err.Error(), "auth.session_revoked") {
		// Revocation wins over expiry: the security event is the one the reader should see.
		t.Errorf("a revoked and expired session answered %v, want the revocation", err)
	}
}

func TestRevokedIsIdempotent(t *testing.T) {
	first := Session{ExpiresAt: sessionNow.Add(time.Hour)}.Revoked(sessionNow)
	second := first.Revoked(sessionNow.Add(time.Minute))
	if !second.RevokedAt.Equal(sessionNow) {
		t.Errorf("the second withdrawal moved the moment: %v", second.RevokedAt)
	}
}

func TestRotatedSlidesTheHorizon(t *testing.T) {
	session := Session{ExpiresAt: sessionNow.Add(time.Hour)}
	later := sessionNow.Add(24 * time.Hour)
	if got := session.Rotated(later).ExpiresAt; !got.Equal(later.Add(RefreshTokenLifetime)) {
		t.Errorf("horizon %v, want %v", got, later.Add(RefreshTokenLifetime))
	}
}

func TestRefreshTokenVerify(t *testing.T) {
	token := RefreshToken{ExpiresAt: sessionNow.Add(time.Hour)}
	if token.IsRotated() {
		t.Error("a fresh token counts as rotated")
	}
	if err := token.Verify(sessionNow); err != nil {
		t.Errorf("a live token refused: %v", err)
	}
	if err := token.Verify(sessionNow.Add(2 * time.Hour)); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("an expired token answered %v", err)
	}
	if err := (RefreshToken{}).Verify(sessionNow); err == nil {
		t.Error("a zero expiry verified")
	}
	if !(RefreshToken{RotatedAt: sessionNow}).IsRotated() {
		t.Error("a rotated token counts as fresh")
	}
}

func TestCheckPassword(t *testing.T) {
	if err := CheckPassword(strings.Repeat("a", MinPasswordLength)); err != nil {
		t.Errorf("the minimum refused: %v", err)
	}
	if err := CheckPassword(strings.Repeat("a", MinPasswordLength-1)); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a short password answered %v", err)
	}
	if err := CheckPassword(strings.Repeat("a", MaxPasswordLength+1)); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an oversized password answered %v", err)
	}
	// Runes, not bytes: a policy counted in bytes would shortchange anybody writing beyond ASCII.
	if err := CheckPassword(strings.Repeat("ü", MinPasswordLength)); err != nil {
		t.Errorf("a twelve-rune password refused: %v", err)
	}
}

func TestLockoutDelayCurve(t *testing.T) {
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, 0}, {1, 0}, {3, 0},
		{4, time.Second}, {5, 2 * time.Second}, {6, 4 * time.Second},
		{10, 64 * time.Second},
		{14, LockoutCeiling}, {100, LockoutCeiling}, {10_000, LockoutCeiling},
	}
	for _, c := range cases {
		if got := LockoutDelay(c.failures); got != c.want {
			t.Errorf("LockoutDelay(%d) = %v, want %v", c.failures, got, c.want)
		}
	}

	if !LockedUntil(3, sessionNow).IsZero() {
		t.Error("a free attempt produced a lock")
	}
	if got := LockedUntil(4, sessionNow); !got.Equal(sessionNow.Add(time.Second)) {
		t.Errorf("LockedUntil(4) = %v, want one second on", got)
	}
}

// T-02: the generic refusal is one answer byte for byte, however the sign-in failed.
func TestTheGenericRefusalIsOneAnswer(t *testing.T) {
	first, second := ErrSignInFailed(), ErrSignInFailed()
	if first.Error() != second.Error() {
		t.Error("two generic refusals differ")
	}
	if !errors.Is(first, shared.ErrUnauthenticated) {
		t.Error("the refusal is not an authentication refusal")
	}
}

func TestSessionNeedsTouch(t *testing.T) {
	session := Session{LastSeenAt: sessionNow}
	if session.NeedsTouch(sessionNow.Add(time.Minute), 5*time.Minute) {
		t.Error("a minute-old touch is stale")
	}
	if !session.NeedsTouch(sessionNow.Add(6*time.Minute), 5*time.Minute) {
		t.Error("a six-minute-old touch is fresh")
	}
	if !(Session{}).NeedsTouch(sessionNow, 5*time.Minute) {
		t.Error("a never-touched session needs no touch")
	}
}

// The two bounds a session answers to beyond revocation and expiry (ADR-0068 §3), each with its own
// code: "this has been open too long" and "you have not used this in a while" are different things
// to be told, and a screen that could not tell them apart would say the wrong one.
func TestASessionAnswersToItsOwnTwoBounds(t *testing.T) {
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	opened, err := NewSession(NewSessionInput{
		ID:        "018f2a1b-0000-7000-8000-00000000dd01",
		TenantID:  "018f2a1b-0000-7000-8000-00000000dd02",
		AccountID: "018f2a1b-0000-7000-8000-00000000dd03",
		Now:       at,
		Bounds:    SessionPolicy{MaxDays: 7, IdleMinutes: 30},
		Method:    SignedInWithPassword,
	})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}

	if opened.SignedInWith != SignedInWithPassword {
		t.Errorf("the session records %q", opened.SignedInWith)
	}
	if opened.HardExpiresAt.IsZero() {
		t.Fatal("no hard expiry was written")
	}
	if err := opened.Verify(at.Add(time.Hour)); err != nil {
		t.Errorf("an hour in, the session answered %v", err)
	}

	// Past the maximum age, whatever the refresh token says.
	tooOld := opened
	tooOld.ExpiresAt = at.Add(30 * 24 * time.Hour)
	if err := tooOld.Verify(at.Add(8 * 24 * time.Hour)); err == nil ||
		!strings.Contains(err.Error(), "auth.session_too_old") {
		t.Errorf("eight days in, the session answered %v", err)
	}

	// Idle past its bound. The bound counts from the last use, so a session nobody has used yet -
	// one whose last use is not recorded - is not idle.
	idle := opened
	idle.LastSeenAt = at
	if err := idle.Verify(at.Add(31 * time.Minute)); err == nil ||
		!strings.Contains(err.Error(), "auth.session_idle") {
		t.Errorf("thirty-one minutes after the last use, the session answered %v", err)
	}
	if err := opened.Verify(at.Add(31 * time.Minute)); err != nil {
		t.Errorf("a session with no recorded use was called idle: %v", err)
	}
}

// A session with no bounds is what an installation that has decided nothing opens, and it behaves
// exactly as every session behaved before the switches existed.
func TestASessionWithoutBoundsIsUnchanged(t *testing.T) {
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	opened, err := NewSession(NewSessionInput{
		ID:        "018f2a1b-0000-7000-8000-00000000dd01",
		TenantID:  "018f2a1b-0000-7000-8000-00000000dd02",
		AccountID: "018f2a1b-0000-7000-8000-00000000dd03",
		Now:       at,
	})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if !opened.HardExpiresAt.IsZero() || opened.IdleMinutes != 0 {
		t.Error("a bound was invented")
	}
	if err := opened.Verify(at.Add(29 * 24 * time.Hour)); err != nil {
		t.Errorf("twenty-nine days in, the unbounded session answered %v", err)
	}
}

// The rotation is the third comparison, and its input is the workspace's rather than the session's.
func TestASessionOpenedBeforeARotationIsOver(t *testing.T) {
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	session := Session{CreatedAt: at}

	if err := session.VerifyAgainstRotation(time.Time{}); err != nil {
		t.Errorf("a workspace that asked for nothing ended a session: %v", err)
	}
	if err := session.VerifyAgainstRotation(at.Add(-time.Hour)); err != nil {
		t.Errorf("a rotation older than the session ended it: %v", err)
	}
	err := session.VerifyAgainstRotation(at.Add(time.Hour))
	if err == nil || !strings.Contains(err.Error(), "auth.session_rotated") {
		t.Errorf("a session opened before the rotation answered %v", err)
	}
}

// ADR-0075 §2: a step-up at the provider counts only with an auth_time that is named, inside the
// step-up's window, and not from beyond the clock skew.
func TestProviderProofFresh(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	window := 5 * time.Minute
	cases := map[string]struct {
		authTime time.Time
		fresh    bool
	}{
		"not named":                  {time.Time{}, false},
		"a moment ago":               {at.Add(-10 * time.Second), true},
		"at the window's edge":       {at.Add(-window), true},
		"older than the window":      {at.Add(-window - time.Second), false},
		"ahead within the skew":      {at.Add(30 * time.Second), true},
		"ahead beyond the skew":      {at.Add(ProviderClockSkew + time.Second), false},
		"an hour ago, a stale proof": {at.Add(-time.Hour), false},
	}
	for name, tc := range cases {
		if got := ProviderProofFresh(tc.authTime, at, window); got != tc.fresh {
			t.Errorf("%s: fresh %v, want %v", name, got, tc.fresh)
		}
	}
}
