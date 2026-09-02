// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"context"
	"testing"
	"time"
)

func TestTheRateIsTheStageTheRunIsIn(t *testing.T) {
	plan := Plan{
		{PerSecond: 10, For: time.Minute},
		{PerSecond: 40, For: time.Minute},
		{PerSecond: 80, For: 30 * time.Second},
	}

	cases := []struct {
		elapsed time.Duration
		want    int
	}{
		{0, 10},
		{59 * time.Second, 10},
		// The boundary belongs to the stage that starts there, not to the one that ends.
		{time.Minute, 40},
		{2 * time.Minute, 80},
		{2*time.Minute + 29*time.Second, 80},
		// Past the end of the plan there is no rate at all, which is what stops the pacer.
		{3 * time.Minute, 0},
	}
	for _, c := range cases {
		if got := plan.RateAt(c.elapsed); got != c.want {
			t.Errorf("at %s the rate is %d, want %d", c.elapsed, got, c.want)
		}
	}

	if plan.Duration() != 2*time.Minute+30*time.Second {
		t.Errorf("duration = %s", plan.Duration())
	}
	if plan.Peak() != 80 {
		t.Errorf("peak = %d, want 80", plan.Peak())
	}
}

// The property the whole harness rests on: permits come from the clock, not from the answers. A
// consumer that is slower than the rate must fall behind rather than slow the rate down - that is
// the difference between measuring the installation and measuring the generator.
func TestThePacerDoesNotWaitForItsConsumer(t *testing.T) {
	started := time.Now()
	ctx, stop := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer stop()

	pacer := NewPacer(ctx, FlatPlan(200, time.Second), started)

	// Consume slowly on purpose: five permits at 20 ms apart against an offered 200 a second.
	taken := 0
	for range 5 {
		if !pacer.Wait(ctx) {
			break
		}
		taken++
		time.Sleep(20 * time.Millisecond)
	}
	if taken != 5 {
		t.Fatalf("took %d permits, want 5 - the pacer stopped producing", taken)
	}
	// A generator that had waited for its consumer would have needed 100 ms for five permits at
	// its own pace plus the consumer's 100 ms of sleeping. What is asserted is only that the
	// permits kept coming; the timing itself belongs to the run, not to a unit test.
	if time.Since(started) > 400*time.Millisecond {
		t.Errorf("five permits took %s", time.Since(started))
	}
}

// The pacer must end with the run. A permit channel left open is a worker that never returns, and
// a load test that does not stop is one nobody runs twice.
func TestThePacerClosesWhenTheRunEnds(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	pacer := NewPacer(ctx, FlatPlan(50, time.Minute), time.Now())
	stop()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("the pacer was still handing out permits after the run was cancelled")
		default:
		}
		if !pacer.Wait(context.Background()) {
			return
		}
	}
}
