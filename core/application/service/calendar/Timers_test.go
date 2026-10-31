// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var (
	tenant  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f1")
	subject = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f2")
	timerID = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f3")
)

// timerStore is the repository as a map, with the version check the adapter makes.
type timerStore struct{ rows map[shared.ID]domain.Timer }

func (s *timerStore) Insert(_ context.Context, timer domain.Timer, _ time.Time) error {
	for _, row := range s.rows {
		if row.Purpose == timer.Purpose && row.SubjectID == timer.SubjectID && row.Name == timer.Name &&
			(row.State == domain.TimerRunning || row.State == domain.TimerPaused) {
			return shared.ErrConflict.WithDetail("timer.already_running")
		}
	}
	s.rows[timer.ID] = timer
	return nil
}

func (s *timerStore) FindLive(_ context.Context, purpose domain.Purpose, subject shared.ID, name string) (domain.Timer, error) {
	for _, row := range s.rows {
		if row.Purpose == purpose && row.SubjectID == subject && row.Name == name &&
			(row.State == domain.TimerRunning || row.State == domain.TimerPaused) {
			return row, nil
		}
	}
	return domain.Timer{}, shared.ErrNotFound
}

func (s *timerStore) Update(_ context.Context, timer domain.Timer, expected int, _ time.Time) error {
	if s.rows[timer.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	timer.Version = expected + 1
	s.rows[timer.ID] = timer
	return nil
}

func (s *timerStore) ClaimDue(_ context.Context, now time.Time, limit int) ([]domain.Timer, error) {
	var due []domain.Timer
	for _, row := range s.rows {
		if row.State == domain.TimerRunning && !row.DueAt.After(now) && len(due) < limit {
			due = append(due, row)
		}
	}
	return due, nil
}

func (s *timerStore) NextDue(context.Context) (*time.Time, error) {
	var next *time.Time
	for _, row := range s.rows {
		if row.State == domain.TimerRunning && (next == nil || row.DueAt.Before(*next)) {
			at := *row.DueAt
			next = &at
		}
	}
	return next, nil
}

type jobs struct{ requests []queue.Request }

func (j *jobs) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	j.requests = append(j.requests, request)
	return timerID, nil
}

type fixedIDs struct{}

func (fixedIDs) NewID() shared.ID { return timerID }

type recorder struct{ fired []domain.Timer }

func (r *recorder) Elapsed(_ context.Context, _ appshared.ActorContext, timer domain.Timer) error {
	r.fired = append(r.fired, timer)
	return nil
}

func TestArmSeedsTheTenantsFiringAtTheDueMoment(t *testing.T) {
	store, queued := &timerStore{rows: map[shared.ID]domain.Timer{}}, &jobs{}
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	clocks := Clocks{Timers: store, Jobs: queuer{jobs: queued}, Clock: clock.Fixed(now), IDs: fixedIDs{}}

	timer, err := clocks.Arm(context.Background(), tenant, domain.PurposeSLA, subject, "resolution",
		domain.Continuous(), 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued.requests) != 1 || queued.requests[0].Kind != queue.KindTimerFire ||
		queued.requests[0].TenantID != tenant || !queued.requests[0].RunAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("seeded %+v, want one timer.fire at the due moment", queued.requests)
	}
	if _, err := clocks.Arm(context.Background(), tenant, domain.PurposeSLA, subject, "resolution",
		domain.Continuous(), time.Hour); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second live timer of the same goal was armed: %v", err)
	}

	paused, err := clocks.Pause(context.Background(), domain.PurposeSLA, subject, "resolution")
	if err != nil || paused.State != domain.TimerPaused || paused.Version != timer.Version+1 {
		t.Fatalf("paused %+v, %v", paused, err)
	}
	if len(queued.requests) != 1 {
		t.Error("a pause asked for a firing")
	}
}

func TestTheFiringPassFiresWhatIsDueOnceAndHandsItToItsPurpose(t *testing.T) {
	armed := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	timer, err := domain.Arm(timerID, tenant, domain.PurposeApproval, subject, "decision",
		domain.Continuous(), time.Hour, armed)
	if err != nil {
		t.Fatal(err)
	}
	store := &timerStore{rows: map[shared.ID]domain.Timer{timerID: timer}}
	approvals := &recorder{}
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenant}

	early := FireTimers{Timers: store, Clock: clock.Fixed(armed.Add(30 * time.Minute)),
		Elapsed: map[domain.Purpose]TimerElapsed{domain.PurposeApproval: approvals}}
	outcome, err := early.Execute(context.Background(), actor)
	if err != nil || outcome.Fired != 0 || outcome.NextAt == nil || !outcome.NextAt.Equal(armed.Add(time.Hour)) {
		t.Fatalf("early pass %+v, %v", outcome, err)
	}

	due := early
	due.Clock = clock.Fixed(armed.Add(time.Hour))
	outcome, err = due.Execute(context.Background(), actor)
	if err != nil || outcome.Fired != 1 || outcome.Unhandled != 0 || outcome.NextAt != nil {
		t.Fatalf("due pass %+v, %v", outcome, err)
	}
	if len(approvals.fired) != 1 || approvals.fired[0].State != domain.TimerFired {
		t.Fatalf("the approval's handler got %+v", approvals.fired)
	}
	if again, _ := due.Execute(context.Background(), actor); again.Fired != 0 {
		t.Error("the timer fired twice")
	}
}

func TestAPurposeWithoutAHandlerStillFires(t *testing.T) {
	armed := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	timer, _ := domain.Arm(timerID, tenant, domain.PurposePageReview, subject, "check",
		domain.Continuous(), time.Minute, armed)
	store := &timerStore{rows: map[shared.ID]domain.Timer{timerID: timer}}
	pass := FireTimers{Timers: store, Clock: clock.Fixed(armed.Add(time.Hour))}
	outcome, err := pass.Execute(context.Background(), appshared.ActorContext{TenantID: tenant})
	if err != nil || outcome.Fired != 1 || outcome.Unhandled != 1 {
		t.Fatalf("pass %+v, %v", outcome, err)
	}
	if store.rows[timerID].State != domain.TimerFired {
		t.Error("the unhandled timer was left running")
	}
}

// queuer is the queue port with only Enqueue implemented; anything else a timer must not call
// panics on the nil embedded interface.
type queuer struct {
	queue.Queue
	jobs *jobs
}

func (q queuer) Enqueue(ctx context.Context, request queue.Request) (shared.ID, error) {
	return q.jobs.Enqueue(ctx, request)
}
