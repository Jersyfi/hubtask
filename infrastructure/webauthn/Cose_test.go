// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"testing"
)

// credentialKeyOf cuts the COSE key out of a vector's attestation object by the authenticator data's
// fixed layout: what the registration verifier does properly, done crudely, so these tests stand
// without it.
func credentialKeyOf(t *testing.T, attestationObject string) []byte {
	t.Helper()
	object, err := decodeCBORWhole(unhex(t, attestationObject))
	if err != nil {
		t.Fatal(err)
	}
	authData, ok := object.field("authData")
	if !ok {
		t.Fatal("no authData")
	}
	data := authData.bytes
	idLength := int(binary.BigEndian.Uint16(data[53:55]))
	_, rest, err := decodeCBOR(data[55+idLength:])
	if err != nil {
		t.Fatal(err)
	}
	return data[55+idLength : len(data)-len(rest)]
}

// signedOf is what an assertion signs: the authenticator data and the hash of the client data.
func signedOf(t *testing.T, v specVector) []byte {
	clientHash := sha256.Sum256(unhex(t, v.assertionClientDataJSON))
	return append(unhex(t, v.authenticatorData), clientHash[:]...)
}

// The specification's own credentials verify its own assertions, and a changed byte does not.
func TestTheSpecificationsKeysVerifyItsSignatures(t *testing.T) {
	for _, c := range []struct {
		section string
		alg     int64
	}{
		{"16.2", AlgES256}, {"16.3", AlgES256}, {"16.6", AlgES256}, {"16.7", AlgES256},
		{"16.10", AlgRS256}, {"16.11", AlgEdDSA}, {"16.13", AlgES256}, {"16.16", AlgES256},
	} {
		v := vectorOf(c.section)
		t.Run(c.section, func(t *testing.T) {
			key, err := ParsePublicKey(credentialKeyOf(t, v.attestationObject))
			if err != nil {
				t.Fatalf("%s refused: %v", v.title, err)
			}
			if key.Algorithm() != c.alg {
				t.Errorf("algorithm %d, want %d", key.Algorithm(), c.alg)
			}
			signed, sig := signedOf(t, v), unhex(t, v.signature)
			if !key.Verify(signed, sig) {
				t.Fatal("the specification's signature does not verify")
			}
			signed[len(signed)-1] ^= 1
			if key.Verify(signed, sig) {
				t.Error("a signature verified over a changed message")
			}
		})
	}
}

// The algorithms Hubtask does not offer are refused even though they are sound.
func TestTheAlgorithmsNotOfferedAreRefused(t *testing.T) {
	for section, reason := range map[string]string{
		"16.8": "cose.alg_unsupported", "16.9": "cose.alg_unsupported", "16.12": "cose.alg_unsupported",
	} {
		_, err := ParsePublicKey(credentialKeyOf(t, vectorOf(section).attestationObject))
		if got := reasonOf(err); got != reason {
			t.Errorf("%s: reason %q, want %q", section, got, reason)
		}
	}
}

func ec2Key(x, y any) cborMapOf {
	return cborMapOf{{1, 2}, {3, -7}, {-1, 1}, {-2, x}, {-3, y}}
}

func TestMalformedKeysAreRefused(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := priv.PublicKey.Bytes()
	x, y := raw[1:33], raw[33:]
	offCurve := append([]byte(nil), y...)
	offCurve[31] ^= 1
	edPub, _, _ := ed25519.GenerateKey(rand.Reader)
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	modulus2048 := new(big.Int).Lsh(big.NewInt(1), 2047)
	modulus2048.Or(modulus2048, big.NewInt(1))
	modulus4097 := new(big.Int).Lsh(big.NewInt(1), 4096)

	cases := []struct {
		name   string
		key    any
		reason string
	}{
		{"not a map", []any{1, 2}, "cose.not_a_map"},
		{"no kty", cborMapOf{{3, -7}}, "cose.kty"},
		{"an unknown kty", cborMapOf{{1, 4}, {3, -7}}, "cose.kty"},
		{"no alg", cborMapOf{{1, 2}, {-1, 1}, {-2, x}, {-3, y}}, "cose.alg_missing"},
		{"EC2 with EdDSA", cborMapOf{{1, 2}, {3, -8}, {-1, 1}, {-2, x}, {-3, y}}, "cose.alg_unsupported"},
		{"EC2 on P-384", cborMapOf{{1, 2}, {3, -7}, {-1, 2}, {-2, x}, {-3, y}}, "cose.curve"},
		{"EC2 compressed", ec2Key(x, true), "cose.ec2_point"},
		{"EC2 short x", ec2Key(x[:31], y), "cose.ec2_point"},
		{"EC2 off the curve", ec2Key(x, offCurve), "cose.ec2_point"},
		{"OKP with ES256", cborMapOf{{1, 1}, {3, -7}, {-1, 6}, {-2, []byte(edPub)}}, "cose.alg_unsupported"},
		{"OKP on X25519", cborMapOf{{1, 1}, {3, -8}, {-1, 4}, {-2, []byte(edPub)}}, "cose.curve"},
		{"OKP short", cborMapOf{{1, 1}, {3, -8}, {-1, 6}, {-2, []byte(edPub)[:31]}}, "cose.okp_key"},
		{"RSA with ES256", cborMapOf{{1, 3}, {3, -7}, {-1, small.N.Bytes()}, {-2, []byte{1, 0, 1}}}, "cose.alg_unsupported"},
		{"RSA without e", cborMapOf{{1, 3}, {3, -257}, {-1, small.N.Bytes()}}, "cose.rsa_key"},
		{"RSA of 1024 bits", cborMapOf{{1, 3}, {3, -257}, {-1, small.N.Bytes()}, {-2, []byte{1, 0, 1}}}, "cose.rsa_size"},
		{"RSA of 4097 bits", cborMapOf{{1, 3}, {3, -257}, {-1, modulus4097.Bytes()}, {-2, []byte{1, 0, 1}}}, "cose.rsa_size"},
		{"RSA with an even exponent", cborMapOf{{1, 3}, {3, -257}, {-1, modulus2048.Bytes()}, {-2, []byte{1, 0, 0}}}, "cose.rsa_exponent"},
		{"RSA with exponent 1", cborMapOf{{1, 3}, {3, -257}, {-1, modulus2048.Bytes()}, {-2, []byte{1}}}, "cose.rsa_exponent"},
		{"RSA with a long exponent", cborMapOf{{1, 3}, {3, -257}, {-1, modulus2048.Bytes()}, {-2, []byte{1, 0, 0, 0, 1}}}, "cose.rsa_exponent"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParsePublicKey(encodeCBOR(c.key))
			if got := reasonOf(err); got != c.reason {
				t.Errorf("reason %q, want %q (%v)", got, c.reason, err)
			}
		})
	}

	if _, err := ParsePublicKey(append(encodeCBOR(ec2Key(x, y)), 0)); reasonOf(err) != "cbor.trailing_bytes" {
		t.Errorf("a trailing byte: %v", err)
	}
}

// Keys made here verify signatures made here, so each algorithm is tested beyond the specification's
// one example; and a signature of one algorithm never passes for another.
func TestFreshKeysVerifyAndRefuseTheWrongSignature(t *testing.T) {
	message := []byte("authenticator data and the hash of the client data")
	digest := sha256.Sum256(message)

	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	raw, _ := priv.PublicKey.Bytes()
	ecKey, err := ParsePublicKey(encodeCBOR(ec2Key(raw[1:33], raw[33:])))
	if err != nil {
		t.Fatal(err)
	}
	ecSig, _ := ecdsa.SignASN1(rand.Reader, priv, digest[:])

	edPub, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	edKey, err := ParsePublicKey(encodeCBOR(cborMapOf{{1, 1}, {3, -8}, {-1, 6}, {-2, []byte(edPub)}}))
	if err != nil {
		t.Fatal(err)
	}
	edSig := ed25519.Sign(edPriv, message)

	rsaPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	rsaKey, err := ParsePublicKey(encodeCBOR(cborMapOf{{1, 3}, {3, -257}, {-1, rsaPriv.N.Bytes()}, {-2, big.NewInt(int64(rsaPriv.E)).Bytes()}}))
	if err != nil {
		t.Fatal(err)
	}
	rsaSig, _ := rsa.SignPKCS1v15(rand.Reader, rsaPriv, crypto.SHA256, digest[:])

	keys := map[string]PublicKey{"ES256": ecKey, "EdDSA": edKey, "RS256": rsaKey}
	sigs := map[string][]byte{"ES256": ecSig, "EdDSA": edSig, "RS256": rsaSig}
	for keyName, key := range keys {
		for sigName, sig := range sigs {
			if got, want := key.Verify(message, sig), keyName == sigName; got != want {
				t.Errorf("%s key, %s signature: verified %v", keyName, sigName, got)
			}
		}
		if key.Verify(message, nil) {
			t.Errorf("%s: an empty signature verified", keyName)
		}
	}
	if (PublicKey{}).Verify(message, ecSig) {
		t.Error("the zero key verified something")
	}
}
