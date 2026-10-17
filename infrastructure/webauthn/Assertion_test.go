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
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/passkey"
)

// assertionOf is a vector's authentication ceremony, checked against the credential its own
// registration made.
func assertionOf(t *testing.T, v specVector) passkey.Assertion {
	t.Helper()
	credential, err := Verifier{}.VerifyRegistration(registrationOf(t, v))
	if err != nil {
		t.Fatalf("%s: the registration was refused: %v", v.section, err)
	}
	return passkey.Assertion{
		ClientDataJSON: unhex(t, v.assertionClientDataJSON), AuthenticatorData: unhex(t, v.authenticatorData),
		Signature: unhex(t, v.signature), RawID: credential.ID, Challenge: unhex(t, v.assertionChallenge),
		RelyingParty: exampleOrg, CredentialID: credential.ID, PublicKey: credential.PublicKey,
		Algorithm: credential.Algorithm, BackupEligible: credential.BackupEligible,
	}
}

// The specification's assertions verify with their registration's key, and their flags are
// reported as they are - several carry presence without verification, which is the caller's to
// judge, not the verifier's.
func TestTheSpecificationsAssertionsVerifyAndReportTheirFlags(t *testing.T) {
	for _, section := range []string{"16.2", "16.3", "16.6", "16.7", "16.10", "16.11", "16.13", "16.14", "16.15", "16.16"} {
		v := vectorOf(section)
		t.Run(section, func(t *testing.T) {
			a := assertionOf(t, v)
			asserted, err := Verifier{}.VerifyAssertion(a)
			if err != nil {
				t.Fatalf("%s refused: %v (%s)", v.title, err, reasonOf(err))
			}
			flags := a.AuthenticatorData[32]
			if asserted.UserVerified != (flags&flagUV != 0) || asserted.BackedUp != (flags&flagBS != 0) ||
				asserted.SignCount != binary.BigEndian.Uint32(a.AuthenticatorData[33:37]) {
				t.Errorf("reported %+v for flags %#x", asserted, flags)
			}
		})
	}
}

func TestTheSpecificationsCrossOriginAssertionsAreRefused(t *testing.T) {
	for _, section := range []string{"16.4", "16.5"} {
		v := vectorOf(section)
		// Their registrations are cross-origin too; the credential is taken from a registration that
		// is not, so only the assertion's own client data decides.
		base := assertionOf(t, vectorOf("16.2"))
		base.ClientDataJSON = unhex(t, v.assertionClientDataJSON)
		base.Challenge = unhex(t, v.assertionChallenge)
		_, err := Verifier{}.VerifyAssertion(base)
		assertSignInRefused(t, section, err, "client_data.cross_origin")
	}
}

func TestEachPartOfASpecificationAssertionIsHeldToIt(t *testing.T) {
	base := assertionOf(t, vectorOf("16.6"))
	cases := map[string]struct {
		change func(a *passkey.Assertion)
		reason string
	}{
		"another challenge":     {func(a *passkey.Assertion) { a.Challenge = bytes.Repeat([]byte{1}, 32) }, "client_data.challenge"},
		"another origin":        {func(a *passkey.Assertion) { a.RelyingParty.Origin = "https://example.com" }, "client_data.origin"},
		"another RP ID":         {func(a *passkey.Assertion) { a.RelyingParty.ID = "example.com" }, "auth_data.rp_id"},
		"a registration's data": {func(a *passkey.Assertion) { a.ClientDataJSON = unhex(t, vectorOf("16.6").clientDataJSON) }, "client_data.type"},
		"a changed signature":   {func(a *passkey.Assertion) { a.Signature = append([]byte(nil), a.Signature...); a.Signature[10] ^= 1 }, "signature.invalid"},
		"a changed counter": {func(a *passkey.Assertion) {
			a.AuthenticatorData = append([]byte(nil), a.AuthenticatorData...)
			a.AuthenticatorData[36] ^= 1
		}, "signature.invalid"},
		"another credential's key": {func(a *passkey.Assertion) {
			other := assertionOf(t, vectorOf("16.2"))
			a.PublicKey, a.Algorithm = other.PublicKey, other.Algorithm
		}, "signature.invalid"},
		"a stored algorithm that is not the key's": {func(a *passkey.Assertion) { a.Algorithm = AlgRS256 }, "signature.algorithm"},
		"another raw id":      {func(a *passkey.Assertion) { a.RawID = []byte{1, 2, 3} }, "credential.raw_id"},
		"eligibility changed": {func(a *passkey.Assertion) { a.BackupEligible = !a.BackupEligible }, "auth_data.backup_eligibility"},
		"no signature":        {func(a *passkey.Assertion) { a.Signature = nil }, "signature.size"},
		"no relying party":    {func(a *passkey.Assertion) { a.RelyingParty = passkey.RelyingParty{} }, "relying_party.unset"},
		"no stored key":       {func(a *passkey.Assertion) { a.PublicKey = nil }, "relying_party.unset"},
		"a trailing byte": {func(a *passkey.Assertion) {
			a.AuthenticatorData = append(append([]byte(nil), a.AuthenticatorData...), 0)
		}, "auth_data.trailing_bytes"},
		"truncated data": {func(a *passkey.Assertion) { a.AuthenticatorData = a.AuthenticatorData[:36] }, "auth_data.truncated"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			a := base
			c.change(&a)
			_, err := Verifier{}.VerifyAssertion(a)
			assertSignInRefused(t, name, err, c.reason)
		})
	}
}

func assertSignInRefused(t *testing.T, name string, err error, reason string) {
	t.Helper()
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.Category != shared.CategoryUnauthenticated ||
		domainErr.DetailCode != passkey.CodeSignInFailed {
		t.Errorf("%s: not the sign-in's one refusal: %v", name, err)
		return
	}
	if got := reasonOf(err); got != reason {
		t.Errorf("%s: reason %q, want %q", name, got, reason)
	}
}

// signer is a test authenticator's sign-in half: a P-256 key whose assertions can be shaped flag by
// flag and byte by byte.
type signer struct {
	key  *ecdsa.PrivateKey
	cose []byte
}

func newSigner(t *testing.T) signer {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := key.PublicKey.Bytes()
	return signer{key: key, cose: encodeCBOR(ec2Key(raw[1:33], raw[33:]))}
}

type assertionShape struct {
	clientData map[string]any
	rpID       string
	flags      byte
	signCount  uint32
	extensions []byte
	trailing   []byte
}

func defaultShape() assertionShape {
	return assertionShape{
		clientData: map[string]any{
			"type": "webauthn.get", "challenge": base64.RawURLEncoding.EncodeToString(testChallenge),
			"origin": testParty.Origin, "crossOrigin": false,
		},
		rpID: testParty.ID, flags: flagUP | flagUV, signCount: 7,
	}
}

func (s signer) assert(t *testing.T, shape assertionShape) passkey.Assertion {
	t.Helper()
	hash := sha256.Sum256([]byte(shape.rpID))
	data := append([]byte(nil), hash[:]...)
	data = append(data, shape.flags)
	data = binary.BigEndian.AppendUint32(data, shape.signCount)
	data = append(append(data, shape.extensions...), shape.trailing...)
	clientJSON := []byte(jsonOf(t, shape.clientData))
	clientHash := sha256.Sum256(clientJSON)
	digest := sha256.Sum256(append(append([]byte(nil), data...), clientHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, s.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	id := []byte{0x07, 0x07, 0x07}
	return passkey.Assertion{
		ClientDataJSON: clientJSON, AuthenticatorData: data, Signature: sig, RawID: id,
		Challenge: testChallenge, RelyingParty: testParty, CredentialID: id, PublicKey: s.cose,
		Algorithm: AlgES256, BackupEligible: shape.flags&flagBE != 0,
	}
}

func TestATestAuthenticatorsAssertionVerifies(t *testing.T) {
	s := newSigner(t)
	asserted, err := Verifier{}.VerifyAssertion(s.assert(t, defaultShape()))
	if err != nil || !asserted.UserVerified || asserted.BackedUp || asserted.SignCount != 7 {
		t.Fatalf("%+v %v", asserted, err)
	}
	// Presence alone verifies, and says so: the caller decides whether that is enough.
	shape := defaultShape()
	shape.flags = flagUP | flagBE | flagBS
	shape.signCount = 0
	asserted, err = Verifier{}.VerifyAssertion(s.assert(t, shape))
	if err != nil || asserted.UserVerified || !asserted.BackedUp || asserted.SignCount != 0 {
		t.Fatalf("presence only: %+v %v", asserted, err)
	}
	// Extensions announced and present.
	shape = defaultShape()
	shape.flags |= flagED
	shape.extensions = encodeCBOR(cborMapOf{{"credProtect", 2}})
	if _, err := (Verifier{}).VerifyAssertion(s.assert(t, shape)); err != nil {
		t.Errorf("with extensions: %v", err)
	}
}

func TestEveryCheckOfAnAssertionRefusesOnItsOwn(t *testing.T) {
	s := newSigner(t)
	cases := map[string]struct {
		change func(shape *assertionShape)
		reason string
	}{
		"a registration's type":         {func(sh *assertionShape) { sh.clientData["type"] = "webauthn.create" }, "client_data.type"},
		"cross-origin":                  {func(sh *assertionShape) { sh.clientData["crossOrigin"] = true }, "client_data.cross_origin"},
		"a top origin":                  {func(sh *assertionShape) { sh.clientData["topOrigin"] = "https://evil.example" }, "client_data.cross_origin"},
		"token binding present":         {func(sh *assertionShape) { sh.clientData["tokenBinding"] = map[string]any{"status": "present"} }, "client_data.token_binding"},
		"the parent's origin":           {func(sh *assertionShape) { sh.clientData["origin"] = "https://hubtask.example" }, "client_data.origin"},
		"another workspace":             {func(sh *assertionShape) { sh.rpID = "other.hubtask.example" }, "auth_data.rp_id"},
		"no presence":                   {func(sh *assertionShape) { sh.flags = flagUV }, "auth_data.user_presence"},
		"backed up, not eligible":       {func(sh *assertionShape) { sh.flags |= flagBS }, "auth_data.backup_state"},
		"attested data in an assertion": {func(sh *assertionShape) { sh.flags |= flagAT }, "auth_data.attested_in_assertion"},
		"ED without extensions":         {func(sh *assertionShape) { sh.flags |= flagED }, "cbor.truncated"},
		"extensions not a map":          {func(sh *assertionShape) { sh.flags |= flagED; sh.extensions = encodeCBOR([]any{1}) }, "auth_data.extensions"},
		"extensions without ED":         {func(sh *assertionShape) { sh.extensions = encodeCBOR(cborMapOf{{"x", 1}}) }, "auth_data.trailing_bytes"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			shape := defaultShape()
			c.change(&shape)
			_, err := Verifier{}.VerifyAssertion(s.assert(t, shape))
			assertSignInRefused(t, name, err, c.reason)
		})
	}

	a := s.assert(t, defaultShape())
	a.AuthenticatorData = make([]byte, maxAuthenticatorData+1)
	_, err := Verifier{}.VerifyAssertion(a)
	assertSignInRefused(t, "oversized data", err, "auth_data.size")
	a = s.assert(t, defaultShape())
	a.Signature = make([]byte, maxSignature+1)
	_, err = Verifier{}.VerifyAssertion(a)
	assertSignInRefused(t, "an oversized signature", err, "signature.size")
}
