// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"testing"
	"time"

	calendarservice "github.com/Jersyfi/hubtask/core/application/service/calendar"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// liveRecords answers the fixture's live records as the workspace's, one page.
type liveRecords struct {
	sla *recordStore
	ola []domain.EntrySLA
}

func (l liveRecords) LiveSLAs(_ context.Context, after shared.ID, _ int) ([]domain.EntrySLA, error) {
	if !after.IsZero() {
		return nil, nil
	}
	var out []domain.EntrySLA
	for _, row := range l.sla.rows {
		if live(row) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (l liveRecords) LiveOLAs(context.Context, shared.ID, int) ([]domain.EntrySLA, error) {
	return l.ola, nil
}

type timedApprovals []domain.Approval

func (t timedApprovals) TimedPending(_ context.Context, after shared.ID, _ int) ([]domain.Approval, error) {
	if !after.IsZero() {
		return nil, nil
	}
	return t, nil
}

func (f fixture) restored(now time.Time, approvals ...domain.Approval) RestoredClocks {
	return RestoredClocks{
		Records: liveRecords{sla: f.records}, Approvals: timedApprovals(approvals),
		Policies: f.clocks.Policies, Calendars: f.clocks.Calendars, Items: f.items,
		Timers: calendarservice.Clocks{Timers: f.timers, IDs: f.ids, Clock: at(now)},
	}
}

// A restore brings the records back without their timers (backup-restore.md §8.4): re-armed, a
// goal past its due moment breaches through the timer pass, and a paused one keeps what it had
// spent, counting on from there when the entry moves again.
func TestARestoredGoalIsReArmedFromItsRecord(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, event.ItemCreated, monday, nil)
	f.items.item.Status = "waiting"
	f.deliver(t, event.ItemUpdated, monday.Add(time.Hour), map[string]any{"change_set": map[string]any{"status": map[string]any{}}})
	if paused := f.records.goal(domain.GoalResolution); paused.Consumed != time.Hour {
		t.Fatalf("the paused record keeps %s spent, want 1h", paused.Consumed)
	}

	// The restore: the records are back, no timer is.
	f.timers.rows = nil
	restoredAt := monday.Add(3 * time.Hour)
	if err := f.restored(restoredAt).Rearm(context.Background(), tenantID); err != nil {
		t.Fatal(err)
	}
	if first := f.timers.named("first_response"); first.State != calendar.TimerRunning || !first.DueAt.Equal(restoredAt) {
		t.Fatalf("the overdue first response was re-armed as %+v, want due at once", first)
	}
	for _, name := range []string{"resolution", "resolution_warn"} {
		if timer := f.timers.named(name); timer.State != calendar.TimerPaused || timer.Consumed != time.Hour {
			t.Errorf("%s was re-armed as %s with %s spent, want paused with 1h", name, timer.State, timer.Consumed)
		}
	}

	fire := calendarservice.FireTimers{
		Timers: f.timers, Clock: at(restoredAt),
		Elapsed: map[calendar.Purpose]calendarservice.TimerElapsed{
			calendar.PurposeSLA: SLAElapsed{Records: f.records, Items: f.items, Events: f.events, IDs: f.ids},
		},
	}
	if _, err := fire.Execute(context.Background(), appshared.ActorContext{TenantID: tenantID, Kind: appshared.ActorSystem}); err != nil {
		t.Fatal(err)
	}
	if f.records.goal(domain.GoalFirstResponse).Standing != domain.StandingBreached {
		t.Error("the restored first response past its due moment was never breached")
	}

	f.items.item.Status = "open"
	f.deliver(t, event.ItemUpdated, restoredAt, map[string]any{"change_set": map[string]any{"status": map[string]any{}}})
	// One working hour spent, three left from 12:00 Berlin: due 15:00 Berlin.
	if resumed := f.records.goal(domain.GoalResolution); resumed.Standing != domain.StandingRunning ||
		!resumed.DueAt.Equal(monday.Add(6*time.Hour)) {
		t.Errorf("the restored resolution resumed as %+v, want due 15:00 Berlin", resumed)
	}
}

// Re-arming replaces what a replaced workspace's timers still said, and arms a running goal to
// the due moment its record names.
func TestReArmingReplacesAStaleTimer(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, event.ItemCreated, monday, nil)
	stale := f.timers.named("resolution")
	if err := f.restored(monday.Add(time.Hour)).Rearm(context.Background(), tenantID); err != nil {
		t.Fatal(err)
	}
	live := 0
	for _, row := range f.timers.rows {
		if row.Name == "resolution" && liveTimer(row) {
			live++
			if row.ID == stale.ID || !row.DueAt.Equal(*stale.DueAt) {
				t.Errorf("re-armed %+v, want a new timer due %s", row, stale.DueAt)
			}
		}
	}
	if live != 1 {
		t.Errorf("%d live resolution timers, want 1", live)
	}
}

// An approval's level is re-armed from when it began: a reminder whose moment has passed is not
// sent late, the due moment still ahead is armed to the same instant.
func TestARestoredApprovalReArmsWhatIsStillAhead(t *testing.T) {
	f := newFixture(t)
	now := monday.Add(3 * time.Hour)
	approval := domain.Approval{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005a1"), State: domain.ApprovalPending,
		DueHours: 4, Current: 1, LevelSince: monday,
	}
	if err := f.restored(now, approval).Rearm(context.Background(), tenantID); err != nil {
		t.Fatal(err)
	}
	if remind := f.timers.named("level_1_remind"); remind.ID != "" {
		t.Errorf("the reminder whose moment passed was armed: %+v", remind)
	}
	due := f.timers.named("level_1_due")
	if due.State != calendar.TimerRunning || !due.DueAt.Equal(monday.Add(4*time.Hour)) || due.TenantID != tenantID {
		t.Errorf("the due timer was re-armed as %+v, want due at the level's fourth hour", due)
	}
}
