// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"testing"
	"time"
)

// UC-TIM-01 check 9: a start and a stop queued offline keep the device's moments when the server
// receives them later; a moment ahead of the server's clock is now, and a start more than a day
// before it is a timer run too long.
func TestATimerQueuedOfflineKeepsTheDevicesMoments(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.clock.now = monday.Add(3 * time.Hour)
	timer, err := StartTimer{Keeper: f.keeper}.ExecuteAt(ctx, as(ana), checkout, "development", "", nil, monday)
	if err != nil || !timer.StartedAt.Equal(monday) {
		t.Fatalf("a start queued offline: %+v %v", timer, err)
	}
	r, err := StopTimer{Keeper: f.keeper}.ExecuteAt(ctx, as(ana), time.UTC, monday.Add(51*time.Minute))
	if err != nil || r.Minutes != 51 || !r.CreatedAt.Equal(f.clock.now) {
		t.Fatalf("a stop queued offline: %+v %v, want 51 minutes recorded now", r, err)
	}

	ahead, err := StartTimer{Keeper: f.keeper}.ExecuteAt(ctx, as(ana), checkout, "development", "", nil, f.clock.now.Add(time.Hour))
	if err != nil || !ahead.StartedAt.Equal(f.clock.now) {
		t.Fatalf("a device's clock ahead: %+v %v, want now", ahead, err)
	}
	if err := (DiscardTimer{Keeper: f.keeper}).Execute(ctx, as(ana)); err != nil {
		t.Fatal(err)
	}
	if _, err := (StartTimer{Keeper: f.keeper}).ExecuteAt(ctx, as(ana), checkout, "development", "", nil, f.clock.now.Add(-25*time.Hour)); detail(err) != "timers.ran_too_long" {
		t.Fatalf("a start a day and more ago: %v", err)
	}
}
