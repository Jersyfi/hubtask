// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/passkey"
)

// SG-8 for the registration: a client data and an attestation object as any client can send them.
//
// An ordinary test over its seeds in `make gate-unit`, a fuzz target in the nightly. Properties:
//
//  1. Nothing presented makes the verifier panic, and every refusal is the one validation error
//     with a reason as its cause.
//  2. What is accepted holds what was checked: the credential id is the raw id given, at most 1023
//     bytes; the kept key parses again with the algorithm reported; the user was present.
func FuzzVerifyRegistration(f *testing.F) {
	for _, section := range []string{"16.2", "16.6", "16.10", "16.11", "16.4", "16.8"} {
		v := vectorOf(section)
		f.Add(unhex(f, v.clientDataJSON), unhex(f, v.attestationObject))
	}
	f.Add([]byte(`{"type":"webauthn.create"}`), []byte{0xa0})
	f.Fuzz(func(t *testing.T, clientDataJSON, attestationObject []byte) {
		r := passkey.Registration{
			ClientDataJSON: clientDataJSON, AttestationObject: attestationObject,
			Challenge: []byte{0}, RelyingParty: exampleOrg,
		}
		// The challenge and raw id the input itself names, so a well-formed input can get past those
		// two checks and reach the rest.
		if challenge, err := (Verifier{}).ChallengeOf(clientDataJSON); err == nil {
			r.Challenge = challenge
		}
		if object, err := decodeCBORWhole(attestationObject); err == nil {
			if data, ok := object.field("authData"); ok && data.kind == cborBytes {
				if parsed, err := parseAuthenticatorData(data.bytes); err == nil {
					r.RawID = parsed.credentialID
				}
			}
		}

		credential, err := Verifier{}.VerifyRegistration(r)
		if err != nil {
			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != passkey.CodeRegistrationInvalid ||
				reasonOf(err) == "" {
				t.Fatalf("not the one refusal: %v", err)
			}
			return
		}
		if !bytes.Equal(credential.ID, r.RawID) || len(credential.ID) == 0 || len(credential.ID) > maxCredentialID {
			t.Fatalf("credential id %x against raw id %x", credential.ID, r.RawID)
		}
		key, err := ParsePublicKey(credential.PublicKey)
		if err != nil || key.Algorithm() != credential.Algorithm {
			t.Fatalf("the kept key: %v", err)
		}
	})
}
