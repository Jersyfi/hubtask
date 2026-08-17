// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// HLC is a hybrid logical clock: physical time, a counter, and the device that stamped it
// (offline-sync.md §4.1).
//
// Device clocks are wrong, sometimes by hours. "The latest timestamp wins" therefore lets a device
// with a fast clock outvote every other one permanently, which is why every field change carries
// one of these instead of a bare timestamp: the counter orders changes made within the same
// millisecond, and the device identifier breaks the remaining ties the same way on every device,
// so two clients merging the same pair of changes reach the same result.
//
// The textual form is `<physical>:<counter>:<device>`, with the first two zero-padded so that
// comparing two of them as strings compares them as clocks. The database stores it as text
// (db/schema.sql, change_log.hlc), and an index on that column then sorts in clock order.
type HLC struct {
	// Physical is truncated to milliseconds. Finer resolution would be a promise the counter is
	// there to keep instead, and it would differ between platforms.
	Physical time.Time
	Counter  uint32
	// Device is the origin: a client device, or the installation itself for a server-side change.
	Device string
}

const (
	// hlcPhysicalDigits holds milliseconds since the epoch until the year 2286.
	hlcPhysicalDigits = 13
	// hlcCounterDigits bounds how many changes one device can stamp inside one millisecond before
	// the clock moves on by itself.
	hlcCounterDigits = 5
	hlcMaxCounter    = 99999
)

// NewHLC builds a clock reading. It is the only way to get one with a device that cannot be
// parsed back.
func NewHLC(physical time.Time, counter uint32, device string) (HLC, error) {
	switch {
	case physical.IsZero():
		return HLC{}, ErrInternal.WithDetail("sync.hlc_incomplete")
	case device == "", strings.Contains(device, ":"):
		// A colon in the device would make the text form ambiguous, and an empty device would make
		// two clocks compare equal that are not.
		return HLC{}, ErrInternal.
			WithDetail("sync.hlc_device_malformed").
			WithParams(map[string]string{"device": device})
	case counter > hlcMaxCounter:
		return HLC{}, ErrInternal.WithDetail("sync.hlc_counter_exhausted")
	}
	return HLC{Physical: physical.UTC().Truncate(time.Millisecond), Counter: counter, Device: device}, nil
}

// Tick returns the next reading of this clock for the given wall time.
//
// The physical part only ever moves forward: a wall clock that jumped backwards - an NTP
// correction, a suspended laptop - would otherwise produce a change that sorts before one already
// recorded, and the change would be silently discarded on merge. When time has not moved on, the
// counter does; when the counter is exhausted, the clock borrows a millisecond from the future,
// which is the standard way out and is bounded by how fast a device can produce changes.
func (h HLC) Tick(now time.Time, device string) (HLC, error) {
	now = now.UTC().Truncate(time.Millisecond)

	if h.Physical.IsZero() || now.After(h.Physical) {
		return NewHLC(now, 0, device)
	}
	if h.Counter >= hlcMaxCounter {
		return NewHLC(h.Physical.Add(time.Millisecond), 0, device)
	}
	return NewHLC(h.Physical, h.Counter+1, device)
}

// String is the stored and transmitted form.
func (h HLC) String() string {
	if h.Physical.IsZero() {
		return ""
	}
	return fmt.Sprintf("%0*d:%0*d:%s",
		hlcPhysicalDigits, h.Physical.UnixMilli(), hlcCounterDigits, h.Counter, h.Device)
}

// IsZero reports the absent reading.
func (h HLC) IsZero() bool { return h.Physical.IsZero() }

// ParseHLC reads the textual form. It is what a client's mutation arrives as, so an unusable value
// is input rather than a defect - the caller decides what to do with a client that sends nonsense
// (offline-sync.md §3.2).
func ParseHLC(raw string) (HLC, error) {
	malformed := ErrValidation.
		WithDetail("sync.hlc_malformed").
		WithParams(map[string]string{"value": raw})

	physical, rest, found := strings.Cut(raw, ":")
	if !found {
		return HLC{}, malformed
	}
	counter, device, found := strings.Cut(rest, ":")
	if !found || device == "" {
		return HLC{}, malformed
	}

	milliseconds, err := strconv.ParseInt(physical, 10, 64)
	if err != nil || milliseconds < 0 {
		return HLC{}, malformed
	}
	ticks, err := strconv.ParseUint(counter, 10, 32)
	if err != nil || ticks > hlcMaxCounter {
		return HLC{}, malformed
	}

	return HLC{
		Physical: time.UnixMilli(milliseconds).UTC(),
		Counter:  uint32(ticks),
		Device:   device,
	}, nil
}

// Compare orders two readings: physical time first, then the counter, then the device.
//
// The device is part of the order rather than a tie-break left to chance. Two devices that stamped
// a change in the same millisecond with the same counter have to be ordered the same way
// everywhere, or the two of them converge on different results - which is the one failure a
// merge rule must not have.
func (h HLC) Compare(other HLC) int {
	if !h.Physical.Equal(other.Physical) {
		if h.Physical.Before(other.Physical) {
			return -1
		}
		return 1
	}
	if h.Counter != other.Counter {
		if h.Counter < other.Counter {
			return -1
		}
		return 1
	}
	return strings.Compare(h.Device, other.Device)
}

// After reports whether this reading wins a last-writer-wins comparison against the other one.
func (h HLC) After(other HLC) bool { return h.Compare(other) > 0 }
