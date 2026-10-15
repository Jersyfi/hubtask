// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/passkey"
)

// The bounds of a registration's inputs. The client data is a short JSON object; a browser's is
// around 200 bytes, and 4 KiB leaves room for any field a later level adds.
const (
	maxClientDataJSON = 4 << 10
	maxFormatLength   = 32
	maxCredentialID   = 1023
	maxTransports     = 6
)

// The authenticator data's flags (WebAuthn Level 3 §6.1). Bits 1 and 5 are reserved and ignored.
const (
	flagUP = 0x01
	flagUV = 0x04
	flagBE = 0x08
	flagBS = 0x10
	flagAT = 0x40
	flagED = 0x80
)

// The fixed head of the authenticator data: the RP ID hash, the flags, the counter.
const (
	rpIDHashLength   = 32
	authDataHead     = rpIDHashLength + 1 + 4
	aaguidLength     = 16
	credentialIDHead = 2
)

// transports is the closed list of hints a browser reports (WebAuthn Level 3 §5.8.4).
var transports = []string{"usb", "nfc", "ble", "hybrid", "internal", "smart-card"}

// Verifier is the adapter behind passkey.Verifier. It holds nothing: every expectation comes with
// the call, because the relying party is a workspace's and the adapter is the installation's.
type Verifier struct{}

var _ passkey.Verifier = Verifier{}

// clientData is the part of the client data a relying party reads (WebAuthn Level 3 §5.8.1).
type clientData struct {
	Type         string        `json:"type"`
	Challenge    string        `json:"challenge"`
	Origin       string        `json:"origin"`
	CrossOrigin  *bool         `json:"crossOrigin"`
	TopOrigin    *string       `json:"topOrigin"`
	TokenBinding *tokenBinding `json:"tokenBinding"`
}

type tokenBinding struct {
	Status string `json:"status"`
}

// ChallengeOf reads the client data's challenge.
func (Verifier) ChallengeOf(clientDataJSON []byte) ([]byte, error) {
	data, err := parseClientData(clientDataJSON)
	if err != nil {
		return nil, invalid(err)
	}
	challenge, err := base64.RawURLEncoding.DecodeString(data.Challenge)
	if err != nil || len(challenge) == 0 {
		return nil, invalid(refuse("client_data.challenge"))
	}
	return challenge, nil
}

// VerifyRegistration checks a registration ceremony in the order of WebAuthn Level 3 §7.1.
func (Verifier) VerifyRegistration(r passkey.Registration) (passkey.Credential, error) {
	credential, err := verifyRegistration(r)
	if err != nil {
		return passkey.Credential{}, invalid(err)
	}
	return credential, nil
}

func invalid(err error) error {
	return shared.ErrValidation.WithDetail(passkey.CodeRegistrationInvalid).WithCause(err)
}

func parseClientData(raw []byte) (clientData, error) {
	if len(raw) == 0 || len(raw) > maxClientDataJSON {
		return clientData{}, refuse("client_data.size")
	}
	var data clientData
	if err := json.Unmarshal(raw, &data); err != nil {
		return clientData{}, refuse("client_data.json")
	}
	return data, nil
}

func verifyRegistration(r passkey.Registration) (passkey.Credential, error) {
	if r.RelyingParty.ID == "" || r.RelyingParty.Origin == "" || len(r.Challenge) == 0 {
		return passkey.Credential{}, refuse("relying_party.unset")
	}

	// Steps 5 to 11: the client data, read from the bytes the browser sent and hashed as they are -
	// a re-serialisation would hash something the authenticator never saw.
	data, err := parseClientData(r.ClientDataJSON)
	if err != nil {
		return passkey.Credential{}, err
	}
	if data.Type != "webauthn.create" {
		return passkey.Credential{}, refuse("client_data.type")
	}
	challenge, err := base64.RawURLEncoding.DecodeString(data.Challenge)
	if err != nil || subtle.ConstantTimeCompare(challenge, r.Challenge) != 1 {
		return passkey.Credential{}, refuse("client_data.challenge")
	}
	if data.Origin != r.RelyingParty.Origin {
		return passkey.Credential{}, refuse("client_data.origin")
	}
	// A page of this workspace inside another site's frame is not this workspace asking.
	if (data.CrossOrigin != nil && *data.CrossOrigin) || data.TopOrigin != nil {
		return passkey.Credential{}, refuse("client_data.cross_origin")
	}
	if data.TokenBinding != nil && data.TokenBinding.Status == "present" {
		return passkey.Credential{}, refuse("client_data.token_binding")
	}

	// Step 12: the attestation object, exactly its three members.
	object, err := decodeCBORWhole(r.AttestationObject)
	if err != nil {
		return passkey.Credential{}, err
	}
	if object.kind != cborMap || len(object.pairs) != 3 {
		return passkey.Credential{}, refuse("attestation.shape")
	}
	format, okFmt := object.field("fmt")
	statement, okStmt := object.field("attStmt")
	authData, okData := object.field("authData")
	if !okFmt || !okStmt || !okData || format.kind != cborText || statement.kind != cborMap ||
		authData.kind != cborBytes {
		return passkey.Credential{}, refuse("attestation.shape")
	}
	if len(format.bytes) == 0 || len(format.bytes) > maxFormatLength {
		return passkey.Credential{}, refuse("attestation.format")
	}

	// Steps 13 to 18: the authenticator data.
	parsed, err := parseAuthenticatorData(authData.bytes)
	if err != nil {
		return passkey.Credential{}, err
	}
	rpIDHash := sha256.Sum256([]byte(r.RelyingParty.ID))
	if subtle.ConstantTimeCompare(parsed.rpIDHash, rpIDHash[:]) != 1 {
		return passkey.Credential{}, refuse("auth_data.rp_id")
	}
	if parsed.flags&flagUP == 0 {
		return passkey.Credential{}, refuse("auth_data.user_presence")
	}
	if parsed.flags&flagBS != 0 && parsed.flags&flagBE == 0 {
		return passkey.Credential{}, refuse("auth_data.backup_state")
	}

	// Step 19: the statement. `none` carries an empty one. Any other format is accepted with its
	// statement unread - a deliberate departure from step 19: a statement proves only that some
	// maker's key signed, and Hubtask trusts no maker's key (identity.md §23.3, ADR-0085 §1).
	if string(format.bytes) == "none" && len(statement.pairs) != 0 {
		return passkey.Credential{}, refuse("attestation.none_statement")
	}

	// Step 22's bound, and the browser's word for the identifier held to the authenticator's.
	if !bytes.Equal(parsed.credentialID, r.RawID) {
		return passkey.Credential{}, refuse("credential.raw_id")
	}

	hints, err := transportsOf(r.Transports)
	if err != nil {
		return passkey.Credential{}, err
	}

	return passkey.Credential{
		ID:             append([]byte(nil), parsed.credentialID...),
		PublicKey:      append([]byte(nil), parsed.publicKey...),
		Algorithm:      parsed.key.Algorithm(),
		SignCount:      parsed.signCount,
		UserVerified:   parsed.flags&flagUV != 0,
		BackupEligible: parsed.flags&flagBE != 0,
		BackedUp:       parsed.flags&flagBS != 0,
		Transports:     hints,
	}, nil
}

// authenticatorData is a registration's authenticator data, read.
type authenticatorData struct {
	rpIDHash     []byte
	flags        byte
	signCount    uint32
	credentialID []byte
	publicKey    []byte
	key          PublicKey
}

// parseAuthenticatorData reads the authenticator data of a registration: the head, the attested
// credential data, and the extensions exactly when ED says they are there - and nothing after them.
// The AAGUID is read past and kept nowhere: Hubtask names no maker (UC-ID-22).
func parseAuthenticatorData(data []byte) (authenticatorData, error) {
	if len(data) < authDataHead {
		return authenticatorData{}, refuse("auth_data.truncated")
	}
	parsed := authenticatorData{
		rpIDHash:  data[:rpIDHashLength],
		flags:     data[rpIDHashLength],
		signCount: binary.BigEndian.Uint32(data[rpIDHashLength+1 : authDataHead]),
	}
	rest := data[authDataHead:]

	if parsed.flags&flagAT == 0 {
		return authenticatorData{}, refuse("auth_data.no_credential")
	}
	if len(rest) < aaguidLength+credentialIDHead {
		return authenticatorData{}, refuse("auth_data.truncated")
	}
	rest = rest[aaguidLength:]
	idLength := int(binary.BigEndian.Uint16(rest[:credentialIDHead]))
	rest = rest[credentialIDHead:]
	if idLength == 0 || idLength > maxCredentialID {
		return authenticatorData{}, refuse("credential.id_length")
	}
	if len(rest) < idLength {
		return authenticatorData{}, refuse("auth_data.truncated")
	}
	parsed.credentialID, rest = rest[:idLength], rest[idLength:]

	keyValue, afterKey, err := decodeCBOR(rest)
	if err != nil {
		return authenticatorData{}, err
	}
	parsed.publicKey = rest[:len(rest)-len(afterKey)]
	if parsed.key, err = publicKeyOf(keyValue); err != nil {
		return authenticatorData{}, err
	}
	rest = afterKey

	if parsed.flags&flagED != 0 {
		extensions, afterExtensions, err := decodeCBOR(rest)
		if err != nil {
			return authenticatorData{}, err
		}
		if extensions.kind != cborMap {
			return authenticatorData{}, refuse("auth_data.extensions")
		}
		rest = afterExtensions
	}
	if len(rest) != 0 {
		return authenticatorData{}, refuse("auth_data.trailing_bytes")
	}
	return parsed, nil
}

// transportsOf keeps the hints from the closed list, once each.
func transportsOf(given []string) ([]string, error) {
	if len(given) > maxTransports {
		return nil, refuse("credential.transports")
	}
	kept := make([]string, 0, len(given))
	for _, transport := range given {
		if !slices.Contains(transports, transport) {
			return nil, refuse("credential.transports")
		}
		if !slices.Contains(kept, transport) {
			kept = append(kept, transport)
		}
	}
	return kept, nil
}
