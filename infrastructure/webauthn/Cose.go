// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"math/big"
)

// The COSE algorithms a credential may use (RFC 9053, RFC 8230), and the only three the creation
// options offer (identity.md §23.3).
const (
	AlgES256 int64 = -7
	AlgEdDSA int64 = -8
	AlgRS256 int64 = -257
)

// The COSE key labels and values read here (RFC 9052 §7, RFC 9053 §7, RFC 8230 §4).
const (
	coseKty = 1
	coseAlg = 3

	coseKtyOKP = 1
	coseKtyEC2 = 2
	coseKtyRSA = 3

	coseCrv = -1 // EC2 and OKP
	coseX   = -2 // EC2 and OKP
	coseY   = -3 // EC2
	coseN   = -1 // RSA
	coseE   = -2 // RSA

	coseCrvP256    = 1
	coseCrvEd25519 = 6
)

// The RSA bounds: below 2048 bits a key is no longer considered safe; above 4096 a signature check
// becomes a way to spend the server's processor.
const (
	minRSABits = 2048
	maxRSABits = 4096
)

// PublicKey is a credential's public key, checked: on its curve, of its length, of an algorithm
// Hubtask accepts, the algorithm matching the key's type.
type PublicKey struct {
	alg     int64
	ecdsa   *ecdsa.PublicKey
	ed25519 ed25519.PublicKey
	rsa     *rsa.PublicKey
}

// Algorithm is the key's COSE algorithm.
func (k PublicKey) Algorithm() int64 { return k.alg }

// Verify reports whether sig is the key's signature over signed: ECDSA as ASN.1 DER over SHA-256,
// Ed25519 over the message itself, RSASSA-PKCS1-v1_5 over SHA-256 (WebAuthn Level 3 §6.5.6).
func (k PublicKey) Verify(signed, sig []byte) bool {
	switch k.alg {
	case AlgES256:
		digest := sha256.Sum256(signed)
		return ecdsa.VerifyASN1(k.ecdsa, digest[:], sig)
	case AlgEdDSA:
		return len(sig) == ed25519.SignatureSize && ed25519.Verify(k.ed25519, signed, sig)
	case AlgRS256:
		digest := sha256.Sum256(signed)
		return rsa.VerifyPKCS1v15(k.rsa, crypto.SHA256, digest[:], sig) == nil
	default:
		return false
	}
}

// ParsePublicKey reads the COSE bytes a credential was stored with.
func ParsePublicKey(cose []byte) (PublicKey, error) {
	value, err := decodeCBORWhole(cose)
	if err != nil {
		return PublicKey{}, err
	}
	return publicKeyOf(value)
}

// publicKeyOf checks a decoded COSE key. Labels it does not read (a `kid`, `key_ops`) are let be:
// they change nothing about what the key verifies.
func publicKeyOf(key cborValue) (PublicKey, error) {
	if key.kind != cborMap {
		return PublicKey{}, refuse("cose.not_a_map")
	}
	kty, ok := intAt(key, coseKty)
	if !ok {
		return PublicKey{}, refuse("cose.kty")
	}
	alg, ok := intAt(key, coseAlg)
	if !ok {
		return PublicKey{}, refuse("cose.alg_missing")
	}

	switch kty {
	case coseKtyEC2:
		if alg != AlgES256 {
			return PublicKey{}, refuse("cose.alg_unsupported")
		}
		if crv, ok := intAt(key, coseCrv); !ok || crv != coseCrvP256 {
			return PublicKey{}, refuse("cose.curve")
		}
		x, okX := bytesAt(key, coseX)
		// A boolean y is the compressed form (RFC 9053 §7.1.1), which WebAuthn does not use.
		y, okY := bytesAt(key, coseY)
		if !okX || !okY || len(x) != 32 || len(y) != 32 {
			return PublicKey{}, refuse("cose.ec2_point")
		}
		point := append(append([]byte{0x04}, x...), y...)
		pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
		if err != nil {
			return PublicKey{}, refuse("cose.ec2_point")
		}
		return PublicKey{alg: alg, ecdsa: pub}, nil

	case coseKtyOKP:
		if alg != AlgEdDSA {
			return PublicKey{}, refuse("cose.alg_unsupported")
		}
		if crv, ok := intAt(key, coseCrv); !ok || crv != coseCrvEd25519 {
			return PublicKey{}, refuse("cose.curve")
		}
		x, ok := bytesAt(key, coseX)
		if !ok || len(x) != ed25519.PublicKeySize {
			return PublicKey{}, refuse("cose.okp_key")
		}
		return PublicKey{alg: alg, ed25519: ed25519.PublicKey(append([]byte(nil), x...))}, nil

	case coseKtyRSA:
		if alg != AlgRS256 {
			return PublicKey{}, refuse("cose.alg_unsupported")
		}
		n, okN := bytesAt(key, coseN)
		e, okE := bytesAt(key, coseE)
		if !okN || !okE {
			return PublicKey{}, refuse("cose.rsa_key")
		}
		modulus := new(big.Int).SetBytes(n)
		if bits := modulus.BitLen(); bits < minRSABits || bits > maxRSABits {
			return PublicKey{}, refuse("cose.rsa_size")
		}
		// At most four bytes, odd, at least three: what crypto/rsa accepts, and what every
		// authenticator sends (65537).
		if len(e) == 0 || len(e) > 4 {
			return PublicKey{}, refuse("cose.rsa_exponent")
		}
		exponent := new(big.Int).SetBytes(e)
		if exponent.Bit(0) == 0 || exponent.Cmp(big.NewInt(3)) < 0 || exponent.BitLen() > 31 {
			return PublicKey{}, refuse("cose.rsa_exponent")
		}
		return PublicKey{alg: alg, rsa: &rsa.PublicKey{N: modulus, E: int(exponent.Int64())}}, nil

	default:
		return PublicKey{}, refuse("cose.kty")
	}
}

func intAt(m cborValue, label int64) (int64, bool) {
	v, ok := m.lookup(label)
	if !ok {
		return 0, false
	}
	return v.int()
}

func bytesAt(m cborValue, label int64) ([]byte, bool) {
	v, ok := m.lookup(label)
	if !ok || v.kind != cborBytes {
		return nil, false
	}
	return v.bytes, true
}
