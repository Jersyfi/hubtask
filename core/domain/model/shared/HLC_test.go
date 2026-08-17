// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import (
	"errors"
	"slices"
	"testing"
	"time"
)

var hlcNow = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)

func mustHLC(t *testing.T, physical time.Time, counter uint32, device string) HLC {
	t.Helper()
	clock, err := NewHLC(physical, counter, device)
	if err != nil {
		t.Fatalf("building the clock reading failed: %v", err)
	}
	return clock
}

// The textual form is what the database stores, and it has to sort as a string exactly as the
// clock sorts as a value - otherwise an index on the column is in the wrong order.
func TestTheTextFormSortsLikeTheClock(t *testing.T) {
	readings := []HLC{
		mustHLC(t, hlcNow, 0, "server"),
		mustHLC(t, hlcNow, 1, "server"),
		mustHLC(t, hlcNow, 2, "server"),
		mustHLC(t, hlcNow.Add(time.Millisecond), 0, "server"),
		mustHLC(t, hlcNow.Add(time.Second), 0, "server"),
	}

	texts := make([]string, 0, len(readings))
	for i, reading := range readings {
		texts = append(texts, reading.String())
		if i > 0 && !reading.After(readings[i-1]) {
			t.Errorf("%s does not come after %s", reading, readings[i-1])
		}
	}
	if !slices.IsSorted(texts) {
		t.Errorf("the text forms do not sort in clock order: %v", texts)
	}
}

func TestParseHLCReadsWhatStringWrote(t *testing.T) {
	original := mustHLC(t, hlcNow, 7, "device-a3")

	parsed, err := ParseHLC(original.String())
	if err != nil {
		t.Fatalf("%q could not be read back: %v", original, err)
	}
	if parsed.Compare(original) != 0 || parsed.Device != "device-a3" {
		t.Errorf("read back %+v, want %+v", parsed, original)
	}
}

// A client sends these, so an unusable one is input rather than a defect.
func TestParseHLCRefusesNonsenseAsInput(t *testing.T) {
	for _, raw := range []string{
		"", "1755", "1755:7", "1755:7:", "abc:7:device", "1755:abc:device",
		"-1:0:device", "1755:100000:device",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseHLC(raw); !errors.Is(err, ErrValidation) {
				t.Errorf("%q was accepted or failed as something other than input: %v", raw, err)
			}
		})
	}
}

// The physical part only ever moves forward. A wall clock that jumps backwards - an NTP
// correction, a suspended laptop - would otherwise produce a change that sorts before one already
// recorded, and that change would be discarded on merge without anyone noticing.
func TestTickNeverGoesBackwards(t *testing.T) {
	current := mustHLC(t, hlcNow, 0, "server")

	backwards, err := current.Tick(hlcNow.Add(-time.Hour), "server")
	if err != nil {
		t.Fatalf("the tick failed: %v", err)
	}
	if !backwards.After(current) {
		t.Errorf("%s does not come after %s although the wall clock went back", backwards, current)
	}
	if !backwards.Physical.Equal(current.Physical) {
		t.Errorf("the physical part followed the wall clock backwards: %s", backwards)
	}
}

func TestTickAdvances(t *testing.T) {
	t.Run("the counter within one millisecond", func(t *testing.T) {
		first := mustHLC(t, hlcNow, 0, "server")

		second, err := first.Tick(hlcNow, "server")
		if err != nil {
			t.Fatalf("the tick failed: %v", err)
		}
		if second.Counter != 1 || !second.Physical.Equal(first.Physical) {
			t.Errorf("unexpected reading: %+v", second)
		}
	})

	t.Run("the physical part once time has moved on", func(t *testing.T) {
		first := mustHLC(t, hlcNow, 4, "server")

		second, err := first.Tick(hlcNow.Add(time.Millisecond), "server")
		if err != nil {
			t.Fatalf("the tick failed: %v", err)
		}
		if second.Counter != 0 || !second.Physical.After(first.Physical) {
			t.Errorf("unexpected reading: %+v", second)
		}
	})

	// An exhausted counter borrows a millisecond from the future rather than repeating itself.
	t.Run("past an exhausted counter", func(t *testing.T) {
		first := mustHLC(t, hlcNow, 99999, "server")

		second, err := first.Tick(hlcNow, "server")
		if err != nil {
			t.Fatalf("the tick failed: %v", err)
		}
		if !second.After(first) || second.Counter != 0 {
			t.Errorf("unexpected reading: %+v", second)
		}
	})

	t.Run("from the zero value", func(t *testing.T) {
		first, err := HLC{}.Tick(hlcNow, "server")
		if err != nil {
			t.Fatalf("the tick failed: %v", err)
		}
		if first.IsZero() || !first.Physical.Equal(hlcNow) {
			t.Errorf("unexpected reading: %+v", first)
		}
	})
}

// Two devices that stamped the same millisecond with the same counter have to be ordered the same
// way everywhere, or two clients merging the same pair of changes reach different results.
func TestTheDeviceBreaksTheTie(t *testing.T) {
	a := mustHLC(t, hlcNow, 3, "device-a")
	b := mustHLC(t, hlcNow, 3, "device-b")

	if !b.After(a) || a.After(b) {
		t.Errorf("the tie between %s and %s is not broken consistently", a, b)
	}
	if a.Compare(a) != 0 {
		t.Error("a reading does not equal itself")
	}
}

func TestNewHLCRefusesWhatCannotBeReadBack(t *testing.T) {
	cases := map[string]struct {
		physical   time.Time
		counter    uint32
		device     string
		detailCode string
	}{
		"without a time":           {time.Time{}, 0, "server", "sync.hlc_incomplete"},
		"without a device":         {hlcNow, 0, "", "sync.hlc_device_malformed"},
		"a device with a colon":    {hlcNow, 0, "a:b", "sync.hlc_device_malformed"},
		"a counter past the width": {hlcNow, 100000, "server", "sync.hlc_counter_exhausted"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewHLC(c.physical, c.counter, c.device)
			if !errors.Is(err, ErrInternal) {
				t.Fatalf("error %v, want an internal one", err)
			}
			if got := AsError(err).DetailCode; got != c.detailCode {
				t.Errorf("detail code %s, want %s", got, c.detailCode)
			}
		})
	}
}

func TestTheZeroReadingHasNoTextForm(t *testing.T) {
	if (HLC{}).String() != "" || !(HLC{}).IsZero() {
		t.Error("the zero reading pretends to be a clock")
	}
}
