// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// clocks keeps the live review timers by page.
type clocks struct {
	live    map[shared.ID]calendar.Timer
	stopped int
}

func (c *clocks) Arm(_ context.Context, tenant shared.ID, purpose calendar.Purpose, subject shared.ID, name string,
	cal calendar.BusinessCalendar, budget time.Duration) (calendar.Timer, error) {
	if _, ok := c.live[subject]; ok {
		return calendar.Timer{}, shared.ErrConflict.WithDetail("timer.already_running")
	}
	timer, err := calendar.Arm(subject, tenant, purpose, subject, name, cal, budget, start)
	if err == nil {
		c.live[subject] = timer
	}
	return timer, err
}

func (c *clocks) Stop(_ context.Context, _ calendar.Purpose, subject shared.ID, _ string) (calendar.Timer, error) {
	timer, ok := c.live[subject]
	if !ok {
		return calendar.Timer{}, shared.ErrNotFound
	}
	delete(c.live, subject)
	c.stopped++
	return timer, nil
}

type told struct{ pages []shared.ID }

func (t *told) PageReviewDue(_ context.Context, page domain.Page) error {
	t.pages = append(t.pages, page.ID)
	return nil
}

// UC-DOC-12 check 4: verifying a page arms its review clock to the date, verifying it again
// replaces the clock, taking the verification away stops it - and when it elapses the owner is told,
// unless the page is no longer verified.
func TestAPagesOwnerIsRemindedWhenItFallsDue(t *testing.T) {
	f := newFixture()
	timers := &clocks{live: map[shared.ID]calendar.Timer{}}
	f.space.Reviews = timers
	page := f.create(t, anna, hub, "", "VPN setup", "Connect first")
	verify := func(until *time.Time) {
		t.Helper()
		if _, err := (VerifyPage{Space: f.space}).Execute(context.Background(), as(anna), VerifyPageCommand{
			PageID: page.ID, OwnerID: bert, Until: until,
		}); err != nil {
			t.Fatalf("VerifyPage: %v", err)
		}
	}
	threeMonths := start.AddDate(0, 3, 0)
	verify(&threeMonths)
	timer, ok := timers.live[page.ID]
	if !ok || timer.Purpose != calendar.PurposePageReview || timer.DueAt == nil || !timer.DueAt.Equal(threeMonths) {
		t.Fatalf("the clock is %+v (armed %v), want one due %v", timer, ok, threeMonths)
	}
	later := start.AddDate(0, 6, 0)
	verify(&later)
	if timers.stopped != 1 || !timers.live[page.ID].DueAt.Equal(later) {
		t.Fatalf("verifying again left %+v after %d stops", timers.live[page.ID], timers.stopped)
	}

	notified := &told{}
	elapsed := PageReviewElapsed{Pages: f.store, Timers: timers, Notify: notified, Clock: f.clock}
	f.clock.at = later
	delete(timers.live, page.ID)
	if err := elapsed.Elapsed(context.Background(), as(anna), calendar.Timer{SubjectID: page.ID}); err != nil {
		t.Fatalf("Elapsed: %v", err)
	}
	if len(notified.pages) != 1 || notified.pages[0] != page.ID {
		t.Fatalf("told %v, want the page", notified.pages)
	}

	verify(nil)
	if _, ok := timers.live[page.ID]; ok {
		t.Fatal("taking the verification away left the clock running")
	}
	if err := elapsed.Elapsed(context.Background(), as(anna), calendar.Timer{SubjectID: page.ID}); err != nil || len(notified.pages) != 1 {
		t.Fatalf("an unverified page was told again: %v, %v", notified.pages, err)
	}
}

// A date beyond a timer's bound is reached in legs: the leg that elapses early arms the next.
func TestAReviewBeyondAYearIsReachedInLegs(t *testing.T) {
	f := newFixture()
	timers := &clocks{live: map[shared.ID]calendar.Timer{}}
	f.space.Reviews = timers
	page := f.create(t, anna, hub, "", "Policy", "Rules")
	twoYears := start.AddDate(2, 0, 0)
	if _, err := (VerifyPage{Space: f.space}).Execute(context.Background(), as(anna), VerifyPageCommand{
		PageID: page.ID, Until: &twoYears,
	}); err != nil {
		t.Fatalf("VerifyPage: %v", err)
	}
	if timers.live[page.ID].Budget != calendar.MaxBudget {
		t.Fatalf("the first leg is %v, want the bound", timers.live[page.ID].Budget)
	}
	delete(timers.live, page.ID)
	notified := &told{}
	f.clock.at = start.Add(calendar.MaxBudget)
	elapsed := PageReviewElapsed{Pages: f.store, Timers: timers, Notify: notified, Clock: f.clock}
	if err := elapsed.Elapsed(context.Background(), as(anna), calendar.Timer{SubjectID: page.ID}); err != nil {
		t.Fatalf("Elapsed: %v", err)
	}
	if len(notified.pages) != 0 {
		t.Fatal("the owner was told a year early")
	}
	if _, ok := timers.live[page.ID]; !ok {
		t.Fatal("the next leg was not armed")
	}
}
