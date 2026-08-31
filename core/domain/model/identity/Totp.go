// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // G505: RFC 6238's algorithm; every authenticator app expects it, and HMAC-SHA-1 is not collision-bound
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// RFC 6238, dependency-free (0.6.0 decision 4): the whole of TOTP is crypto/hmac, crypto/sha1
// and crypto/subtle, and a library would be a supply chain decision for thirty lines of
// arithmetic. The parameters are the RFC's defaults, because every authenticator app ships them:
// an installation that deviated would be one whose QR codes quietly produce wrong codes in half
// the apps people actually use.
const (
	// TotpSecretBytes is 160 bits, RFC 4226 §4's requirement for HMAC-SHA-1.
	TotpSecretBytes = 20
	// TotpDigits and TotpStepSeconds are the defaults every authenticator assumes.
	TotpDigits      = 6
	TotpStepSeconds = 30
	// TotpDrift is how many steps either side of now a code may verify: one, per H-02 - a phone
	// whose clock is half a minute out still signs in, and a code is never good for more than
	// ninety seconds end to end.
	TotpDrift = 1
)

// TotpStep is the RFC's T: which thirty-second window a moment falls in.
func TotpStep(at time.Time) int64 { return at.Unix() / TotpStepSeconds }

// TotpCode computes the code for one step - HOTP (RFC 4226 §5) over the step counter, which is
// all TOTP is. Exported for the enrolment confirmation and the tests; production callers verify
// rather than compute.
func TotpCode(secret []byte, step int64) string {
	mac := hmac.New(sha1.New, secret)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step)) //nolint:gosec // G115: steps are positive for the next few thousand years
	mac.Write(counter[:])
	digest := mac.Sum(nil)

	// Dynamic truncation, RFC 4226 §5.3: the low nibble of the last byte picks a four-byte
	// window, whose 31 bits become the code.
	offset := digest[len(digest)-1] & 0x0F
	value := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7FFFFFFF

	code := strconv.FormatUint(uint64(value%1_000_000), 10)
	return strings.Repeat("0", TotpDigits-len(code)) + code
}

// VerifyTotp judges a presented code: within one step of drift either side, in constant time per
// candidate, and never at or below the last accepted step - the same code verifying twice is a
// code somebody shoulder-read (H-02).
//
// The accepted step is returned so the caller can record it; the boolean is the answer. Every
// candidate window is checked even after a match, so a wrong code and a right one cost the same
// work in the same order.
func VerifyTotp(secret []byte, presented string, now time.Time, lastStep int64) (int64, bool) {
	presented = strings.TrimSpace(presented)
	current := TotpStep(now)

	var acceptedStep int64
	accepted := false
	for delta := int64(-TotpDrift); delta <= TotpDrift; delta++ {
		step := current + delta
		match := subtle.ConstantTimeCompare([]byte(TotpCode(secret, step)), []byte(presented)) == 1
		if match && step > lastStep && !accepted {
			acceptedStep, accepted = step, true
		}
	}
	return acceptedStep, accepted
}

// TotpProvisioningURI is what a client renders the QR image from (the rendering is the client's
// job, H-02). The otpauth scheme is the de-facto contract every authenticator reads; the secret
// travels base32 without padding, as they expect it.
func TotpProvisioningURI(issuer, account string, secret []byte) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	query := url.Values{}
	query.Set("secret", TotpSecretBase32(secret))
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", strconv.Itoa(TotpDigits))
	query.Set("period", strconv.Itoa(TotpStepSeconds))
	return "otpauth://totp/" + label + "?" + query.Encode()
}

// TotpSecretBase32 is the secret as a person types it where no camera reaches the QR.
func TotpSecretBase32(secret []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

// The recovery codes (H-02): ten, single-use, shown once. Eighty bits each - far past guessable
// behind the attempt ledger, short enough to read to a phone's support hotline digit by digit.
const (
	RecoveryCodeCount = 10
	RecoveryCodeBytes = 10
)

// NewRecoveryCodes formats drawn material into the ten codes. The material comes from the
// caller, because the domain draws nothing itself (rule 4).
func NewRecoveryCodes(material []byte) ([]string, error) {
	if len(material) != RecoveryCodeCount*RecoveryCodeBytes {
		return nil, shared.ErrInternal.WithDetail("auth.session_unmintable")
	}
	codes := make([]string, 0, RecoveryCodeCount)
	for i := range RecoveryCodeCount {
		chunk := material[i*RecoveryCodeBytes : (i+1)*RecoveryCodeBytes]
		raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(chunk)
		// Grouped for reading aloud; the normalisation strips the grouping back out.
		codes = append(codes, raw[:4]+"-"+raw[4:8]+"-"+raw[8:12]+"-"+raw[12:16])
	}
	return codes, nil
}

// NormalizeRecoveryCode is what a presented code becomes before it is hashed: the grouping,
// case and spacing people introduce reading a code off paper must not make a right code wrong.
func NormalizeRecoveryCode(raw string) string {
	cleaned := strings.ToUpper(strings.TrimSpace(raw))
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	return strings.ReplaceAll(cleaned, " ", "")
}

// The pending credential of a two-step sign-in (H-02): the password answered it, and it can do
// nothing but complete the sign-in it belongs to.
const (
	// PendingTokenPrefix marks it, with the session tokens' reasoning.
	PendingTokenPrefix = "hbt_mfa_" //nolint:gosec // G101: a public format marker, not a credential
	// PendingLifetime is how long the second step may take. Minutes: long enough to find the
	// phone, short enough that an abandoned half sign-in is not a standing door.
	PendingLifetime = 5 * time.Minute
)

// PendingPurpose is what the credential may complete.
type PendingPurpose string

const (
	// PendingTotp presents a code or a recovery code.
	PendingTotp PendingPurpose = "TOTP"
	// PendingEnroll is the enforcement route: an administrator the tenant switch requires a
	// factor of, not yet enrolled, allowed exactly as far as enrolment and its confirmation.
	PendingEnroll PendingPurpose = "ENROLL"
)

// ParsePendingToken and NewPendingToken are the credential's shape, ParseToken's discipline.
func ParsePendingToken(raw string) (Token, error) { return parsePrefixed(raw, PendingTokenPrefix) }

func NewPendingToken(tenantID shared.ID, secret []byte) (Token, error) {
	return newPrefixed(PendingTokenPrefix, tenantID, secret)
}

// PendingCredential is the stored half of the row.
type PendingCredential struct {
	ID         shared.ID
	TenantID   shared.ID
	AccountID  shared.ID
	Purpose    PendingPurpose
	UserAgent  string
	IPClass    string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt time.Time
}

// Verify decides whether the credential may still complete its sign-in. One indistinguishable
// refusal for consumed and expired: which of the two ended a stolen token is not for its thief
// to learn.
func (p PendingCredential) Verify(now time.Time) error {
	if !p.ConsumedAt.IsZero() || p.ExpiresAt.IsZero() || !now.Before(p.ExpiresAt) {
		return shared.ErrUnauthenticated.WithDetail("auth.mfa_challenge_failed")
	}
	return nil
}
