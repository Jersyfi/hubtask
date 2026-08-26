// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package clock

import (
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math/big"

	port "github.com/Jersyfi/hubtask/core/port/clock"
)

// CryptoRandom answers the RandomSource port from the operating system's entropy.
//
// crypto/rand rather than a seeded generator, and the standard library's rejection sampling
// rather than a modulo of our own: security.md §8 forbids home-grown randomness, and an
// assignment that could be predicted from the previous one would make "who gets this task" a
// thing an insider can steer.
type CryptoRandom struct{}

var (
	_ port.RandomSource = CryptoRandom{}
	_ port.Entropy      = CryptoRandom{}
)

// IntN returns a uniform value in [0, n). n must be positive - the port's contract, and
// crypto/rand.Int refuses anything else.
func (CryptoRandom) IntN(n int) int {
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(n)))
	if err != nil {
		// crypto/rand's Reader is documented never to fail since Go 1.24; a machine whose
		// entropy source is broken cannot serve requests anyway (the reasoning of UUIDv7).
		panic(err)
	}
	return int(value.Int64())
}

// Bytes draws a credential's worth of entropy. crypto/rand again, and the error is returned
// rather than panicked: a token that could not be minted is a request that fails, which is a
// better answer than a process that stops serving everybody else's.
func (CryptoRandom) Bytes(n int) ([]byte, error) {
	if n <= 0 {
		return nil, errors.New("clock: entropy of a non-positive length")
	}
	drawn := make([]byte, n)
	if _, err := cryptorand.Read(drawn); err != nil {
		return nil, fmt.Errorf("drawing %d bytes of entropy: %w", n, err)
	}
	return drawn, nil
}
