// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package clock_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/infrastructure/clock"
)

func hybrid(t *testing.T, source port.Clock) *clock.HybridClock {
	t.Helper()
	hlc, err := clock.NewHybridClock(source, "server-1")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	return hlc
}

// A stopped wall clock must still produce readings that can be ordered - that is what the counter
// is for.
func TestTwoReadingsInOneMillisecondAreOrdered(t *testing.T) {
	hlc := hybrid(t, port.Fixed(start))

	first, second := hlc.Next(), hlc.Next()
	if !second.After(first) {
		t.Errorf("%s does not come after %s", second, first)
	}
	if second.Counter != first.Counter+1 {
		t.Errorf("the counter went from %d to %d", first.Counter, second.Counter)
	}
}

func TestTheReadingFollowsTheWallClock(t *testing.T) {
	source := &moving{at: start}
	hlc := hybrid(t, source)

	first, second := hlc.Next(), hlc.Next()
	if !second.Physical.After(first.Physical) || second.Counter != 0 {
		t.Errorf("%s does not follow the wall clock past %s", second, first)
	}
}

// Two requests stamping at once get two readings, and both are usable: a repeated reading would
// make two changes unorderable, which is exactly what the merge rules cannot have.
func TestConcurrentStampsAreDistinct(t *testing.T) {
	hlc := hybrid(t, port.Fixed(start))

	const stamps = 200
	readings := make([]shared.HLC, stamps)
	var wait sync.WaitGroup

	for i := range stamps {
		wait.Add(1)
		// Through SafeGo like every other goroutine in this repository (ADR-0016): the ban is not
		// relaxed for tests, because a panic in a test goroutine is just as invisible.
		concurrency.Go(t.Context(), "test.stamp", func(context.Context) {
			defer wait.Done()
			readings[i] = hlc.Next()
		})
	}
	wait.Wait()

	seen := make(map[string]bool, stamps)
	for _, reading := range readings {
		if reading.IsZero() {
			t.Fatal("a stamp came back empty")
		}
		if seen[reading.String()] {
			t.Fatalf("%s was handed out twice", reading)
		}
		seen[reading.String()] = true
	}
}

// A device identifier that could not be read back out of the stored form is refused at
// construction, which is what lets Next have no error to report.
func TestABadDeviceIsRefusedAtConstruction(t *testing.T) {
	for _, device := range []string{"", "server:1"} {
		if _, err := clock.NewHybridClock(port.Fixed(start), device); err == nil {
			t.Errorf("the device %q was accepted", device)
		}
	}
}

// The wall clock going backwards - an NTP correction, a suspended machine - must not produce a
// reading that sorts before one already handed out.
func TestAClockThatJumpsBackwardsDoesNotUndoAReading(t *testing.T) {
	source := &jumping{at: start}
	hlc := hybrid(t, source)

	first := hlc.Next()
	source.at = start.Add(-time.Hour)
	second := hlc.Next()

	if !second.After(first) {
		t.Errorf("%s does not come after %s although the wall clock went back", second, first)
	}
}

type jumping struct{ at time.Time }

func (j *jumping) Now() time.Time { return j.at }
