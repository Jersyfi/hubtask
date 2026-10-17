// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/passkey"
)

// SG-8 for the assertion: a client data, authenticator data and signature as any client can send
// them, against a credential the server holds.
//
// An ordinary test over its seeds in `make gate-unit`, a fuzz target in the nightly. Properties:
//
//  1. Nothing presented makes the verifier panic, and every refusal is the sign-in's one answer with
//     a reason as its cause.
//  2. What verifies carried the vector's own signature: no input verifies that the fuzzer made up
//     without the private key, so an accepted input is one whose bytes equal a seed's signed parts.
func FuzzVerifyAssertion(f *testing.F) {
	for _, section := range []string{"16.2", "16.6", "16.10", "16.11"} {
		v := vectorOf(section)
		f.Add(unhex(f, v.assertionClientDataJSON), unhex(f, v.authenticatorData), unhex(f, v.signature))
	}
	f.Add([]byte(`{"type":"webauthn.get"}`), []byte{0}, []byte{0})
	held := passkey.Assertion{}
	f.Fuzz(func(t *testing.T, clientDataJSON, authenticatorData, signature []byte) {
		if held.PublicKey == nil {
			held = assertionOf(t, vectorOf("16.6"))
		}
		a := held
		a.ClientDataJSON, a.AuthenticatorData, a.Signature = clientDataJSON, authenticatorData, signature
		if challenge, err := (Verifier{}).ChallengeOf(clientDataJSON); err == nil {
			a.Challenge = challenge
		}
		_, err := Verifier{}.VerifyAssertion(a)
		if err == nil {
			if string(clientDataJSON) != string(held.ClientDataJSON) ||
				string(authenticatorData) != string(held.AuthenticatorData) {
				t.Fatalf("an assertion verified that the credential never signed")
			}
			return
		}
		var domainErr *shared.Error
		if !errors.As(err, &domainErr) || domainErr.DetailCode != passkey.CodeSignInFailed || reasonOf(err) == "" {
			t.Fatalf("not the one refusal: %v", err)
		}
	})
}
