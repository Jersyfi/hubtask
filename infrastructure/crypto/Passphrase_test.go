// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package crypto_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	port "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
)

// A derivation costs 64 mebibytes and three passes by design, so this file derives a handful of
// times rather than in a loop. What is under test is the shape of the answer, not the cipher -
// Argon2id itself is x/crypto's to prove.

// cheap is the cost the tests that are about the plumbing rather than about the cost use. It is
// deliberately not the build's setting: a test that took the real one everywhere would spend a
// second on arithmetic nobody is checking.
func cheap(salt []byte) port.Derivation {
	return port.Derivation{
		Salt: salt, Passes: 1, MemoryKiB: 64, Parallelism: 1, KeyLength: 32,
	}
}

func TestTheSamePassphraseAndSaltGiveTheSameKey(t *testing.T) {
	deriver := crypto.NewPassphrase(clockadapter.CryptoRandom{})
	from := cheap([]byte("0123456789abcdef"))

	first, err := deriver.Derive(secret.New("correct horse battery staple"), from)
	if err != nil {
		t.Fatalf("deriving: %v", err)
	}
	second, err := deriver.Derive(secret.New("correct horse battery staple"), from)
	if err != nil {
		t.Fatalf("deriving again: %v", err)
	}

	if !first.Equal(second) {
		t.Fatal("the same passphrase and salt gave two different keys")
	}
	if first.Len() != 32 {
		t.Fatalf("the key is %d bytes", first.Len())
	}
	// The key must not be the passphrase, which is the mistake a placeholder implementation makes.
	if bytes.Contains(first.Reveal(), []byte("correct horse")) {
		t.Fatal("the key contains the passphrase")
	}
}

func TestADifferentSaltGivesADifferentKey(t *testing.T) {
	deriver := crypto.NewPassphrase(clockadapter.CryptoRandom{})

	first, err := deriver.Derive(secret.New("the same passphrase"), cheap([]byte("0123456789abcdef")))
	if err != nil {
		t.Fatalf("deriving: %v", err)
	}
	second, err := deriver.Derive(secret.New("the same passphrase"), cheap([]byte("fedcba9876543210")))
	if err != nil {
		t.Fatalf("deriving: %v", err)
	}

	if first.Equal(second) {
		t.Fatal("two targets with the same passphrase share a key - cracking one cracks both")
	}
}

// A fresh salt every time, and this build's cost stated rather than assumed. The cost is in the
// derivation so that an archive written today opens after the constants are raised.
func TestANewDerivationDrawsAFreshSaltAndStatesTheCost(t *testing.T) {
	deriver := crypto.NewPassphrase(clockadapter.CryptoRandom{})

	first, err := deriver.NewDerivation()
	if err != nil {
		t.Fatalf("drawing a derivation: %v", err)
	}
	second, err := deriver.NewDerivation()
	if err != nil {
		t.Fatalf("drawing a second: %v", err)
	}

	switch {
	case first.IsZero():
		t.Fatal("the derivation carries no salt")
	case len(first.Salt) != crypto.DerivationSaltBytes:
		t.Fatalf("the salt is %d bytes", len(first.Salt))
	case bytes.Equal(first.Salt, second.Salt):
		t.Fatal("two derivations drew the same salt")
	case first.Passes != crypto.DerivationPasses:
		t.Fatalf("passes %d", first.Passes)
	case first.MemoryKiB != crypto.DerivationMemoryKiB:
		t.Fatalf("memory %d KiB", first.MemoryKiB)
	case first.Parallelism != crypto.DerivationParallelism:
		t.Fatalf("parallelism %d", first.Parallelism)
	case first.KeyLength != crypto.DerivationKeyBytes:
		t.Fatalf("key length %d", first.KeyLength)
	}
}

// This build's own parameters have to be inside the bounds a derivation read back from an archive
// is held to. If they ever are not, this build writes archives it will refuse to open.
func TestThisBuildsOwnCostIsAcceptedBack(t *testing.T) {
	deriver := crypto.NewPassphrase(clockadapter.CryptoRandom{})

	from, err := deriver.NewDerivation()
	if err != nil {
		t.Fatalf("drawing a derivation: %v", err)
	}
	if _, err := deriver.Derive(secret.New("a passphrase"), from); err != nil {
		t.Fatalf("this build refuses its own cost: %v", err)
	}
}

// The parameters come out of a manifest, and a manifest comes from wherever the archive came
// from. A line in somebody else's file must not be able to ask this process for a gibibyte per
// attempt.
func TestADerivationFromAnArchiveIsHeldToBounds(t *testing.T) {
	deriver := crypto.NewPassphrase(clockadapter.CryptoRandom{})
	salt := []byte("0123456789abcdef")

	cases := map[string]port.Derivation{
		"no salt": {Passes: 1, MemoryKiB: 64, Parallelism: 1, KeyLength: 32},
		"a salt too short to be one": {
			Salt: []byte("abc"), Passes: 1, MemoryKiB: 64, Parallelism: 1, KeyLength: 32,
		},
		"no passes": {Salt: salt, MemoryKiB: 64, Parallelism: 1, KeyLength: 32},
		"more passes than anybody writes": {
			Salt: salt, Passes: 1_000, MemoryKiB: 64, Parallelism: 1, KeyLength: 32,
		},
		"a gibibyte and a half": {
			Salt: salt, Passes: 1, MemoryKiB: 1_600_000, Parallelism: 1, KeyLength: 32,
		},
		"no parallelism": {Salt: salt, Passes: 1, MemoryKiB: 64, KeyLength: 32},
		"a key too short to be one": {
			Salt: salt, Passes: 1, MemoryKiB: 64, Parallelism: 1, KeyLength: 8,
		},
		"a key longer than any cipher takes": {
			Salt: salt, Passes: 1, MemoryKiB: 64, Parallelism: 1, KeyLength: 4_096,
		},
	}

	for name, from := range cases {
		t.Run(name, func(t *testing.T) {
			key, err := deriver.Derive(secret.New("a passphrase"), from)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("accepted: %v", err)
			}
			if code := shared.AsError(err).DetailCode; code != "crypto.derivation_unusable" {
				t.Fatalf("detail code %q", code)
			}
			if !key.IsEmpty() {
				t.Fatal("a refused derivation returned a key")
			}
		})
	}
}

func TestDerivingWithoutAPassphraseIsRefused(t *testing.T) {
	deriver := crypto.NewPassphrase(clockadapter.CryptoRandom{})

	key, err := deriver.Derive(secret.Secret{}, cheap([]byte("0123456789abcdef")))
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("deriving from nothing: %v", err)
	}
	if code := shared.AsError(err).DetailCode; code != port.CodePassphraseRequired {
		t.Fatalf("detail code %q", code)
	}
	if !key.IsEmpty() {
		t.Fatal("a refused derivation returned a key")
	}
}

func TestAnUnusableEntropySourceStopsADerivation(t *testing.T) {
	deriver := crypto.NewPassphrase(exhausted{})

	if _, err := deriver.NewDerivation(); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("drawing a salt with no entropy: %v", err)
	}
}

var _ clockport.Entropy = clockadapter.CryptoRandom{}
