// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package clock is the port for the one thing that makes otherwise pure code untestable: the
// current time.
//
// The domain and the application layer never call time.Now (CLAUDE.md rule 4, arc42 §8.13). A
// use case that reads the clock directly cannot be tested at a DST boundary, at an expiry
// boundary, or on 29 February - and those are exactly the cases that produce the bug reports.
//
// RandomSource is the third of the trio arc42 §8.13 names. It stayed undeclared until the first
// code that needs it arrived - the assignment strategies (C-02) - because an interface nobody
// implements is only a second thing to keep in step.
package clock

import (
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Clock is the current time. Implementations return UTC - a local time zone in a comparison is a
// bug waiting for a summer.
type Clock interface {
	Now() time.Time
}

// Fixed is a clock that does not move. It is here rather than in a test helper package because
// every layer's tests need it, and a second copy per package is a second thing to get wrong.
type Fixed time.Time

func (f Fixed) Now() time.Time { return time.Time(f) }

// IDGenerator mints identifiers. UUIDv7 in the application rather than in the database
// (arc42 §8.13): time-ordered, so an index stays dense, and known before the insert - which is
// what lets one transaction write a row, an event that references it, and an audit entry about
// it, without a round trip in between.
//
// It is a port because a test that cannot predict the identifiers cannot assert on what was
// written.
type IDGenerator interface {
	NewID() shared.ID
}

// RandomSource is uniform randomness. The random assignment strategies draw through it rather
// than through a generator of their own, so that a table test can say which candidate wins
// (arc42 §8.13) - and so that production draws come from crypto/rand, because "who gets this
// task" must not be predictable from "who got the last one" (security.md §8).
type RandomSource interface {
	// IntN returns a uniform value in [0, n). n must be positive.
	IntN(n int) int
}

// Entropy draws unguessable bytes. Separate from RandomSource, and deliberately: drawing "which
// candidate gets this task" and drawing a credential are different needs with different
// consequences, and one interface with both would put a method on every double that implements
// the other for no reason.
//
// It is the port a minted token's secret half comes through (D-08), so that a test can fix the
// credential it is asserting on and production draws from crypto/rand.
type Entropy interface {
	// Bytes returns n unguessable bytes, or an error if the machine cannot produce them. n must
	// be positive.
	Bytes(n int) ([]byte, error)
}

// FixedEntropy is an entropy source that answers the same bytes every time, counting up from a
// seed. For tests, which is why it lives here beside Fixed and Scripted rather than in a helper
// package every layer would need its own copy of.
type FixedEntropy struct {
	// Seed is the first byte; each following one is the previous plus one.
	Seed byte
}

func (e FixedEntropy) Bytes(n int) ([]byte, error) {
	if n <= 0 {
		return nil, errors.New("clock: entropy of a non-positive length")
	}
	drawn := make([]byte, n)
	for i := range drawn {
		drawn[i] = e.Seed + byte(i)
	}
	return drawn, nil
}

// Scripted is a random source that plays a rehearsed sequence, one value per call, cycling when
// the script runs out. A scripted value outside [0, n) is taken modulo n, so a script stays in
// bounds when the pool it draws from is smaller than the test expected.
//
// It is here rather than in a test helper package for the reason Fixed is: every layer's tests
// need randomness they can predict, and a second copy per package is a second thing to get wrong.
type Scripted struct {
	values []int
	next   int
}

// NewScripted takes the sequence to play. At least one value: a script with nothing to say has no
// answer for IntN.
func NewScripted(values ...int) *Scripted { return &Scripted{values: values} }

func (s *Scripted) IntN(n int) int {
	value := s.values[s.next%len(s.values)]
	s.next++
	return value % n
}

// HLCSource stamps a change with a hybrid logical clock (offline-sync.md §4.1).
//
// It is stateful in a way Clock is not: the counter has to advance when two changes fall in the
// same millisecond, so the last reading is remembered between calls. That state belongs to the
// process, which is why this is a port and not a function - one instance per process, and the
// tests get a source they can drive.
type HLCSource interface {
	// Next returns the reading for the change being written now. Implementations are safe for
	// concurrent use: two requests stamping at once must not receive the same reading.
	Next() shared.HLC
}
