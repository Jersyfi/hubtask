// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package crypto

import (
	"fmt"

	"golang.org/x/crypto/argon2"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	port "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The cost this build derives at, and why each number is the number it is.
//
// RFC 9106 §4 gives two recommended parameter sets; these are the second - t=3, m=64 MiB, p=4 -
// which is the one meant for a machine that cannot spare two gibibytes per derivation. A server
// that derives a backup key while also answering requests is exactly that machine, and a
// parameter set an operator would be tempted to lower is worse than a slightly cheaper one they
// leave alone.
//
// Memory is the parameter that matters. Passes and lanes can be bought with more silicon;
// 64 mebibytes per guess cannot, which is what makes Argon2id worth its cost over a hash that
// only counts iterations (backup-restore.md §4).
//
// Raising them later is safe and is expected: every derivation stores the numbers it used, so an
// archive written under these opens under these even after this build has moved on.
const (
	DerivationPasses      = 3
	DerivationMemoryKiB   = 64 * 1024
	DerivationParallelism = 4
	DerivationKeyBytes    = 32
	// DerivationSaltBytes is RFC 9106's recommendation. A salt is not a secret; it exists so that
	// two targets protected by the same passphrase do not share a key, and so that a table
	// computed for one installation is worth nothing against another.
	DerivationSaltBytes = 16
)

// The ceilings a derivation read back from an archive is held to.
//
// The parameters travel in a manifest, and a manifest comes from wherever the archive came from.
// Without a bound, a single line in a file somebody else wrote would ask this process for a
// gibibyte-sized allocation per attempt - a denial of service that needs no credentials and no
// exploit. The bounds are generous enough that any honest archive, including one written by a
// future build with much higher settings, still opens.
const (
	maxDerivationMemoryKiB   = 1024 * 1024 // one gibibyte
	maxDerivationPasses      = 16
	maxDerivationParallelism = 16
	maxDerivationKeyBytes    = 64
	minDerivationKeyBytes    = 16
	maxDerivationSaltBytes   = 64
	minDerivationSaltBytes   = 8
)

// Passphrase derives a key from something a person typed (backup-restore.md §4).
//
// The passphrase is stored nowhere - not here, not in the archive, not in the row that describes
// the target. That is the property the whole feature rests on and the reason
// `BackupTargetCreate.encryption_passphrase` is documented in the contract as not stored: without
// it the backup is useless, which is stated as an unmissable notice at setup rather than softened.
type Passphrase struct {
	entropy clockport.Entropy
}

func NewPassphrase(entropy clockport.Entropy) Passphrase { return Passphrase{entropy: entropy} }

var _ port.KeyDeriver = Passphrase{}

// NewDerivation draws a fresh salt and states this build's cost.
//
// A new salt every time, never a reused one and never one derived from the target's name: two
// targets protected by the same passphrase must not arrive at the same key, or cracking one
// cracks both.
func (p Passphrase) NewDerivation() (port.Derivation, error) {
	salt, err := p.entropy.Bytes(DerivationSaltBytes)
	if err != nil {
		return port.Derivation{}, shared.ErrUnavailable.
			WithDetail("crypto.entropy_unavailable").
			WithCause(fmt.Errorf("drawing a derivation salt: %w", err))
	}
	return port.Derivation{
		Salt:        salt,
		Passes:      DerivationPasses,
		MemoryKiB:   DerivationMemoryKiB,
		Parallelism: DerivationParallelism,
		KeyLength:   DerivationKeyBytes,
	}, nil
}

// Derive computes the key. Same passphrase and same derivation, same key - on any machine, in any
// version, which is what lets an archive written last year open today.
func (p Passphrase) Derive(
	passphrase secret.Secret, from port.Derivation,
) (secret.Bytes, error) {
	if passphrase.IsEmpty() {
		return secret.Bytes{}, shared.ErrValidation.WithDetail(port.CodePassphraseRequired)
	}
	if err := checkDerivation(from); err != nil {
		return secret.Bytes{}, err
	}

	key := argon2.IDKey(
		[]byte(passphrase.Reveal()), from.Salt,
		from.Passes, from.MemoryKiB, from.Parallelism, from.KeyLength)
	return secret.NewBytes(key), nil
}

// checkDerivation holds a set of parameters to what this process is willing to spend on it.
func checkDerivation(from port.Derivation) error {
	unusable := func(parameter string) error {
		return shared.ErrValidation.
			WithDetail("crypto.derivation_unusable").
			WithParams(map[string]string{"parameter": parameter})
	}

	switch {
	case len(from.Salt) < minDerivationSaltBytes, len(from.Salt) > maxDerivationSaltBytes:
		return unusable("salt")
	case from.Passes == 0, from.Passes > maxDerivationPasses:
		return unusable("passes")
	case from.MemoryKiB == 0, from.MemoryKiB > maxDerivationMemoryKiB:
		return unusable("memory")
	case from.Parallelism == 0, from.Parallelism > maxDerivationParallelism:
		return unusable("parallelism")
	case from.KeyLength < minDerivationKeyBytes, from.KeyLength > maxDerivationKeyBytes:
		return unusable("key_length")
	}
	return nil
}
