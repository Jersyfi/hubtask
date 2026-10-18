// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/Jersyfi/hubtask/core/port/passkey"
)

// A registration a real browser made: Chromium's virtual authenticator, driven by the web app's walk
// (apps/webapp/e2e/passkeys.test.mjs, run with CAPTURE set). The specification's vectors prove the
// checks against the standard; this proves them against what a browser actually sends - its client
// data's field order and extra members, its attestation object's encoding - so a check stricter than
// the browsers would fail here before it failed a person.
type capturedRegistration struct {
	Origin            string   `json:"origin"`
	RPID              string   `json:"rp_id"`
	Challenge         string   `json:"challenge"`
	RawID             string   `json:"raw_id"`
	ClientDataJSON    string   `json:"client_data_json"`
	AttestationObject string   `json:"attestation_object"`
	Transports        []string `json:"transports"`
}

func TestARegistrationABrowserMadeIsAccepted(t *testing.T) {
	raw, err := os.ReadFile("testdata/chromium-registration.json")
	if err != nil {
		t.Fatal(err)
	}
	var captured capturedRegistration
	if err := json.Unmarshal(raw, &captured); err != nil {
		t.Fatal(err)
	}
	decode := func(text string) []byte {
		bytes, err := base64.RawURLEncoding.DecodeString(text)
		if err != nil {
			t.Fatalf("decoding %q: %v", text, err)
		}
		return bytes
	}
	registration := passkey.Registration{
		ClientDataJSON: decode(captured.ClientDataJSON), AttestationObject: decode(captured.AttestationObject),
		RawID: decode(captured.RawID), Transports: captured.Transports, Challenge: decode(captured.Challenge),
		RelyingParty: passkey.RelyingParty{ID: captured.RPID, Origin: captured.Origin},
	}

	challenge, err := Verifier{}.ChallengeOf(registration.ClientDataJSON)
	if err != nil || string(challenge) != string(registration.Challenge) {
		t.Fatalf("ChallengeOf: %x %v", challenge, err)
	}
	credential, err := Verifier{}.VerifyRegistration(registration)
	if err != nil {
		t.Fatalf("the browser's registration was refused: %v (%s)", err, reasonOf(err))
	}
	// The virtual authenticator verifies the user and is a platform authenticator.
	if !credential.UserVerified || credential.Algorithm != AlgES256 || len(credential.Transports) != 1 ||
		credential.Transports[0] != "internal" {
		t.Errorf("credential %+v", credential)
	}

	// The same answer for another workspace, or at another port, is refused.
	other := registration
	other.RelyingParty.ID = "acme.localhost"
	if _, err := (Verifier{}).VerifyRegistration(other); reasonOf(err) != "auth_data.rp_id" {
		t.Errorf("another RP ID: %v", err)
	}
	other = registration
	other.RelyingParty.Origin = "http://localhost:1"
	if _, err := (Verifier{}).VerifyRegistration(other); reasonOf(err) != "client_data.origin" {
		t.Errorf("another origin: %v", err)
	}
}
