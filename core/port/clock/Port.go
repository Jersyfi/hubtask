// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package clock is the port for the one thing that makes otherwise pure code untestable: the
// current time.
//
// The domain and the application layer never call time.Now (CLAUDE.md rule 4, arc42 §8.13). A
// use case that reads the clock directly cannot be tested at a DST boundary, at an expiry
// boundary, or on 29 February - and those are exactly the cases that produce the bug reports.
//
// IDGenerator and RandomSource are the other two of the trio arc42 §8.13 names. They arrive with
// the first code that needs them; declaring an interface nobody implements would only be a
// second thing to keep in step.
package clock

import "time"

// Clock is the current time. Implementations return UTC - a local time zone in a comparison is a
// bug waiting for a summer.
type Clock interface {
	Now() time.Time
}

// Fixed is a clock that does not move. It is here rather than in a test helper package because
// every layer's tests need it, and a second copy per package is a second thing to get wrong.
type Fixed time.Time

func (f Fixed) Now() time.Time { return time.Time(f) }
