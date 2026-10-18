// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package passkey is the seam the application layer checks a WebAuthn ceremony through
// (identity.md §23, ADR-0085). The core decides who may add a credential, binds the challenge to a
// session and keeps the result; what a ceremony's bytes say - the client data's JSON, the
// attestation object's CBOR, the COSE key - is the adapter's alone (rule 1), so the core never parses
// either and the verifier's every check sits in one package with its fuzz targets.
package passkey

// CodeRegistrationInvalid is the one refusal a malformed or mismatched ceremony answers. Which check
// failed travels as the error's cause, for the log - a person cannot act on the difference between a
// wrong RP ID hash and a truncated key, and an attacker would learn from it.
const CodeRegistrationInvalid = "auth.passkey_registration_invalid"

// CodeSignInFailed is the sign-in's one refusal (identity.md §4.3), which a refused assertion answers
// as a wrong password does.
const CodeSignInFailed = "auth.sign_in_failed"

// RelyingParty is the workspace as a ceremony must name it: the RP ID (its host) and the one origin
// a page of it is served from.
type RelyingParty struct {
	ID     string
	Origin string
}

// Registration is what a browser's `navigator.credentials.create()` answered, with what the server
// expects of it.
type Registration struct {
	ClientDataJSON    []byte
	AttestationObject []byte
	// RawID is the credential's identifier as the browser reported it; it must equal the one inside
	// the authenticator data.
	RawID      []byte
	Transports []string
	// Challenge is the one the server issued, which the client data must carry.
	Challenge    []byte
	RelyingParty RelyingParty
}

// Credential is a verified registration: what is kept of it.
type Credential struct {
	ID []byte
	// PublicKey is the COSE key exactly as the authenticator sent it.
	PublicKey []byte
	Algorithm int64
	SignCount uint32
	// UserVerified is whether this ceremony verified the person (fingerprint, face, PIN). What a
	// credential proves later is read from each later ceremony (identity.md §23.1).
	UserVerified   bool
	BackupEligible bool
	BackedUp       bool
	// Transports are the browser's hints, from the closed list, without duplicates.
	Transports []string
}

// Assertion is what a browser's `navigator.credentials.get()` answered, with the credential it names
// as the server holds it and what the server expects of the ceremony.
type Assertion struct {
	ClientDataJSON    []byte
	AuthenticatorData []byte
	Signature         []byte
	// RawID is the credential's identifier as the browser reported it.
	RawID []byte
	// Challenge is the one the server issued, which the client data must carry.
	Challenge    []byte
	RelyingParty RelyingParty
	// The credential as stored: its identifier, its COSE key and algorithm, and whether it was
	// eligible for backup when it was made - which a later ceremony may not change.
	CredentialID   []byte
	PublicKey      []byte
	Algorithm      int64
	BackupEligible bool
}

// Asserted is what a verified assertion reports. Whether user verification was required, and what
// the counter means, is the caller's to decide: one door asks for the person verified, another only
// for the key present (identity.md §23.1).
type Asserted struct {
	UserVerified bool
	BackedUp     bool
	SignCount    uint32
}

// Verifier checks ceremonies.
type Verifier interface {
	// ChallengeOf reads the challenge the client data carries, checking nothing but its shape: the
	// core needs it to find the pending registration the challenge belongs to, before anything else
	// is checked against it.
	ChallengeOf(clientDataJSON []byte) ([]byte, error)

	// VerifyRegistration checks a registration ceremony (WebAuthn Level 3 §7.1) and answers the
	// credential, or a validation error with CodeRegistrationInvalid.
	VerifyRegistration(registration Registration) (Credential, error)

	// VerifyAssertion checks an authentication ceremony (WebAuthn Level 3 §7.2) and reports its flags
	// and counter, or answers CodeSignInFailed, the reason as its
	// cause.
	VerifyAssertion(assertion Assertion) (Asserted, error)
}
