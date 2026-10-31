// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	timerID   = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000e1")
	timerTen  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000e2")
	timerItem = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000e3")
)

// A four-hour SLA armed on Friday afternoon, paused over the waiting-on-customer weekend status
// and resumed on Monday: the due moment counts only working time while the clock runs.
func TestATimerCountsWorkingTimeWhileItRuns(t *testing.T) {
	cal := officeHours(t)
	timer, err := Arm(timerID, timerTen, PurposeSLA, timerItem, "resolution", cal, 4*time.Hour, berlin(t, "2026-10-30 15:00"))
	if err != nil {
		t.Fatal(err)
	}
	if want := berlin(t, "2026-11-02 11:00"); !timer.DueAt.Equal(want) {
		t.Fatalf("armed due %s, want %s", timer.DueAt, want)
	}

	timer, err = timer.Pause(berlin(t, "2026-10-30 16:00"))
	if err != nil || timer.State != TimerPaused || timer.DueAt != nil || timer.Consumed != time.Hour {
		t.Fatalf("paused: %+v, %v", timer, err)
	}
	// A paused clock spends nothing, however long it waits.
	if got := timer.Remaining(berlin(t, "2026-11-03 12:00")); got != 3*time.Hour {
		t.Errorf("remaining while paused %s, want 3h", got)
	}

	timer, err = timer.Resume(berlin(t, "2026-11-03 10:00"))
	if err != nil {
		t.Fatal(err)
	}
	if want := berlin(t, "2026-11-03 14:00"); !timer.DueAt.Equal(want) {
		t.Errorf("resumed due %s, want %s (lunch is not working time)", timer.DueAt, want)
	}

	if _, err := timer.Fire(berlin(t, "2026-11-03 13:59")); err == nil {
		t.Error("a timer fired before its due moment")
	}
	fired, err := timer.Fire(berlin(t, "2026-11-03 14:00"))
	if err != nil || fired.State != TimerFired || fired.FiredAt == nil {
		t.Fatalf("fired: %+v, %v", fired, err)
	}
	if _, err := fired.Fire(berlin(t, "2026-11-03 15:00")); err == nil {
		t.Error("a timer fired twice")
	}
	for name, act := range map[string]func(time.Time) (Timer, error){
		"pause": fired.Pause, "resume": fired.Resume, "stop": fired.Stop,
	} {
		if _, err := act(berlin(t, "2026-11-03 15:00")); err == nil {
			t.Errorf("%s on a fired timer was accepted", name)
		}
	}
}

func TestStopKeepsWhatWasSpent(t *testing.T) {
	timer, err := Arm(timerID, timerTen, PurposeApproval, timerItem, "decision", Continuous(), 8*time.Hour,
		time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := timer.Stop(time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC))
	if err != nil || stopped.State != TimerStopped || stopped.Consumed != 3*time.Hour || stopped.DueAt != nil {
		t.Fatalf("stopped: %+v, %v", stopped, err)
	}
	if again, err := stopped.Stop(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)); err != nil || again.Consumed != 3*time.Hour {
		t.Errorf("stopping twice changed the timer: %+v, %v", again, err)
	}
}

func TestArmRefusesWhatCannotRun(t *testing.T) {
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	for name, arm := range map[string]func() (Timer, error){
		"unknown purpose": func() (Timer, error) {
			return Arm(timerID, timerTen, Purpose("NAP"), timerItem, "goal", Continuous(), time.Hour, now)
		},
		// No handler answers an effectiveness check: such a check is a RELATIVE_DATE rule, so a
		// timer of that purpose would only fire into nothing.
		"a purpose nothing handles": func() (Timer, error) {
			return Arm(timerID, timerTen, Purpose("EFFECTIVENESS_CHECK"), timerItem, "check", Continuous(), time.Hour, now)
		},
		"no calendar": func() (Timer, error) {
			return Arm(timerID, timerTen, PurposeSLA, timerItem, "goal", BusinessCalendar{}, time.Hour, now)
		},
		"no budget": func() (Timer, error) {
			return Arm(timerID, timerTen, PurposeSLA, timerItem, "goal", Continuous(), 0, now)
		},
		"too long": func() (Timer, error) {
			return Arm(timerID, timerTen, PurposeSLA, timerItem, "goal", Continuous(), MaxBudget+time.Hour, now)
		},
		"bad name": func() (Timer, error) {
			return Arm(timerID, timerTen, PurposeSLA, timerItem, "First Response", Continuous(), time.Hour, now)
		},
		"no subject": func() (Timer, error) {
			return Arm(timerID, timerTen, PurposeSLA, shared.ID(""), "goal", Continuous(), time.Hour, now)
		},
	} {
		if _, err := arm(); err == nil {
			t.Errorf("%s: armed", name)
		}
	}
}
