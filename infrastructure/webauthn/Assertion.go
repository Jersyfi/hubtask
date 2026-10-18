// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/passkey"
)

// The bounds of an assertion's binary parts. A signature is at most an RSA-4096 one (512 bytes) or
// an ECDSA signature in DER; the authenticator data of an assertion holds no key, only the head and
// the extensions.
const (
	maxAuthenticatorData = 64 << 10
	maxSignature         = 1024
)

// VerifyAssertion checks an authentication ceremony in the order of WebAuthn Level 3 §7.2.
func (Verifier) VerifyAssertion(a passkey.Assertion) (passkey.Asserted, error) {
	asserted, err := verifyAssertion(a)
	if err != nil {
		// The sign-in's one answer: which check failed is for the log, never for whoever presented
		// the ceremony (identity.md §4.3).
		return passkey.Asserted{}, shared.ErrUnauthenticated.WithDetail(passkey.CodeSignInFailed).WithCause(err)
	}
	return asserted, nil
}

func verifyAssertion(a passkey.Assertion) (passkey.Asserted, error) {
	if a.RelyingParty.ID == "" || a.RelyingParty.Origin == "" || len(a.Challenge) == 0 ||
		len(a.PublicKey) == 0 {
		return passkey.Asserted{}, refuse("relying_party.unset")
	}
	// Steps 5 and 6: the credential is the one the browser named, and the server holds it.
	if len(a.RawID) == 0 || !bytes.Equal(a.RawID, a.CredentialID) {
		return passkey.Asserted{}, refuse("credential.raw_id")
	}

	// Steps 10 to 17: the client data, from the bytes the browser sent.
	data, err := parseClientData(a.ClientDataJSON)
	if err != nil {
		return passkey.Asserted{}, err
	}
	if data.Type != "webauthn.get" {
		return passkey.Asserted{}, refuse("client_data.type")
	}
	challenge, err := base64.RawURLEncoding.DecodeString(data.Challenge)
	if err != nil || subtle.ConstantTimeCompare(challenge, a.Challenge) != 1 {
		return passkey.Asserted{}, refuse("client_data.challenge")
	}
	if data.Origin != a.RelyingParty.Origin {
		return passkey.Asserted{}, refuse("client_data.origin")
	}
	if (data.CrossOrigin != nil && *data.CrossOrigin) || data.TopOrigin != nil {
		return passkey.Asserted{}, refuse("client_data.cross_origin")
	}
	if data.TokenBinding != nil && data.TokenBinding.Status == "present" {
		return passkey.Asserted{}, refuse("client_data.token_binding")
	}

	// Steps 18 to 21: the authenticator data.
	head, err := parseAssertionData(a.AuthenticatorData)
	if err != nil {
		return passkey.Asserted{}, err
	}
	rpIDHash := sha256.Sum256([]byte(a.RelyingParty.ID))
	if subtle.ConstantTimeCompare(head.rpIDHash, rpIDHash[:]) != 1 {
		return passkey.Asserted{}, refuse("auth_data.rp_id")
	}
	if head.flags&flagUP == 0 {
		return passkey.Asserted{}, refuse("auth_data.user_presence")
	}
	if head.flags&flagBS != 0 && head.flags&flagBE == 0 {
		return passkey.Asserted{}, refuse("auth_data.backup_state")
	}
	// Level 3 §7.2 step 20: eligibility for backup is fixed when the credential is made.
	if (head.flags&flagBE != 0) != a.BackupEligible {
		return passkey.Asserted{}, refuse("auth_data.backup_eligibility")
	}

	// Steps 22 and 23: the signature over the authenticator data and the client data's hash, with
	// the stored key - whose algorithm must still be the one it was stored with.
	if len(a.Signature) == 0 || len(a.Signature) > maxSignature {
		return passkey.Asserted{}, refuse("signature.size")
	}
	key, err := ParsePublicKey(a.PublicKey)
	if err != nil {
		return passkey.Asserted{}, err
	}
	if key.Algorithm() != a.Algorithm {
		return passkey.Asserted{}, refuse("signature.algorithm")
	}
	clientHash := sha256.Sum256(a.ClientDataJSON)
	signed := make([]byte, 0, len(a.AuthenticatorData)+len(clientHash))
	signed = append(append(signed, a.AuthenticatorData...), clientHash[:]...)
	if !key.Verify(signed, a.Signature) {
		return passkey.Asserted{}, refuse("signature.invalid")
	}

	return passkey.Asserted{
		UserVerified: head.flags&flagUV != 0,
		BackedUp:     head.flags&flagBS != 0,
		SignCount:    head.signCount,
	}, nil
}

// assertionData is the authenticator data of an assertion, read.
type assertionData struct {
	rpIDHash  []byte
	flags     byte
	signCount uint32
}

// parseAssertionData reads an assertion's authenticator data: the head, and the extensions exactly
// when ED says they are there. An assertion carries no attested credential, so AT is refused, and
// nothing may follow.
func parseAssertionData(data []byte) (assertionData, error) {
	if len(data) > maxAuthenticatorData {
		return assertionData{}, refuse("auth_data.size")
	}
	if len(data) < authDataHead {
		return assertionData{}, refuse("auth_data.truncated")
	}
	head := assertionData{
		rpIDHash:  data[:rpIDHashLength],
		flags:     data[rpIDHashLength],
		signCount: binary.BigEndian.Uint32(data[rpIDHashLength+1 : authDataHead]),
	}
	if head.flags&flagAT != 0 {
		return assertionData{}, refuse("auth_data.attested_in_assertion")
	}
	rest := data[authDataHead:]
	if head.flags&flagED != 0 {
		extensions, after, err := decodeCBOR(rest)
		if err != nil {
			return assertionData{}, err
		}
		if extensions.kind != cborMap {
			return assertionData{}, refuse("auth_data.extensions")
		}
		rest = after
	}
	if len(rest) != 0 {
		return assertionData{}, refuse("auth_data.trailing_bytes")
	}
	return head, nil
}
