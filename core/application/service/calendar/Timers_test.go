// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"context"
	"errors"
	"fmt"
	"maps"
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

func (s *timerStore) Defer(_ context.Context, id shared.ID, now time.Time) error {
	row := s.rows[id]
	if row.State == domain.TimerRunning {
		later := now.Add(time.Minute)
		row.DueAt = &later
		s.rows[id] = row
	}
	return nil
}

// savepoints undoes what fn changed in the store when it fails, as a savepoint does.
type savepoints struct{ store *timerStore }

func (p savepoints) WithinSavepoint(ctx context.Context, fn func(context.Context) error) error {
	kept := maps.Clone(p.store.rows)
	if err := fn(ctx); err != nil {
		p.store.rows = kept
		return err
	}
	return nil
}

// failing is a purpose's handler that fails for one subject and records the rest.
type failing struct {
	subject shared.ID
	fired   []domain.Timer
}

func (f *failing) Elapsed(_ context.Context, _ appshared.ActorContext, timer domain.Timer) error {
	if timer.SubjectID == f.subject {
		return shared.ErrConflict.WithDetail("notification.refused")
	}
	f.fired = append(f.fired, timer)
	return nil
}

type failedSignals struct{ purposes []string }

func (s *failedSignals) TimerFailed(_ context.Context, purpose string) {
	s.purposes = append(s.purposes, purpose)
}

// One timer whose handler always fails degrades only itself: it is undone and put back with a
// delay, counted, and the timers behind it still fire in the same pass.
func TestAFailingTimerDoesNotHoldBackTheOthers(t *testing.T) {
	armed := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	bad, _ := domain.Arm(timerID, tenant, domain.PurposeSLA, subject, "resolution", domain.Continuous(), time.Minute, armed)
	otherID := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f4")
	other, _ := domain.Arm(otherID, tenant, domain.PurposeSLA, otherID, "resolution", domain.Continuous(), 2*time.Minute, armed)
	store := &timerStore{rows: map[shared.ID]domain.Timer{timerID: bad, otherID: other}}
	handler, signals := &failing{subject: subject}, &failedSignals{}
	now := armed.Add(time.Hour)
	pass := FireTimers{
		Timers: store, Clock: clock.Fixed(now), Savepoints: savepoints{store}, Signals: signals,
		Elapsed: map[domain.Purpose]TimerElapsed{domain.PurposeSLA: handler},
	}
	outcome, err := pass.Execute(context.Background(), appshared.ActorContext{TenantID: tenant})
	if err != nil {
		t.Fatalf("one failing timer failed the pass: %v", err)
	}
	if outcome.Fired != 1 || outcome.Failed != 1 || len(handler.fired) != 1 || handler.fired[0].ID != otherID {
		t.Fatalf("pass %+v fired %+v, want the other timer fired and the bad one failed", outcome, handler.fired)
	}
	kept := store.rows[timerID]
	if kept.State != domain.TimerRunning || !kept.DueAt.After(now) {
		t.Errorf("the failing timer is %s due %v, want running and put back", kept.State, kept.DueAt)
	}
	if store.rows[otherID].State != domain.TimerFired {
		t.Error("the other timer did not fire")
	}
	if len(signals.purposes) != 1 || signals.purposes[0] != string(domain.PurposeSLA) {
		t.Errorf("failures counted %v", signals.purposes)
	}
	if outcome.NextAt == nil || !outcome.NextAt.After(now) {
		t.Errorf("next at %v, want the deferred moment", outcome.NextAt)
	}
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

type countedIDs struct{ next *int }

func (c countedIDs) NewID() shared.ID {
	*c.next++
	return shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8000-%012d", *c.next))
}

// A rebuilt clock replaces what stood under its name and asks for the firing at its due moment -
// at once, when its budget is spent.
func TestRebuildReplacesTheLiveTimerAndSeedsItsFiring(t *testing.T) {
	store, queued := &timerStore{rows: map[shared.ID]domain.Timer{}}, &jobs{}
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	clocks := Clocks{Timers: store, Jobs: queuer{jobs: queued}, Clock: clock.Fixed(now), IDs: countedIDs{next: new(0)}}
	ctx := context.Background()

	stale, err := clocks.Arm(ctx, tenant, domain.PurposeSLA, subject, "resolution", domain.Continuous(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := clocks.Rebuild(ctx, tenant, domain.PurposeSLA, subject, "resolution", domain.Continuous(),
		4*time.Hour, 4*time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	if store.rows[stale.ID].State != domain.TimerStopped {
		t.Errorf("the timer standing before is %s, want STOPPED", store.rows[stale.ID].State)
	}
	if rebuilt.State != domain.TimerRunning || !rebuilt.DueAt.Equal(now) {
		t.Fatalf("rebuilt %+v, want running and due now", rebuilt)
	}
	last := queued.requests[len(queued.requests)-1]
	if last.Kind != queue.KindTimerFire || !last.RunAt.Equal(now) {
		t.Errorf("seeded %+v, want a firing now", last)
	}
	pass := FireTimers{Timers: store, Clock: clock.Fixed(now)}
	if outcome, err := pass.Execute(ctx, appshared.ActorContext{TenantID: tenant}); err != nil || outcome.Fired != 1 {
		t.Errorf("the next pass fired %+v, %v; want the rebuilt clock", outcome, err)
	}
}
