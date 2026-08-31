// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The token shapes of security.md §5, as law rather than as design (0.6.0 decision 2).
const (
	// AccessTokenLifetime is how long the signed half of the pair lives. Short enough that a
	// stolen one is a fifteen-minute problem (T-01); long enough that a client is not refreshing
	// mid-thought.
	AccessTokenLifetime = 15 * time.Minute
	// RefreshTokenLifetime is how long one refresh token could be exchanged, and therefore how
	// far a session's horizon slides on each rotation. A session nobody touches for thirty days
	// runs out with the token that could have renewed it.
	RefreshTokenLifetime = 30 * 24 * time.Hour
	// RedemptionLifetime is how long an invitation's token stays redeemable. Two weeks: long
	// enough to survive a holiday, short enough that the credential is not lying around for
	// months - which is the reason the token exists only from this milestone at all
	// (data-catalog.md §7.5).
	RedemptionLifetime = 14 * 24 * time.Hour
)

// RefreshTokenPrefix and RedemptionTokenPrefix mark the two stored credentials of the sign-in
// flow, with TokenPrefix's reasoning: fixed and public so secret scanning finds a pasted one, and
// carrying the tenant because the lookup needs a context before it can happen.
const (
	RefreshTokenPrefix    = "hbt_srt_"
	RedemptionTokenPrefix = "hbt_inv_"
)

// ParseRefreshToken and ParseRedemptionToken read a presented credential of their shape. Shape
// only, ParseToken's discipline: every failure is one indistinguishable error, because the format
// is the one part an attacker does not have to guess at.
func ParseRefreshToken(raw string) (Token, error)    { return parsePrefixed(raw, RefreshTokenPrefix) }
func ParseRedemptionToken(raw string) (Token, error) { return parsePrefixed(raw, RedemptionTokenPrefix) }

// NewRefreshToken and NewRedemptionToken build a credential from freshly drawn randomness. The
// bytes come from the caller, because the domain draws nothing itself (rule 4).
func NewRefreshToken(tenantID shared.ID, secret []byte) (Token, error) {
	return newPrefixed(RefreshTokenPrefix, tenantID, secret)
}

func NewRedemptionToken(tenantID shared.ID, secret []byte) (Token, error) {
	return newPrefixed(RedemptionTokenPrefix, tenantID, secret)
}

// MaxUserAgentLength bounds what a session records about its client. A user agent is a label for
// recognising one's own devices, and anything longer than this is a client saying too much.
const MaxUserAgentLength = 400

// Session is a sign-in: the row a person sees and revokes, and the thing both tokens of the pair
// hang off (security.md §5). Ending it ends them together.
type Session struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	CreatedAt time.Time
	// LastSeenAt is when the session last acted, written back at most once per interval - the
	// retention anchor of the SESSION data kind, and what lets a person spot a session nobody
	// uses.
	LastSeenAt time.Time
	// UserAgent and IPClass are the client-binding hint T-01 asks to log: enough to recognise
	// one's own devices, deliberately not a precise address. Both are personal data - they carry
	// catalogue rows and travel in no log or trace (rule 10).
	UserAgent string
	IPClass   string
	// ExpiresAt is the horizon of the newest refresh token. Rotation slides it.
	ExpiresAt time.Time
	RevokedAt time.Time
}

// NewSessionInput is what opening a session needs.
type NewSessionInput struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	// UserAgent is recorded as the client introduced itself, bounded.
	UserAgent string
	// RemoteAddr is the peer's address as the adapter saw it. Only its class is kept: an IPv4
	// network of 24 bits, an IPv6 network of 48 - never the full address.
	RemoteAddr string
	Now        time.Time
}

// NewSession opens the row a sign-in creates. The address is coarsened here, at recording time,
// so the precise value never exists anywhere it could be stored by mistake.
func NewSession(in NewSessionInput) (Session, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.AccountID.IsZero() {
		return Session{}, shared.ErrInternal.WithDetail("auth.session_incomplete")
	}
	return Session{
		ID:        in.ID,
		TenantID:  in.TenantID,
		AccountID: in.AccountID,
		CreatedAt: in.Now.UTC(),
		UserAgent: boundedUserAgent(in.UserAgent),
		IPClass:   IPClass(in.RemoteAddr),
		ExpiresAt: in.Now.Add(RefreshTokenLifetime).UTC(),
	}, nil
}

// Verify decides whether the session may still answer for its account at this moment.
//
// Revocation before expiry, AccessToken.Verify's order: a revoked session is a security event and
// an expired one is routine, and whoever reads the log should see the first of those.
func (s Session) Verify(now time.Time) error {
	if !s.RevokedAt.IsZero() && !now.Before(s.RevokedAt) {
		return shared.ErrUnauthenticated.WithDetail("auth.session_revoked")
	}
	if s.ExpiresAt.IsZero() || !now.Before(s.ExpiresAt) {
		return shared.ErrUnauthenticated.WithDetail("auth.session_expired")
	}
	return nil
}

// Revoked stamps the session. Idempotent in the caller's sense: the first withdrawal is the one
// that mattered.
func (s Session) Revoked(at time.Time) Session {
	if !s.RevokedAt.IsZero() {
		return s
	}
	s.RevokedAt = at.UTC()
	return s
}

// Rotated slides the horizon to the newest refresh token's.
func (s Session) Rotated(now time.Time) Session {
	s.ExpiresAt = now.Add(RefreshTokenLifetime).UTC()
	return s
}

// NeedsTouch reports whether the last use is stale enough to be worth a write, with
// AccessToken.NeedsTouch's reasoning: minutes of resolution are plenty for "is anybody still
// using this?".
func (s Session) NeedsTouch(now time.Time, interval time.Duration) bool {
	return s.LastSeenAt.IsZero() || now.Sub(s.LastSeenAt) >= interval
}

// RefreshToken is the stored half of one link in a session's refresh chain. The secret is not
// here and never was: only its hash is stored, and the hash never leaves the persistence adapter
// (security.md §8).
type RefreshToken struct {
	ID        shared.ID
	TenantID  shared.ID
	SessionID shared.ID
	CreatedAt time.Time
	ExpiresAt time.Time
	// RotatedAt is set by the exchange that retired it. A retired token presented again means
	// two holders - the reuse T-01 exists to detect - which is why the row outlives its use.
	RotatedAt time.Time
}

// IsRotated reports whether this token was already exchanged. The caller asks before Verify,
// because reuse is not a refusal like the others: it kills the whole family and raises the
// alarm, and a caller that cannot tell it apart cannot do either.
func (t RefreshToken) IsRotated() bool { return !t.RotatedAt.IsZero() }

// Verify decides whether the token may be exchanged at this moment. Rotation is IsRotated's
// question and deliberately not repeated here.
func (t RefreshToken) Verify(now time.Time) error {
	if t.ExpiresAt.IsZero() || !now.Before(t.ExpiresAt) {
		return shared.ErrUnauthenticated.WithDetail("auth.refresh_token_expired")
	}
	return nil
}

// The password policy of security.md §5. Twelve at least; the ceiling exists so a paste of a
// whole document fails as a validation error rather than as a hashing bill.
const (
	MinPasswordLength = 12
	MaxPasswordLength = 1024
)

// CheckPassword refuses what the policy forbids. Only where a password is *set*: the sign-in
// check compares whatever was presented, because refusing a short guess differently from a wrong
// one would leak which it was.
func CheckPassword(password string) error {
	switch {
	case utf8.RuneCountInString(password) < MinPasswordLength:
		return shared.ErrValidation.
			WithDetail("auth.password_too_short").
			WithParams(map[string]string{"minimum": itoa(MinPasswordLength)}).
			WithFields(shared.FieldError{Path: "/password", Code: "auth.password_too_short"})
	case utf8.RuneCountInString(password) > MaxPasswordLength:
		return shared.ErrValidation.
			WithDetail("auth.password_too_long").
			WithParams(map[string]string{"maximum": itoa(MaxPasswordLength)}).
			WithFields(shared.FieldError{Path: "/password", Code: "auth.password_too_long"})
	}
	return nil
}

// The lockout curve of T-02: free attempts first, then a delay that doubles per failure up to a
// ceiling. The delay *is* the lockout - a curve that reaches fifteen minutes needs no second
// mechanism, and one mechanism is one to test.
const (
	// lockoutFreeAttempts are refused without delay. Everybody mistypes.
	lockoutFreeAttempts = 3
	// lockoutBaseDelay is the first delay, doubled per further failure.
	lockoutBaseDelay = time.Second
	// LockoutCeiling caps the delay. Fifteen minutes holds an online guesser to four attempts an
	// hour without locking the real owner out of their morning.
	LockoutCeiling = 15 * time.Minute
)

// LockoutDelay is how long the subject waits after that many failures.
func LockoutDelay(failures int) time.Duration {
	if failures <= lockoutFreeAttempts {
		return 0
	}
	exceeded := failures - lockoutFreeAttempts - 1
	if exceeded >= 10 {
		// 2^10 seconds is past the ceiling; shifting further would eventually overflow.
		return LockoutCeiling
	}
	delay := lockoutBaseDelay << exceeded
	if delay > LockoutCeiling {
		return LockoutCeiling
	}
	return delay
}

// LockedUntil is the moment the subject may try again.
func LockedUntil(failures int, lastFailure time.Time) time.Time {
	delay := LockoutDelay(failures)
	if delay == 0 {
		return time.Time{}
	}
	return lastFailure.Add(delay).UTC()
}

// ErrSignInFailed is the one generic refusal of T-02, built in exactly one place so that "wrong
// password" and "no such account" are the same answer byte for byte - whether an account exists
// is exactly what a guessing client is trying to learn.
func ErrSignInFailed() error {
	return shared.ErrUnauthenticated.WithDetail("auth.sign_in_failed")
}

// IPClass coarsens an address at recording time: an IPv4 /24, an IPv6 /48, and nothing for an
// address that does not parse. The precise value never exists past this call (T-01, rule 10).
func IPClass(remoteAddr string) string {
	raw := strings.TrimSpace(remoteAddr)
	if raw == "" {
		return ""
	}
	// The adapter may hand host:port, as net/http's RemoteAddr does.
	if addrPort, err := netip.ParseAddrPort(raw); err == nil {
		raw = addrPort.Addr().String()
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return ""
	}
	addr = addr.Unmap()
	bits := 48
	if addr.Is4() {
		bits = 24
	}
	prefix, err := addr.Prefix(bits)
	if err != nil {
		return ""
	}
	return prefix.String()
}

func boundedUserAgent(raw string) string {
	agent := strings.TrimSpace(raw)
	if utf8.RuneCountInString(agent) <= MaxUserAgentLength {
		return agent
	}
	runes := []rune(agent)
	return string(runes[:MaxUserAgentLength])
}

func parsePrefixed(raw, prefix string) (Token, error) {
	body, found := strings.CutPrefix(raw, prefix)
	if !found {
		return Token{}, errTokenMalformed()
	}
	tenantHex, secret, found := strings.Cut(body, "_")
	if !found || len(tenantHex) != tenantHexLength || len(secret) != tokenSecretLength ||
		!isBase64URL(secret) {
		return Token{}, errTokenMalformed()
	}
	tenantID, err := tenantFromHex(tenantHex)
	if err != nil {
		return Token{}, errTokenMalformed()
	}
	return Token{tenantID: tenantID, raw: raw}, nil
}

func newPrefixed(prefix string, tenantID shared.ID, secret []byte) (Token, error) {
	token, err := NewToken(tenantID, secret)
	if err != nil {
		return Token{}, err
	}
	return Token{
		tenantID: tenantID,
		raw:      prefix + strings.TrimPrefix(token.raw, TokenPrefix),
	}, nil
}
