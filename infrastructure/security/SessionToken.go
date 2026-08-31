// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// sessionTokenInfo is the domain-separation label (see TokenHasher): a session access token can
// never be replayed as a media token, a cursor, or anything else derived from the same
// installation secret.
//
//nolint:gosec // G101: a public derivation label, not a credential - the secret is the installation's
const sessionTokenInfo = "hubtask/session-access/v1"

// sessionTokenTagLength truncates the tag to 128 bits, with cursorTagLength's justification.
const sessionTokenTagLength = 16

// uuidLength is the canonical 8-4-4-4-12 form the three identifiers travel in.
const uuidLength = 36

// SessionTokenClaims is what a verified access token says: which session is speaking, for which
// account, in which tenant. The values are only ever returned after the tag has been verified,
// so a caller cannot be handed a tenant nobody signed for.
type SessionTokenClaims struct {
	TenantID  shared.ID
	SessionID shared.ID
	AccountID shared.ID
	ExpiresAt time.Time
}

// SessionTokenIssuer signs the access half of the pair (H-01, security.md §5): fifteen minutes,
// verified by its signature without a database read - the same discipline every cursor and feed
// token already follows. What the signature does not answer - is the session still alive, may the
// account act - is the row's business, read afterwards by whoever holds the claims.
type SessionTokenIssuer struct {
	key []byte
}

// NewSessionTokenIssuer derives the signing key from the installation secret, under this token's
// own label.
func NewSessionTokenIssuer(installationSecret secret.Secret) SessionTokenIssuer {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(sessionTokenInfo))
	return SessionTokenIssuer{key: mac.Sum(nil)}
}

// Issue mints the token for one session, until expiresAt.
//
// Everything the claims carry travels in clear and signed, MediaTokenIssuer's reasoning: the
// bearer route has nothing else that could say which tenant to open the transaction as, and a
// value read from a header would be a way around row level security. Swapping any of it
// invalidates the tag.
func (i SessionTokenIssuer) Issue(claims SessionTokenClaims) string {
	payload := i.payload(claims)

	mac := hmac.New(sha256.New, i.key)
	mac.Write(payload)
	tag := mac.Sum(nil)[:sessionTokenTagLength]

	token := make([]byte, 0, sessionTokenTagLength+8+3*uuidLength)
	token = append(token, tag...)
	token = binary.BigEndian.AppendUint64(token, uint64(claims.ExpiresAt.Unix())) //nolint:gosec // G115: a unix timestamp fits until the year 292277026596
	token = append(token, claims.TenantID.String()...)
	token = append(token, claims.SessionID.String()...)
	token = append(token, claims.AccountID.String()...)
	return identity.SessionAccessTokenPrefix + base64.RawURLEncoding.EncodeToString(token)
}

// Validate judges a presented token. A forgery of any kind is one indistinguishable answer, for
// the reason a page cursor's is; expiry is the one distinguished refusal, because an expired
// token is one this server really minted and "refresh" is actionable where "invalid" is not.
func (i SessionTokenIssuer) Validate(presented string, now time.Time) (SessionTokenClaims, error) {
	body, found := strings.CutPrefix(presented, identity.SessionAccessTokenPrefix)
	if !found {
		return SessionTokenClaims{}, errSessionTokenInvalid()
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil || len(raw) != sessionTokenTagLength+8+3*uuidLength {
		return SessionTokenClaims{}, errSessionTokenInvalid()
	}

	expiry := int64(binary.BigEndian.Uint64(raw[sessionTokenTagLength : sessionTokenTagLength+8])) //nolint:gosec // G115: the value was written by Issue
	ids := raw[sessionTokenTagLength+8:]
	claims := SessionTokenClaims{
		TenantID:  shared.ID(ids[:uuidLength]),
		SessionID: shared.ID(ids[uuidLength : 2*uuidLength]),
		AccountID: shared.ID(ids[2*uuidLength:]),
		ExpiresAt: time.Unix(expiry, 0).UTC(),
	}

	mac := hmac.New(sha256.New, i.key)
	mac.Write(i.payload(claims))
	if !hmac.Equal(raw[:sessionTokenTagLength], mac.Sum(nil)[:sessionTokenTagLength]) {
		return SessionTokenClaims{}, errSessionTokenInvalid()
	}
	if now.Unix() > expiry {
		return SessionTokenClaims{}, shared.ErrUnauthenticated.WithDetail("access.token_expired")
	}
	return claims, nil
}

func (i SessionTokenIssuer) payload(claims SessionTokenClaims) []byte {
	return []byte(claims.TenantID.String() + "\x00" + claims.SessionID.String() + "\x00" +
		claims.AccountID.String() + "\x00" + strconv.FormatInt(claims.ExpiresAt.Unix(), 10))
}

func errSessionTokenInvalid() error {
	return shared.ErrUnauthenticated.WithDetail("access.token_malformed")
}

// The stored credentials of the sign-in flow, each hashed under its own purpose label with
// TokenHasher's construction and reasoning: HMAC-SHA-256 keyed on a pepper derived from the
// installation secret, no salt and no Argon2 because a 256-bit random secret has nothing to
// guess at - and a hash from one purpose can never be replayed as another's.
//
//nolint:gosec // G101: public derivation labels, not credentials
const (
	refreshTokenInfo    = "hubtask/session-refresh/v1"
	redemptionTokenInfo = "hubtask/invitation-redemption/v1"
	authAttemptInfo     = "hubtask/auth-attempt/v1"
)

// SessionRefreshHasher turns a presented refresh token into the value stored in
// session_refresh_token.token_hash.
type SessionRefreshHasher struct{ pepper []byte }

func NewSessionRefreshHasher(installationSecret secret.Secret) SessionRefreshHasher {
	return SessionRefreshHasher{pepper: derivePepper(installationSecret, refreshTokenInfo)}
}

func (h SessionRefreshHasher) Hash(presented string) []byte { return hashUnder(h.pepper, presented) }

// RedemptionTokenHasher turns a presented redemption token into the value stored in
// account.redemption_token_hash.
type RedemptionTokenHasher struct{ pepper []byte }

func NewRedemptionTokenHasher(installationSecret secret.Secret) RedemptionTokenHasher {
	return RedemptionTokenHasher{pepper: derivePepper(installationSecret, redemptionTokenInfo)}
}

func (h RedemptionTokenHasher) Hash(presented string) []byte { return hashUnder(h.pepper, presented) }

// AuthAttemptHasher turns a lockout subject - an address somebody signed in as, or the network
// they came from - into the value stored in auth_attempt.subject_hash. Hashed so the ledger can
// count attempts against addresses that hold no account without becoming a list of guessed
// addresses (T-02).
type AuthAttemptHasher struct{ pepper []byte }

func NewAuthAttemptHasher(installationSecret secret.Secret) AuthAttemptHasher {
	return AuthAttemptHasher{pepper: derivePepper(installationSecret, authAttemptInfo)}
}

func (h AuthAttemptHasher) Hash(subject string) []byte { return hashUnder(h.pepper, subject) }

func derivePepper(installationSecret secret.Secret, info string) []byte {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(info))
	return mac.Sum(nil)
}

func hashUnder(pepper []byte, value string) []byte {
	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(value))
	return mac.Sum(nil)
}
