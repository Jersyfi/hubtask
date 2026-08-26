// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package crypto_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	port "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
)

// Two keys, long enough to be keys, different enough that a value sealed under one visibly does
// not open under the other.
const (
	materialA = "the first master key of this installation"
	materialB = "the second master key of this installation"
)

const purpose = port.Purpose("backup_target.credential:0192f000-0000-7000-8000-00000000000a")

func ring(t *testing.T, entries ...crypto.KeyMaterial) crypto.Keyring {
	t.Helper()
	built, err := crypto.NewKeyring(entries)
	if err != nil {
		t.Fatalf("building the keyring: %v", err)
	}
	return built
}

func key(id, material string) crypto.KeyMaterial {
	return crypto.KeyMaterial{ID: id, Material: secret.New(material)}
}

// envelope over the real entropy source, which is what production uses and what the nonce test
// needs: a fixed source would make two seals identical and prove nothing.
func envelope(t *testing.T, entries ...crypto.KeyMaterial) crypto.Envelope {
	t.Helper()
	return crypto.NewEnvelope(ring(t, entries...), clockadapter.CryptoRandom{})
}

func TestAValueRoundTrips(t *testing.T) {
	sealer := envelope(t, key("a", materialA))

	sealed, err := sealer.Seal(t.Context(), secret.New("s3-secret-access-key"), purpose)
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}
	if sealed.KeyID != "a" {
		t.Fatalf("sealed under %q", sealed.KeyID)
	}
	// The plaintext must not be findable in the ciphertext, which is the one property a reader
	// of a database dump checks first.
	if bytes.Contains(sealed.Ciphertext, []byte("s3-secret-access-key")) {
		t.Fatal("the plaintext is in the ciphertext")
	}

	opened, err := sealer.Open(t.Context(), sealed, purpose)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if opened.Reveal() != "s3-secret-access-key" {
		t.Fatalf("opened to something else")
	}
}

// The acceptance criterion of the task: a value sealed with `a` and then rotated to `b` still
// opens with the `a` recorded in its key identifier, and a value sealed with `b` does not open
// with `a` alone.
func TestARotationLeavesOldValuesReadable(t *testing.T) {
	before := envelope(t, key("a", materialA))
	sealedUnderA, err := before.Seal(t.Context(), secret.New("older secret"), purpose)
	if err != nil {
		t.Fatalf("sealing under a: %v", err)
	}

	// The rotation: b becomes current, a stays readable. No data is rewritten.
	after := envelope(t, key("b", materialB), key("a", materialA))
	if after.ActiveKeyID() != "b" {
		t.Fatalf("the current key is %q", after.ActiveKeyID())
	}

	opened, err := after.Open(t.Context(), sealedUnderA, purpose)
	if err != nil {
		t.Fatalf("the rotated installation could not open a value sealed before it: %v", err)
	}
	if opened.Reveal() != "older secret" {
		t.Fatal("the value opened to something else")
	}

	sealedUnderB, err := after.Seal(t.Context(), secret.New("newer secret"), purpose)
	if err != nil {
		t.Fatalf("sealing under b: %v", err)
	}
	if sealedUnderB.KeyID != "b" {
		t.Fatalf("a value sealed after the rotation names %q", sealedUnderB.KeyID)
	}

	// And the other direction, which is what makes the rotation worth anything: an installation
	// that only holds the old key cannot read what the new one wrote.
	_, err = before.Open(t.Context(), sealedUnderB, purpose)
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("the old installation read a value sealed under the new key: %v", err)
	}
	// And it says which key it is missing, because that is the one thing an operator can act on.
	if code := shared.AsError(err).DetailCode; code != port.CodeUnknownKey {
		t.Fatalf("detail code %q, want %s", code, port.CodeUnknownKey)
	}
	if named := shared.AsError(err).Params["key_id"]; named != "b" {
		t.Fatalf("the refusal named key %q", named)
	}
}

// A key that is present but wrong: the identifier resolves, and the value still does not open.
func TestTheWrongKeyFailsWithoutAPartialPlaintext(t *testing.T) {
	sealed, err := envelope(t, key("a", materialA)).
		Seal(t.Context(), secret.New("the value"), purpose)
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	// The same identifier, different material - an operator who restored the wrong secret file.
	impostor := envelope(t, key("a", materialB))
	plaintext, err := impostor.Open(t.Context(), sealed, purpose)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("opening with the wrong material: %v", err)
	}
	if shared.AsError(err).DetailCode != port.CodeNotAuthentic {
		t.Fatalf("detail code %q", shared.AsError(err).DetailCode)
	}
	if !plaintext.IsEmpty() {
		t.Fatal("a failed open returned a plaintext")
	}
}

// A changed byte anywhere is the same answer, and none of them is a partial plaintext. Tampering
// is not distinguishable from a wrong key on purpose.
func TestEveryTamperedByteIsRefused(t *testing.T) {
	sealer := envelope(t, key("a", materialA))
	sealed, err := sealer.Seal(t.Context(), secret.New("the value"), purpose)
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	for position := range sealed.Ciphertext {
		tampered := port.Sealed{KeyID: sealed.KeyID, Ciphertext: bytes.Clone(sealed.Ciphertext)}
		tampered.Ciphertext[position] ^= 0x01

		plaintext, err := sealer.Open(t.Context(), tampered, purpose)
		if err == nil {
			t.Fatalf("a value with byte %d changed opened anyway", position)
		}
		if !plaintext.IsEmpty() {
			t.Fatalf("byte %d changed and a plaintext came back", position)
		}
	}
}

// The purpose is authenticated, so a ciphertext lifted out of one row does not open in another.
func TestACiphertextDoesNotTravelBetweenRows(t *testing.T) {
	sealer := envelope(t, key("a", materialA))
	sealed, err := sealer.Seal(t.Context(), secret.New("the value"), purpose)
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}

	elsewhere := port.Purpose("backup_target.credential:0192f000-0000-7000-8000-00000000000b")
	if _, err := sealer.Open(t.Context(), sealed, elsewhere); err == nil {
		t.Fatal("the ciphertext opened under another row's purpose")
	}
}

// Identical plaintexts must not produce identical ciphertexts: two targets configured with the
// same password must not be visibly the same in a database dump.
func TestIdenticalPlaintextsSealDifferently(t *testing.T) {
	sealer := envelope(t, key("a", materialA))

	seen := map[string]bool{}
	for round := range 32 {
		sealed, err := sealer.Seal(t.Context(), secret.New("the same password"), purpose)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if seen[string(sealed.Ciphertext)] {
			t.Fatalf("round %d repeated a ciphertext - a nonce was reused", round)
		}
		seen[string(sealed.Ciphertext)] = true
	}
}

// An installation with no key refuses to seal rather than writing a plaintext into a column whose
// name says otherwise.
func TestAnInstallationWithNoKeyRefusesToSeal(t *testing.T) {
	sealer := envelope(t)

	if sealer.ActiveKeyID() != "" {
		t.Fatalf("an empty ring named %q as current", sealer.ActiveKeyID())
	}
	_, err := sealer.Seal(t.Context(), secret.New("the value"), purpose)
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("sealing without a key: %v", err)
	}
	if shared.AsError(err).DetailCode != port.CodeNoEncryptionKey {
		t.Fatalf("detail code %q", shared.AsError(err).DetailCode)
	}
}

// The three shapes that are not a value of ours at all. Their own answer, because "this build
// cannot read that" sends an operator somewhere else than "that is the wrong key".
func TestAMalformedCiphertextIsItsOwnAnswer(t *testing.T) {
	sealer := envelope(t, key("a", materialA))

	cases := map[string]port.Sealed{
		"no key named": {Ciphertext: []byte{1, 2, 3}},
		"too short":    {KeyID: "a", Ciphertext: []byte{1, 2, 3}},
		"a version this build does not know": {
			KeyID: "a", Ciphertext: append([]byte{9}, make([]byte, 128)...),
		},
	}
	for name, sealed := range cases {
		_, err := sealer.Open(t.Context(), sealed, purpose)
		if !errors.Is(err, shared.ErrValidation) {
			t.Fatalf("%s: %v", name, err)
		}
		if code := shared.AsError(err).DetailCode; code != port.CodeCiphertextMalformed {
			t.Fatalf("%s: detail code %q", name, code)
		}
	}
}

// Threat T-18, at the boundary this package is: nothing that prints a key, a ring or a sealed
// value may contain the material.
func TestNothingHerePrintsKeyMaterial(t *testing.T) {
	built := ring(t, key("a", materialA), key("b", materialB))

	printed := strings.Join([]string{
		formatted(built), formatted(built.KeyIDs()), formatted(envelope(t, key("a", materialA))),
	}, " ")
	for _, material := range []string{materialA, materialB, "master key of this"} {
		if strings.Contains(printed, material) {
			t.Fatalf("printing the keyring leaked %q: %s", material, printed)
		}
	}
	// The identifiers are not secret and have to be printable: "which keys does this process
	// hold" is the first question asked when something will not open.
	if !strings.Contains(formatted(built.KeyIDs()), "a") {
		t.Error("the key identifiers cannot be printed at all")
	}
}

// The entropy source failing is an error rather than a key of zeroes.
func TestAnUnusableEntropySourceIsAnError(t *testing.T) {
	sealer := crypto.NewEnvelope(ring(t, key("a", materialA)), exhausted{})

	if _, err := sealer.Seal(t.Context(), secret.New("the value"), purpose); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("sealing with no entropy: %v", err)
	}
}

type exhausted struct{}

func (exhausted) Bytes(int) ([]byte, error) { return nil, errors.New("no entropy") }

var _ clockport.Entropy = exhausted{}

// formatted prints a value the way a careless log line would.
func formatted(value any) string {
	return fmt.Sprintf("%v %+v %#v", value, value, value)
}

// The bound on what may be sealed. Everything this envelope protects is a small value somebody
// configured, and the ceiling is what keeps the buffer arithmetic provably safe as well as what
// keeps a column from being handed a megabyte of "credential".
func TestAValueTooLargeToBeACredentialIsRefused(t *testing.T) {
	sealer := envelope(t, key("a", materialA))

	oversized := secret.New(strings.Repeat("a", (1<<20)+1))
	_, err := sealer.Seal(t.Context(), oversized, purpose)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a value larger than the limit was sealed: %v", err)
	}
	if code := shared.AsError(err).DetailCode; code != "crypto.value_too_large" {
		t.Fatalf("detail code %q", code)
	}

	// And a value at the limit still goes through: the bound is a ceiling, not a margin.
	atTheLimit := secret.New(strings.Repeat("a", 1<<20))
	if _, err := sealer.Seal(t.Context(), atTheLimit, purpose); err != nil {
		t.Fatalf("a value at the limit was refused: %v", err)
	}
}

// The derived key: same purpose, same key, every time - and a different one for every purpose.
func TestADerivedKeyIsStableAndBoundToItsPurpose(t *testing.T) {
	sealer := crypto.NewEnvelope(ring(t, key("a", materialA)), clockadapter.CryptoRandom{})

	first, err := sealer.DeriveFromMaster(t.Context(), "backup_target.archive:one", 32)
	if err != nil {
		t.Fatalf("deriving: %v", err)
	}
	again, err := sealer.DeriveFromMaster(t.Context(), "backup_target.archive:one", 32)
	if err != nil {
		t.Fatalf("deriving: %v", err)
	}
	other, err := sealer.DeriveFromMaster(t.Context(), "backup_target.archive:two", 32)
	if err != nil {
		t.Fatalf("deriving: %v", err)
	}

	switch {
	case first.Key.Len() != 32:
		t.Fatalf("a key of %d bytes", first.Key.Len())
	case !first.Key.Equal(again.Key):
		t.Fatal("the same purpose gave two different keys")
	case first.Key.Equal(other.Key):
		t.Fatal("two targets share a key")
	case first.KeyID != "a":
		t.Fatalf("the key names %q", first.KeyID)
	}
}

// An archive written before a rotation opens with the key its manifest names, which is what
// reproducing from a named master key is for.
func TestAKeyIsReproducedFromTheMasterKeyItsIdentifierNames(t *testing.T) {
	before := crypto.NewEnvelope(ring(t, key("a", materialA)), clockadapter.CryptoRandom{})
	written, err := before.DeriveFromMaster(t.Context(), "backup_target.archive:one", 32)
	if err != nil {
		t.Fatalf("deriving: %v", err)
	}

	// The rotation: a new key is added and the old one is kept.
	after := crypto.NewEnvelope(ring(t, key("b", materialB), key("a", materialA)), clockadapter.CryptoRandom{})

	reproduced, err := after.ReproduceFromMaster(t.Context(), written.KeyID, "backup_target.archive:one", 32)
	if err != nil {
		t.Fatalf("reproducing: %v", err)
	}
	if !reproduced.Equal(written.Key) {
		t.Fatal("the key its manifest names did not come back")
	}

	// And what the installation writes now is a different key, under the new identifier.
	fresh, err := after.DeriveFromMaster(t.Context(), "backup_target.archive:one", 32)
	if err != nil {
		t.Fatalf("deriving: %v", err)
	}
	if fresh.KeyID != "b" || fresh.Key.Equal(written.Key) {
		t.Fatalf("after the rotation the key is %q and unchanged: %v", fresh.KeyID, fresh.Key.Equal(written.Key))
	}
}

func TestAKeyFromAMasterKeyTheInstallationNoLongerHoldsIsUnavailable(t *testing.T) {
	sealer := crypto.NewEnvelope(ring(t, key("b", materialB)), clockadapter.CryptoRandom{})

	_, err := sealer.ReproduceFromMaster(t.Context(), "a", "backup_target.archive:one", 32)
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("a key nobody holds: %v", err)
	}
}

// An installation with no master key refuses rather than deriving a key of zeroes, for the reason
// sealing refuses rather than writing plaintext.
func TestDerivingWithoutAMasterKeyIsRefused(t *testing.T) {
	empty, err := crypto.NewKeyring(nil)
	if err != nil {
		t.Fatalf("an empty ring: %v", err)
	}
	sealer := crypto.NewEnvelope(empty, clockadapter.CryptoRandom{})

	if _, err := sealer.DeriveFromMaster(t.Context(), "backup_target.archive:one", 32); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("deriving with no key: %v", err)
	}
}

func TestALengthNothingNeedsIsRefused(t *testing.T) {
	sealer := crypto.NewEnvelope(ring(t, key("a", materialA)), clockadapter.CryptoRandom{})

	for _, length := range []int{0, -1, 65, 1 << 20} {
		if _, err := sealer.DeriveFromMaster(t.Context(), "backup_target.archive:one", length); err == nil {
			t.Fatalf("a key of %d bytes was derived", length)
		}
	}
}
