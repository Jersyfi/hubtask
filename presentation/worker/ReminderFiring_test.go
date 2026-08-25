// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The handler's own job is small and worth pinning down: the tenant it runs for, that it holds its
// own row before reading anything, and when it comes back. What the pass does is the application
// layer's, and is tested there.

// schedule is the reminder store as a pass with nothing due sees it: no work, and one answer about
// when the tenant next owes something. The rest of the port exists because a port is one
// interface.
type schedule struct{ next *time.Time }

func (s *schedule) ClaimDue(context.Context, time.Time, int) ([]workdomain.Reminder, error) {
	return nil, nil
}
func (s *schedule) NextMoment(context.Context) (*time.Time, error) { return s.next, nil }
func (s *schedule) Settle(context.Context, shared.ID, workdomain.ReminderState) (bool, error) {
	return false, nil
}
func (s *schedule) Find(context.Context, shared.ID) (workdomain.Reminder, error) {
	return workdomain.Reminder{}, shared.ErrNotFound
}
func (s *schedule) ListForItem(context.Context, shared.ID) ([]workdomain.Reminder, error) {
	return nil, nil
}
func (s *schedule) ListPendingForItem(context.Context, shared.ID) ([]workdomain.Reminder, error) {
	return nil, nil
}
func (s *schedule) CountForItem(context.Context, shared.ID) (int, error)   { return 0, nil }
func (s *schedule) Insert(context.Context, workdomain.Reminder) error      { return nil }
func (s *schedule) Update(context.Context, workdomain.Reminder, int) error { return nil }
func (s *schedule) Reschedule(context.Context, workdomain.Reminder) error  { return nil }
func (s *schedule) Delete(context.Context, shared.ID, int) error           { return nil }

var _ workrepo.Reminders = (*schedule)(nil)

func firingFor(next *time.Time, at time.Time) (ReminderFiring, *queueDouble) {
	jobs := newQueue()
	return ReminderFiring{
		Firing: work.FireReminders{
			Reminders: &schedule{next: next}, Clock: clock.Fixed(at), BatchSize: 10,
		},
		Queue:        jobs,
		Clock:        clock.Fixed(at),
		Continuation: 2 * time.Second,
		MinimumWait:  time.Second,
	}, jobs
}

var (
	firingTenant = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	firingNow    = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
)

var firingJob = queue.Job{
	ID: "0192f000-0000-7000-8000-0000000000d1", TenantID: firingTenant,
	Kind: queue.KindReminderFire, Attempts: 1, MaxAttempts: 5,
}

// The pass holds its own row before it reads anything. That is what turns a write arriving during
// the pass into a wait rather than into a wake-up nobody scheduled (queue.Queue.Hold).
func TestTheFiringPassHoldsItsOwnJobFirst(t *testing.T) {
	firing, jobs := firingFor(nil, firingNow)

	if _, err := firing.Run(context.Background(), firingJob); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if len(jobs.held) != 1 || jobs.held[0] != firingJob.ID {
		t.Errorf("the pass held %v", jobs.held)
	}
}

// Three answers, and which one is given is the whole of this handler's decision.
func TestWhenTheFiringPassComesBack(t *testing.T) {
	soon := firingNow.Add(30 * time.Second)
	past := firingNow.Add(-time.Hour)

	for name, test := range map[string]struct {
		next       *time.Time
		wantRepeat bool
		wantAfter  time.Duration
	}{
		"nothing left, so the job finishes": {},
		"something later, so it sleeps until then": {
			next: &soon, wantRepeat: true, wantAfter: 30 * time.Second,
		},
		"something already due, so it waits a moment rather than spinning": {
			next: &past, wantRepeat: true, wantAfter: time.Second,
		},
	} {
		t.Run(name, func(t *testing.T) {
			firing, _ := firingFor(test.next, firingNow)

			result, err := firing.Run(context.Background(), firingJob)
			if err != nil {
				t.Fatalf("the pass failed: %v", err)
			}
			if result.Repeat != test.wantRepeat {
				t.Fatalf("repeat is %v", result.Repeat)
			}
			if result.RepeatAfter != test.wantAfter {
				t.Errorf("it comes back after %v rather than %v", result.RepeatAfter, test.wantAfter)
			}
		})
	}
}

// A job without a tenant is a programming error with a name: every read the pass makes is made for
// the tenant the job names.
func TestAFiringJobWithoutATenantIsRefused(t *testing.T) {
	firing, _ := firingFor(nil, firingNow)

	job := firingJob
	job.TenantID = ""
	if _, err := firing.Run(context.Background(), job); err == nil {
		t.Fatal("a job without a tenant was accepted")
	}
}
