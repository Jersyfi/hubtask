// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package crypto

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The port carries no cryptography, so there is nothing here to measure - only something to hold
// in place. The double proves both interfaces can be implemented by a fake, which is what the
// tests of everything above them depend on.
type double struct{}

func (double) Seal(context.Context, secret.Secret, Purpose) (Sealed, error) {
	return Sealed{}, shared.ErrUnavailable.WithDetail(CodeNoEncryptionKey)
}

func (double) Open(context.Context, Sealed, Purpose) (secret.Secret, error) {
	return secret.Secret{}, NotAuthentic()
}

func (double) ActiveKeyID() string { return "" }

func (double) KeyIDs() []string { return nil }

func (double) Rewrap(context.Context, Sealed, Purpose) (Sealed, error) {
	return Sealed{}, shared.ErrUnavailable.WithDetail(CodeNoEncryptionKey)
}

func (double) NewDerivation() (Derivation, error) { return Derivation{}, nil }

func (double) Derive(secret.Secret, Derivation) (secret.Bytes, error) {
	return secret.Bytes{}, shared.ErrValidation.WithDetail(CodePassphraseRequired)
}

var (
	_ Encryptor  = double{}
	_ KeyDeriver = double{}
)

// A failed open is a refusal and never a value. The type of the answer is what stops a caller
// treating "could not decrypt" as "decrypted to nothing", which for a credential would be a
// connection attempted with an empty password.
func TestAFailedOpenAnswersNoPlaintext(t *testing.T) {
	plaintext, err := (double{}).Open(t.Context(), Sealed{KeyID: "a"}, "test")
	if err == nil {
		t.Fatal("a failed open reported success")
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a failed open is reported as %v", err)
	}
	if !plaintext.IsEmpty() {
		t.Error("a failed open returned a plaintext")
	}
}

// An installation with no key refuses rather than writing plaintext, and the refusal is
// distinguishable from a value that would not open - the two need different answers from an
// operator.
func TestSealingWithoutAKeyIsItsOwnRefusal(t *testing.T) {
	_, err := (double{}).Seal(t.Context(), secret.New("hunter2"), "test")
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("sealing without a key is reported as %v", err)
	}
	if shared.AsError(err).DetailCode != CodeNoEncryptionKey {
		t.Errorf("detail code %q", shared.AsError(err).DetailCode)
	}
	if shared.AsError(err).DetailCode == CodeNotAuthentic {
		t.Error("a missing key reads as a value that would not open")
	}
}

func TestTheZeroValuesSayTheyAreEmpty(t *testing.T) {
	if !(Sealed{}).IsZero() {
		t.Error("the zero ciphertext does not say it is empty")
	}
	if (Sealed{KeyID: "a", Ciphertext: []byte{1}}).IsZero() {
		t.Error("a sealed value says it is empty")
	}
	if !(Derivation{}).IsZero() {
		t.Error("the zero derivation does not say it is empty")
	}
	if (Derivation{Salt: []byte{1}}).IsZero() {
		t.Error("a derivation with a salt says it is empty")
	}
}
