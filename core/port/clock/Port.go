// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package clock is the port for the one thing that makes otherwise pure code untestable: the
// current time.
//
// The domain and the application layer never call time.Now (CLAUDE.md rule 4, arc42 §8.13). A
// use case that reads the clock directly cannot be tested at a DST boundary, at an expiry
// boundary, or on 29 February - and those are exactly the cases that produce the bug reports.
//
// RandomSource is the third of the trio arc42 §8.13 names. It arrives with the first code that
// needs it - the assignment strategies; declaring an interface nobody implements would only be a
// second thing to keep in step.
package clock

import (
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
