// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package crypto

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The password cost of security.md §5: Argon2id, m=64 MiB, t=3, p=2 - starting values, reviewed
// yearly. Different from the backup derivation's p=4 deliberately: that one derives rarely on a
// machine told to hurry, this one runs on every sign-in of every person at once, and two lanes
// bound what a burst of sign-ins can demand of the scheduler.
//
// Raising them later is safe and expected: every hash stores the numbers it was computed under,
// so an old hash verifies under its own cost until the next successful sign-in re-hashes it.
const (
	passwordPasses      = 3
	passwordMemoryKiB   = 64 * 1024
	passwordParallelism = 2
	passwordKeyBytes    = 32
	passwordSaltBytes   = 16
)

// The ceilings a stored hash's parameters are held to when verifying, checkDerivation's
// reasoning: the parameters travel in the hash string, and the hash string comes from a database
// column - honest today, but a bound costs nothing and an unbounded allocation per guess would
// hand a tampered row a denial of service.
const (
	maxPasswordMemoryKiB   = 1024 * 1024
	maxPasswordPasses      = 16
	maxPasswordParallelism = 16
)

// Passwords hashes and verifies the local accounts' credential (ADR-0005, T-02).
//
// The stored form is the PHC string the reference implementation defines -
// $argon2id$v=19$m=…,t=…,p=…$salt$hash - because it carries its own parameters: verification
// reads the cost from the hash, so raising this build's numbers leaves every stored password
// verifiable.
type Passwords struct {
	entropy clockport.Entropy
	// decoy is a real hash of a random secret, computed once at construction. Verification
	// against an account that does not exist or holds no password runs against it, so the two
	// refusals cost the same work and answer in the same shape (T-02's constant shape).
	decoy string
}

// NewPasswords draws the decoy. The error path exists because construction draws randomness, and
// a process that cannot draw randomness must not start pretending it can verify credentials.
func NewPasswords(entropy clockport.Entropy) (Passwords, error) {
	p := Passwords{entropy: entropy}
	filler, err := entropy.Bytes(passwordKeyBytes)
	if err != nil {
		return Passwords{}, shared.ErrUnavailable.
			WithDetail("crypto.entropy_unavailable").
			WithCause(fmt.Errorf("drawing the decoy secret: %w", err))
	}
	decoy, err := p.Hash(secret.New(base64.RawStdEncoding.EncodeToString(filler)))
	if err != nil {
		return Passwords{}, err
	}
	p.decoy = decoy
	return p, nil
}

// Hash computes the stored form under this build's cost, with a fresh salt.
func (p Passwords) Hash(password secret.Secret) (string, error) {
	salt, err := p.entropy.Bytes(passwordSaltBytes)
	if err != nil {
		return "", shared.ErrUnavailable.
			WithDetail("crypto.entropy_unavailable").
			WithCause(fmt.Errorf("drawing a password salt: %w", err))
	}

	key := argon2.IDKey([]byte(password.Reveal()), salt,
		passwordPasses, passwordMemoryKiB, passwordParallelism, passwordKeyBytes)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, passwordMemoryKiB, passwordPasses, passwordParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify compares a presented password against a stored hash, in constant time over the derived
// key. It answers false rather than an error for a wrong password, because "wrong" is an answer
// and not a failure; the error path is for a stored value this process cannot read.
func (p Passwords) Verify(stored string, password secret.Secret) (bool, error) {
	salt, key, passes, memoryKiB, parallelism, err := parsePasswordHash(stored)
	if err != nil {
		return false, err
	}

	derived := argon2.IDKey([]byte(password.Reveal()), salt,
		passes, memoryKiB, parallelism, uint32(len(key))) //nolint:gosec // G115: the key length was parsed from a bounded hash
	return subtle.ConstantTimeCompare(derived, key) == 1, nil
}

// VerifyDecoy burns the same work a real verification would, against the construction-time decoy.
// The answer is always false; the point is that "no such account" takes as long as "wrong
// password" and exercises the same code (T-02).
func (p Passwords) VerifyDecoy(password secret.Secret) {
	// The result is discarded and the error cannot occur: the decoy was written by Hash.
	_, _ = p.Verify(p.decoy, password)
}

// parsePasswordHash reads the PHC string. Everything wrong with it is one internal error: the
// value comes from this system's own column, so a shape this process cannot read is a defect,
// never a caller's mistake - and never a distinguishable answer on the sign-in path.
func parsePasswordHash(stored string) (salt, key []byte, passes, memoryKiB uint32, parallelism uint8, err error) {
	fail := func(cause string) error {
		return shared.ErrInternal.
			WithDetail("crypto.password_hash_unreadable").
			WithCause(fmt.Errorf("parsing the stored hash: %s", cause))
	}

	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return nil, nil, 0, 0, 0, fail("not an argon2id hash")
	}

	var version int
	if _, scanErr := fmt.Sscanf(parts[2], "v=%d", &version); scanErr != nil || version != argon2.Version {
		return nil, nil, 0, 0, 0, fail("unknown version")
	}

	var memory, iterations, lanes uint32
	if _, scanErr := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &lanes); scanErr != nil {
		return nil, nil, 0, 0, 0, fail("unreadable cost")
	}
	if memory == 0 || memory > maxPasswordMemoryKiB ||
		iterations == 0 || iterations > maxPasswordPasses ||
		lanes == 0 || lanes > maxPasswordParallelism {
		return nil, nil, 0, 0, 0, fail("cost out of bounds")
	}

	salt, saltErr := base64.RawStdEncoding.DecodeString(parts[4])
	key, keyErr := base64.RawStdEncoding.DecodeString(parts[5])
	if saltErr != nil || keyErr != nil || len(salt) == 0 || len(key) == 0 || len(key) > 64 {
		return nil, nil, 0, 0, 0, fail("unreadable salt or key")
	}

	return salt, key, iterations, memory, uint8(lanes), nil
}
