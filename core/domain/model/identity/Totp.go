// SPDX-License-Identifier: Apache-2.0
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

// RFC 6238, dependency-free (security.md §11, identity.md §8): the whole of TOTP is crypto/hmac,
// crypto/sha1 and crypto/subtle, and a library would be a supply chain decision for thirty lines of
// arithmetic. The parameters are the RFC's defaults, because every authenticator app ships them: an
// installation that deviated would be one whose QR codes quietly produce wrong codes in half the
// apps people actually use.
const (
	// TotpSecretBytes is 160 bits, RFC 4226 §4's requirement for HMAC-SHA-1.
	TotpSecretBytes = 20
	// TotpDigits and TotpStepSeconds are the defaults every authenticator assumes.
	TotpDigits      = 6
	TotpStepSeconds = 30
	// TotpDrift is how many steps either side of now a code may verify: one (identity.md §8) - a
	// phone whose clock is half a minute out still signs in, and a code is never good for more than
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
// candidate, and never at or below the last accepted step - the same code verifying twice is a code
// somebody shoulder-read (identity.md §8).
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
// job). The otpauth scheme is the de-facto contract every authenticator reads; the secret travels
// base32 without padding, as they expect it.
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

// MfaReplacementLifetime is how long a replacement of the authenticator waits to be confirmed
// (identity.md §8): long enough to install an app and scan a code, short enough that a replacement
// somebody walked away from does not lie around sealed beside the factor in force.
const MfaReplacementLifetime = 10 * time.Minute

// TotpSecretBase32 is the secret as a person types it where no camera reaches the QR.
func TotpSecretBase32(secret []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

// The recovery codes (identity.md §9): ten, single-use, shown once. Eighty bits each - far past
// guessable behind the attempt ledger, short enough to read to a phone's support hotline digit by
// digit.
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

// The pending credential of a two-step sign-in (identity.md §4.2): the password answered it, and it
// can do nothing but complete the sign-in it belongs to.
const (
	// PendingTokenPrefix marks it, with the session tokens' reasoning.
	PendingTokenPrefix = "hbt_mfa_" //nolint:gosec // G101: a public format marker, not a credential
	// PendingLifetime is how long the second step may take. Minutes: long enough to find the
	// phone, short enough that an abandoned half sign-in is not a standing door.
	PendingLifetime = 5 * time.Minute
	// ResetLifetime is how long a reset link stays usable (ADR-0068 §6). Half an hour: long
	// enough to reach a mailbox on another device, short enough that a link left in an inbox is
	// not a standing door. The same pending row, a longer clock - the discipline is the second
	// factor's, the window is the mailbox's.
	ResetLifetime = 30 * time.Minute
)

// PendingPurpose is what the credential may complete.
type PendingPurpose string

const (
	// PendingTotp presents a code or a recovery code.
	PendingTotp PendingPurpose = "TOTP"
	// PendingEnroll is the enforcement route: an administrator the tenant switch requires a
	// factor of, not yet enrolled, allowed exactly as far as enrolment and its confirmation.
	PendingEnroll PendingPurpose = "ENROLL"
	// PendingReset is the token a reset mail carries (ADR-0068 §6). Not a session and not a
	// second factor: thirty minutes, single use, and it can do one thing - set a password.
	PendingReset PendingPurpose = "RESET"
	// PendingPassword is the credential the PASSWORD_CHANGE step of a sign-in hands out: the
	// password was right and no longer meets the rule, so the sign-in continues by setting a new
	// one. Confirming it *is* the sign-in, exactly as ENROLL already works (ADR-0068 §3).
	PendingPassword PendingPurpose = "PASSWORD"
	// PendingLink is the credential a provider arrival receives when the account it would connect
	// to already holds a credential of its own (ADR-0071's addendum, E2). It completes with that
	// account's password, and - where the account has a second factor - continues into the TOTP
	// step with the link still carried. The provider's word alone never opens such an account.
	PendingLink PendingPurpose = "LINK"
	// PendingConnect is the link a workspace that switched the password off mails instead of a
	// reset link (ADR-0078 §1): it connects the workspace's provider to the account, together with
	// a fresh sign-in there. The reset link's discipline - thirty minutes, single use, the newest
	// one wins - and it does one thing only: stand in for the password at the provider's first
	// arrival. It is never a session and never replaces the second factor.
	PendingConnect PendingPurpose = "CONNECT"
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
	// Link is the provider identity this sign-in connects once it completes, or nil. Carried by a
	// LINK credential, and by the TOTP credential a LINK hands on to when the account has a second
	// factor - so the connection happens at the end of the account's own proof, never before it.
	Link *LinkIntent
}

// LinkIntent is a provider identity waiting for the account's own proof: which provider, and the
// subject it vouched for.
type LinkIntent struct {
	ProviderID shared.ID
	Subject    string
	// Proof is what proved the account before the connection: the password, or the mailbox with a
	// fresh sign-in at the provider. The trail entry says which (ADR-0078 §1). Empty reads as the
	// password - a credential written before there was a second kind of proof.
	Proof LinkProof
}

// LinkProof names the proof an existing account gave before a provider identity was connected to it.
type LinkProof string

const (
	// LinkProofPassword is the LINK step: the account's own password (ADR-0071's addendum, E2).
	LinkProofPassword LinkProof = "PASSWORD"
	// LinkProofMailbox is the link mailed to the account's address together with a fresh sign-in at
	// the provider, where the workspace switched the password off (ADR-0078 §1).
	LinkProofMailbox LinkProof = "MAILBOX"
)

// Verify decides whether the credential may still complete its sign-in. One indistinguishable
// refusal for consumed and expired: which of the two ended a stolen token is not for its thief
// to learn.
func (p PendingCredential) Verify(now time.Time) error {
	if !p.ConsumedAt.IsZero() || p.ExpiresAt.IsZero() || !now.Before(p.ExpiresAt) {
		return shared.ErrUnauthenticated.WithDetail("auth.mfa_challenge_failed")
	}
	return nil
}
