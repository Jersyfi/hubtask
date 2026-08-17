// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package clock is the adapter behind the clock port. It is three lines because that is the whole
// point: the untestable part of "what time is it" is confined to one place, and everything above
// it takes the answer as a parameter (arc42 §8.13).
package clock

import (
	"time"

	port "github.com/Jersyfi/hubtask/core/port/clock"
)

// System reads the machine's clock and answers in UTC. Always UTC: a comparison against a stored
// timestamptz in a local zone is a bug that only appears twice a year.
type System struct{}

var _ port.Clock = System{}

func (System) Now() time.Time { return time.Now().UTC() }
