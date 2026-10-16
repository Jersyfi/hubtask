// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/passkey"
)

var exampleOrg = passkey.RelyingParty{ID: "example.org", Origin: "https://example.org"}

// rawIDOf reads the credential id out of an attestation object, as the browser reports it.
func rawIDOf(t *testing.T, attestationObject []byte) []byte {
	t.Helper()
	object, err := decodeCBORWhole(attestationObject)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := object.field("authData")
	length := int(binary.BigEndian.Uint16(data.bytes[53:55]))
	return data.bytes[55 : 55+length]
}

func registrationOf(t *testing.T, v specVector) passkey.Registration {
	attestation := unhex(t, v.attestationObject)
	return passkey.Registration{
		ClientDataJSON:    unhex(t, v.clientDataJSON),
		AttestationObject: attestation,
		RawID:             rawIDOf(t, attestation),
		Challenge:         unhex(t, v.challenge),
		RelyingParty:      exampleOrg,
	}
}

// The specification's registrations of the offered algorithms are accepted, whatever their
// attestation format (D6: the statement is unread), and the key kept is the one that verifies the
// same credential's assertion.
func TestTheSpecificationsRegistrationsAreAccepted(t *testing.T) {
	for _, section := range []string{"16.2", "16.3", "16.6", "16.7", "16.10", "16.11", "16.13", "16.14", "16.15", "16.16"} {
		v := vectorOf(section)
		t.Run(section, func(t *testing.T) {
			r := registrationOf(t, v)
			credential, err := Verifier{}.VerifyRegistration(r)
			if err != nil {
				t.Fatalf("%s refused: %v", v.title, err)
			}
			if !bytes.Equal(credential.ID, r.RawID) {
				t.Error("the credential id is not the authenticator's")
			}
			key, err := ParsePublicKey(credential.PublicKey)
			if err != nil {
				t.Fatalf("the kept key does not parse again: %v", err)
			}
			if key.Algorithm() != credential.Algorithm {
				t.Errorf("algorithm %d kept as %d", key.Algorithm(), credential.Algorithm)
			}
			if !key.Verify(signedOf(t, v), unhex(t, v.signature)) {
				t.Error("the kept key does not verify the credential's assertion")
			}
			challenge, err := Verifier{}.ChallengeOf(r.ClientDataJSON)
			if err != nil || !bytes.Equal(challenge, r.Challenge) {
				t.Errorf("ChallengeOf: %x %v", challenge, err)
			}
		})
	}
}

func TestTheSpecificationsRefusedRegistrations(t *testing.T) {
	for section, reason := range map[string]string{
		"16.4":  "client_data.cross_origin",
		"16.5":  "client_data.cross_origin",
		"16.8":  "cose.alg_unsupported",
		"16.9":  "cose.alg_unsupported",
		"16.12": "cose.alg_unsupported",
	} {
		_, err := Verifier{}.VerifyRegistration(registrationOf(t, vectorOf(section)))
		assertRefused(t, section, err, reason)
	}

	// The vector's own flags, read rather than assumed: 16.2's authenticator set UP, BE, BS and AT.
	credential, err := Verifier{}.VerifyRegistration(registrationOf(t, vectorOf("16.2")))
	if err != nil || credential.UserVerified || !credential.BackupEligible || !credential.BackedUp {
		t.Errorf("16.2's flags read as %+v (%v)", credential, err)
	}
}

func TestTheExpectationsAreHeldToTheVector(t *testing.T) {
	base := registrationOf(t, vectorOf("16.2"))
	cases := map[string]struct {
		change func(r *passkey.Registration)
		reason string
	}{
		"another origin":    {func(r *passkey.Registration) { r.RelyingParty.Origin = "https://example.org:8443" }, "client_data.origin"},
		"another RP ID":     {func(r *passkey.Registration) { r.RelyingParty.ID = "org" }, "auth_data.rp_id"},
		"another challenge": {func(r *passkey.Registration) { r.Challenge = bytes.Repeat([]byte{1}, 32) }, "client_data.challenge"},
		"a sign-in's data":  {func(r *passkey.Registration) { r.ClientDataJSON = unhex(t, vectorOf("16.2").assertionClientDataJSON) }, "client_data.type"},
		"another raw id":    {func(r *passkey.Registration) { r.RawID = append([]byte{0}, r.RawID...) }, "credential.raw_id"},
		"no relying party":  {func(r *passkey.Registration) { r.RelyingParty = passkey.RelyingParty{} }, "relying_party.unset"},
		"no challenge":      {func(r *passkey.Registration) { r.Challenge = nil }, "relying_party.unset"},
		"an unknown hint":   {func(r *passkey.Registration) { r.Transports = []string{"usb", "carrier-pigeon"} }, "credential.transports"},
		"too many hints": {func(r *passkey.Registration) {
			r.Transports = []string{"usb", "nfc", "ble", "hybrid", "internal", "smart-card", "usb"}
		}, "credential.transports"},
		"a trailing byte":    {func(r *passkey.Registration) { r.AttestationObject = append(r.AttestationObject, 0) }, "cbor.trailing_bytes"},
		"a truncated object": {func(r *passkey.Registration) { r.AttestationObject = r.AttestationObject[:40] }, "cbor.truncated"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := base
			c.change(&r)
			_, err := Verifier{}.VerifyRegistration(r)
			assertRefused(t, name, err, c.reason)
		})
	}
}

func assertRefused(t *testing.T, name string, err error, reason string) {
	t.Helper()
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.Category != shared.CategoryValidation ||
		domainErr.DetailCode != passkey.CodeRegistrationInvalid {
		t.Errorf("%s: not the one refusal: %v", name, err)
		return
	}
	if got := reasonOf(err); got != reason {
		t.Errorf("%s: reason %q, want %q", name, got, reason)
	}
}

// ceremony is a test authenticator and browser in one: everything a registration carries, each part
// changeable, so that every check can be failed on its own.
type ceremony struct {
	clientData  map[string]any
	rawClient   []byte
	rpID        string
	flags       byte
	signCount   uint32
	credID      []byte
	cose        []byte
	extensions  []byte
	trailing    []byte
	format      any
	statement   any
	extraMember bool
}

var testChallenge = bytes.Repeat([]byte{0x2a}, 32)

var testParty = passkey.RelyingParty{ID: "acme.hubtask.example", Origin: "https://acme.hubtask.example"}

func newCeremony(t *testing.T) ceremony {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := priv.PublicKey.Bytes()
	return ceremony{
		clientData: map[string]any{
			"type": "webauthn.create", "challenge": base64.RawURLEncoding.EncodeToString(testChallenge),
			"origin": testParty.Origin, "crossOrigin": false,
		},
		rpID:      testParty.ID,
		flags:     flagUP | flagUV | flagAT,
		signCount: 0,
		credID:    bytes.Repeat([]byte{0x07}, 16),
		cose:      encodeCBOR(ec2Key(raw[1:33], raw[33:])),
		format:    "none",
		statement: cborMapOf{},
	}
}

func (c ceremony) authData() []byte {
	hash := sha256.Sum256([]byte(c.rpID))
	out := append([]byte(nil), hash[:]...)
	out = append(out, c.flags)
	out = binary.BigEndian.AppendUint32(out, c.signCount)
	if c.flags&flagAT != 0 || c.credID != nil {
		out = append(out, make([]byte, 16)...)
		out = binary.BigEndian.AppendUint16(out, uint16(len(c.credID)))
		out = append(out, c.credID...)
		out = append(out, c.cose...)
	}
	out = append(out, c.extensions...)
	return append(out, c.trailing...)
}

func (c ceremony) registration(t *testing.T) passkey.Registration {
	clientJSON := c.rawClient
	if clientJSON == nil {
		clientJSON = []byte(jsonOf(t, c.clientData))
	}
	object := cborMapOf{{"fmt", c.format}, {"attStmt", c.statement}, {"authData", c.authData()}}
	if c.extraMember {
		object = append(object, cborEntry{"epAtt", true})
	}
	return passkey.Registration{
		ClientDataJSON: clientJSON, AttestationObject: encodeCBOR(object), RawID: c.credID,
		Challenge: testChallenge, RelyingParty: testParty,
	}
}

func jsonOf(t *testing.T, m map[string]any) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("{")
	first := true
	for _, key := range []string{"type", "challenge", "origin", "crossOrigin", "topOrigin", "tokenBinding", "status"} {
		value, ok := m[key]
		if !ok {
			continue
		}
		if !first {
			b.WriteString(",")
		}
		first = false
		b.WriteString(`"` + key + `":`)
		switch v := value.(type) {
		case string:
			b.WriteString(`"` + v + `"`)
		case bool:
			if v {
				b.WriteString("true")
			} else {
				b.WriteString("false")
			}
		case map[string]any:
			b.WriteString(jsonOf(t, v))
		}
	}
	b.WriteString("}")
	return b.String()
}

func TestATestAuthenticatorsCeremonyIsAccepted(t *testing.T) {
	c := newCeremony(t)
	credential, err := Verifier{}.VerifyRegistration(c.registration(t))
	if err != nil {
		t.Fatal(err)
	}
	if !credential.UserVerified || credential.BackupEligible || credential.Algorithm != AlgES256 ||
		!bytes.Equal(credential.PublicKey, c.cose) {
		t.Errorf("credential %+v", credential)
	}

	// Presence alone registers: what a credential proves is read from each later ceremony.
	c.flags = flagUP | flagAT
	c.signCount = 7
	r := c.registration(t)
	r.Transports = []string{"usb", "nfc", "usb"}
	credential, err = Verifier{}.VerifyRegistration(r)
	if err != nil || credential.UserVerified || credential.SignCount != 7 ||
		strings.Join(credential.Transports, ",") != "usb,nfc" {
		t.Errorf("presence only: %+v %v", credential, err)
	}

	// Extensions announced and present, as hmac-secret writes them.
	c = newCeremony(t)
	c.flags |= flagED
	c.extensions = encodeCBOR(cborMapOf{{"hmac-secret", true}, {"credProtect", 2}})
	if _, err := (Verifier{}).VerifyRegistration(c.registration(t)); err != nil {
		t.Errorf("with extensions: %v", err)
	}

	// Another format is accepted with its statement unread.
	c = newCeremony(t)
	c.format = "packed"
	c.statement = cborMapOf{{"alg", -7}, {"sig", []byte{1, 2, 3}}}
	if _, err := (Verifier{}).VerifyRegistration(c.registration(t)); err != nil {
		t.Errorf("packed: %v", err)
	}

	// The longest credential id passes.
	c = newCeremony(t)
	c.credID = bytes.Repeat([]byte{9}, maxCredentialID)
	if _, err := (Verifier{}).VerifyRegistration(c.registration(t)); err != nil {
		t.Errorf("a 1023-byte id: %v", err)
	}
}

func TestEveryCheckOfARegistrationRefusesOnItsOwn(t *testing.T) {
	cases := map[string]struct {
		change func(c *ceremony)
		reason string
	}{
		"client data not JSON": {func(c *ceremony) { c.rawClient = []byte("{type") }, "client_data.json"},
		"client data empty":    {func(c *ceremony) { c.rawClient = []byte{} }, "client_data.size"},
		"client data too long": {func(c *ceremony) {
			c.rawClient = append([]byte(`{"x":"`), bytes.Repeat([]byte("a"), maxClientDataJSON)...)
		}, "client_data.size"},
		"type of a sign-in":         {func(c *ceremony) { c.clientData["type"] = "webauthn.get" }, "client_data.type"},
		"challenge not base64url":   {func(c *ceremony) { c.clientData["challenge"] = "a+b/" }, "client_data.challenge"},
		"challenge padded":          {func(c *ceremony) { c.clientData["challenge"] = base64.URLEncoding.EncodeToString(testChallenge) }, "client_data.challenge"},
		"another challenge":         {func(c *ceremony) { c.clientData["challenge"] = base64.RawURLEncoding.EncodeToString([]byte("other")) }, "client_data.challenge"},
		"origin of the parent":      {func(c *ceremony) { c.clientData["origin"] = "https://hubtask.example" }, "client_data.origin"},
		"origin over http":          {func(c *ceremony) { c.clientData["origin"] = "http://acme.hubtask.example" }, "client_data.origin"},
		"origin with a port":        {func(c *ceremony) { c.clientData["origin"] = "https://acme.hubtask.example:443" }, "client_data.origin"},
		"cross-origin":              {func(c *ceremony) { c.clientData["crossOrigin"] = true }, "client_data.cross_origin"},
		"a top origin":              {func(c *ceremony) { c.clientData["topOrigin"] = "https://evil.example" }, "client_data.cross_origin"},
		"token binding present":     {func(c *ceremony) { c.clientData["tokenBinding"] = map[string]any{"status": "present"} }, "client_data.token_binding"},
		"a fourth member":           {func(c *ceremony) { c.extraMember = true }, "attestation.shape"},
		"fmt not text":              {func(c *ceremony) { c.format = 1 }, "attestation.shape"},
		"fmt empty":                 {func(c *ceremony) { c.format = "" }, "attestation.format"},
		"fmt too long":              {func(c *ceremony) { c.format = strings.Repeat("f", maxFormatLength+1) }, "attestation.format"},
		"attStmt not a map":         {func(c *ceremony) { c.statement = []any{} }, "attestation.shape"},
		"none with a statement":     {func(c *ceremony) { c.statement = cborMapOf{{"sig", []byte{1}}} }, "attestation.none_statement"},
		"another RP ID":             {func(c *ceremony) { c.rpID = "hubtask.example" }, "auth_data.rp_id"},
		"no presence":               {func(c *ceremony) { c.flags = flagUV | flagAT }, "auth_data.user_presence"},
		"backed up, not eligible":   {func(c *ceremony) { c.flags |= flagBS }, "auth_data.backup_state"},
		"no attested credential":    {func(c *ceremony) { c.flags = flagUP | flagUV; c.credID = nil }, "auth_data.no_credential"},
		"an empty credential id":    {func(c *ceremony) { c.credID = []byte{} }, "credential.id_length"},
		"a credential id too long":  {func(c *ceremony) { c.credID = bytes.Repeat([]byte{9}, maxCredentialID+1) }, "credential.id_length"},
		"extensions without ED":     {func(c *ceremony) { c.extensions = encodeCBOR(cborMapOf{{"credProtect", 2}}) }, "auth_data.trailing_bytes"},
		"ED without extensions":     {func(c *ceremony) { c.flags |= flagED }, "cbor.truncated"},
		"extensions not a map":      {func(c *ceremony) { c.flags |= flagED; c.extensions = encodeCBOR([]any{1}) }, "auth_data.extensions"},
		"a byte after the key":      {func(c *ceremony) { c.trailing = []byte{0} }, "auth_data.trailing_bytes"},
		"a key that is no COSE key": {func(c *ceremony) { c.cose = encodeCBOR(cborMapOf{{1, 2}, {3, -7}}) }, "cose.curve"},
		"a key with a tag":          {func(c *ceremony) { c.cose = []byte{0xc0, 0x01} }, "cbor.tag"},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			c := newCeremony(t)
			change.change(&c)
			_, err := Verifier{}.VerifyRegistration(c.registration(t))
			assertRefused(t, name, err, change.reason)
		})
	}
}

func TestAnAuthenticatorDataCutShortIsRefused(t *testing.T) {
	full := newCeremony(t).authData()
	for _, length := range []int{0, 36, 37, 52, 54, 60, len(full) - 1} {
		_, err := parseAuthenticatorData(full[:length])
		if err == nil {
			t.Errorf("%d bytes accepted", length)
		}
		if reasonOf(err) == "" {
			t.Errorf("%d bytes: a refusal without a reason: %v", length, err)
		}
	}
}

func TestChallengeOfReadsOnlyTheShape(t *testing.T) {
	for name, raw := range map[string]string{
		"not JSON":     "{",
		"no challenge": `{"type":"webauthn.create"}`,
		"not base64":   `{"challenge":"***"}`,
		"a number":     `{"challenge":5}`,
	} {
		_, err := Verifier{}.ChallengeOf([]byte(raw))
		var domainErr *shared.Error
		if !errors.As(err, &domainErr) || domainErr.DetailCode != passkey.CodeRegistrationInvalid {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A wrong type or origin is not ChallengeOf's to judge: the verification after the lookup is.
	challenge, err := Verifier{}.ChallengeOf([]byte(`{"type":"webauthn.get","challenge":"AQID","origin":"x"}`))
	if err != nil || !bytes.Equal(challenge, []byte{1, 2, 3}) {
		t.Errorf("%x %v", challenge, err)
	}
}
