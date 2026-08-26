// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"fmt"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	port "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The layout of a sealed value, version 1. Every part is fixed width, so there is nothing to
// length-prefix and nothing a caller could lie about:
//
//	[0]        version
//	[1:13]     the nonce the data key is wrapped under
//	[13:61]    the wrapped data key: 32 bytes of key and GCM's 16-byte tag
//	[61:73]    the nonce the value is encrypted under
//	[73:]      the value and its tag
const (
	envelopeVersion = 1

	nonceBytes   = 12
	tagBytes     = 16
	dataKeyBytes = 32

	wrapOffset    = 1 + nonceBytes
	dataKeyEnd    = wrapOffset + dataKeyBytes + tagBytes
	valueOffset   = dataKeyEnd + nonceBytes
	minimumLength = valueOffset + tagBytes
)

// The two additional-data labels. Separate, so that the wrapped key of one value cannot be
// presented as the body of another, and both carry the caller's purpose - which is what makes a
// ciphertext moved between rows fail rather than open.
const (
	wrapLabel = "hubtask/envelope/v1/wrap:"
	dataLabel = "hubtask/envelope/v1/data:"
)

// maxPlaintextBytes bounds what this envelope will seal.
//
// Everything it protects is a small value a person configured - a password, an access key, a
// connection string - and a bound is what keeps the arithmetic below provably safe: the capacity
// of the buffer is a header length plus the plaintext's, and without a ceiling that sum is an
// overflow away from a negative allocation. An archive is not sealed here; it is encrypted as it
// streams, which is a different piece of code with a different shape.
const maxPlaintextBytes = 1 << 20

// The port's codes, referenced here so that this adapter cannot invent a second spelling.
const (
	portCodeNoEncryptionKey  = port.CodeNoEncryptionKey
	portCodeUnknownKey       = port.CodeUnknownKey
	portCodeCiphertextBroken = port.CodeCiphertextMalformed
)

// Envelope seals a value under a fresh data key and seals that key under the installation's
// current master key (security.md §3, backup-restore.md §4).
//
// Two layers rather than one, and not for ceremony. AES-GCM is safe as long as a nonce is never
// reused under one key, and the risk of that grows with the number of values encrypted under it;
// with an envelope the master key only ever encrypts random 32-byte keys, one per value, while
// every value gets a key of its own. It is also what makes the stored key identifier worth
// having: rotating the master key re-seals nothing, because what it protects is one key per row
// rather than the rows.
type Envelope struct {
	ring    Keyring
	entropy clockport.Entropy
}

func NewEnvelope(ring Keyring, entropy clockport.Entropy) Envelope {
	return Envelope{ring: ring, entropy: entropy}
}

var _ port.Encryptor = Envelope{}

// ActiveKeyID is the key new values are sealed under.
func (e Envelope) ActiveKeyID() string { return e.ring.ActiveKeyID() }

// Seal encrypts under a data key of its own and wraps that key under the current master key.
func (e Envelope) Seal(
	_ context.Context, plaintext secret.Secret, purpose port.Purpose,
) (port.Sealed, error) {
	if len(plaintext.Reveal()) > maxPlaintextBytes {
		// Refused rather than truncated, and refused before the key is touched: a value this
		// large is not a credential, and sealing part of one would store something that opens to
		// a broken secret.
		return port.Sealed{}, shared.ErrValidation.
			WithDetail("crypto.value_too_large").
			WithParams(map[string]string{"limit_bytes": fmt.Sprint(maxPlaintextBytes)})
	}
	if e.ring.IsEmpty() {
		// A refusal rather than a plaintext write. An installation that has not been given a key
		// must not end up with a credential in a column named `credential_enc`.
		return port.Sealed{}, shared.ErrUnavailable.WithDetail(portCodeNoEncryptionKey)
	}

	master, err := e.ring.find(e.ring.ActiveKeyID())
	if err != nil {
		return port.Sealed{}, err
	}

	dataKey, err := e.entropy.Bytes(dataKeyBytes)
	if err != nil {
		return port.Sealed{}, shared.ErrUnavailable.
			WithDetail("crypto.entropy_unavailable").
			WithCause(fmt.Errorf("drawing a data key: %w", err))
	}
	wrapNonce, err := e.entropy.Bytes(nonceBytes)
	if err != nil {
		return port.Sealed{}, shared.ErrUnavailable.
			WithDetail("crypto.entropy_unavailable").
			WithCause(fmt.Errorf("drawing the wrapping nonce: %w", err))
	}
	dataNonce, err := e.entropy.Bytes(nonceBytes)
	if err != nil {
		return port.Sealed{}, shared.ErrUnavailable.
			WithDetail("crypto.entropy_unavailable").
			WithCause(fmt.Errorf("drawing the value nonce: %w", err))
	}

	masterGCM, err := gcm(master.Reveal())
	if err != nil {
		return port.Sealed{}, err
	}
	dataGCM, err := gcm(dataKey)
	if err != nil {
		return port.Sealed{}, err
	}

	sealed := make([]byte, 0, minimumLength+len(plaintext.Reveal()))
	sealed = append(sealed, envelopeVersion)
	sealed = append(sealed, wrapNonce...)
	sealed = masterGCM.Seal(sealed, wrapNonce, dataKey, additional(wrapLabel, purpose))
	sealed = append(sealed, dataNonce...)
	sealed = dataGCM.Seal(sealed, dataNonce, []byte(plaintext.Reveal()), additional(dataLabel, purpose))

	return port.Sealed{KeyID: e.ring.ActiveKeyID(), Ciphertext: sealed}, nil
}

// Open reverses it, and answers nothing at all when anything is wrong.
//
// The order matters: the wrapped key is authenticated before the value is touched, and the value
// is authenticated by GCM before a single byte of it is returned. There is no code path here that
// produces a partial plaintext, which is the property the acceptance asks for.
func (e Envelope) Open(
	_ context.Context, sealed port.Sealed, purpose port.Purpose,
) (secret.Secret, error) {
	if sealed.KeyID == "" {
		return secret.Secret{}, shared.ErrValidation.WithDetail(portCodeCiphertextBroken)
	}
	master, err := e.ring.find(sealed.KeyID)
	if err != nil {
		return secret.Secret{}, err
	}

	body := sealed.Ciphertext
	if len(body) < minimumLength || body[0] != envelopeVersion {
		// Not one of ours, or one this build does not know how to read. Its own answer, because
		// "this installation cannot read that format" and "that is the wrong key" send an
		// operator in different directions.
		return secret.Secret{}, shared.ErrValidation.WithDetail(portCodeCiphertextBroken)
	}

	masterGCM, err := gcm(master.Reveal())
	if err != nil {
		return secret.Secret{}, err
	}
	dataKey, err := masterGCM.Open(
		nil, body[1:wrapOffset], body[wrapOffset:dataKeyEnd], additional(wrapLabel, purpose))
	if err != nil {
		// One answer for a wrong key, a changed byte and a mismatched purpose. Telling them
		// apart would tell somebody probing which of their guesses was closer.
		return secret.Secret{}, port.NotAuthentic()
	}

	dataGCM, err := gcm(dataKey)
	if err != nil {
		return secret.Secret{}, err
	}
	plaintext, err := dataGCM.Open(
		nil, body[dataKeyEnd:valueOffset], body[valueOffset:], additional(dataLabel, purpose))
	if err != nil {
		return secret.Secret{}, port.NotAuthentic()
	}
	return secret.New(string(plaintext)), nil
}

// gcm is the one place a cipher is constructed, so a wrong key length is one error rather than
// four.
func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, shared.ErrInternal.
			WithDetail("crypto.key_unusable").
			WithCause(fmt.Errorf("constructing the cipher: %w", err))
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, shared.ErrInternal.
			WithDetail("crypto.key_unusable").
			WithCause(fmt.Errorf("constructing GCM: %w", err))
	}
	return aead, nil
}

// additional builds the authenticated-but-not-encrypted half: a label that says which of the two
// layers this is, and the caller's purpose.
func additional(label string, purpose port.Purpose) []byte {
	return []byte(label + string(purpose))
}
