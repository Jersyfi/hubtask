// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	streams "github.com/Jersyfi/hubtask/core/application/repository/streams"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// lock stands in for the advisory lock: one holder at a time, and a holder that can be cut off
// without being asked - which is what a lost connection does to a leader.
type lock struct {
	mu     sync.Mutex
	holder string
}

func (l *lock) take(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder != "" {
		return l.holder == name
	}
	l.holder = name
	return true
}

func (l *lock) held(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.holder == name
}

func (l *lock) cut() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.holder = ""
}

// leadership is one instance's view of that lock.
type leadership struct {
	lock     *lock
	name     string
	released bool
}

func (l *leadership) Acquire(context.Context) (bool, error) { return l.lock.take(l.name), nil }
func (l *leadership) Confirm(context.Context) (bool, error) { return l.lock.held(l.name), nil }

func (l *leadership) Release(context.Context) error {
	l.released = true
	if l.lock.held(l.name) {
		l.lock.cut()
	}
	return nil
}

type schedulerSignals struct {
	depths    map[string]int64
	lags      []float64
	freshness map[string]time.Time
}

func newSchedulerSignals() *schedulerSignals {
	return &schedulerSignals{depths: map[string]int64{}, freshness: map[string]time.Time{}}
}

func (s *schedulerSignals) BackupLastSuccess(_ context.Context, targetID string, at time.Time) {
	s.freshness[targetID] = at
}

func (s *schedulerSignals) QueueDepth(_ context.Context, kind string, pending int64) {
	s.depths[kind] = pending
}

func (s *schedulerSignals) SchedulerTickLag(_ context.Context, seconds float64) {
	s.lags = append(s.lags, seconds)
}

type depthQueue struct {
	queueDouble
	depths []queue.Depth
}

func (d *depthQueue) Depth(context.Context) ([]queue.Depth, error) { return d.depths, nil }

func scheduler(l *leadership, jobs queue.Queue, signals SchedulerSignals) Scheduler {
	return Scheduler{
		Leadership:   l,
		Queue:        jobs,
		UnitOfWork:   &unitOfWork{},
		Clock:        clock.Fixed(now),
		Signals:      signals,
		Kinds:        []queue.Kind{queue.KindOutboxDispatch},
		TickInterval: 10 * time.Second,
	}
}

// A kind with nothing waiting still gets a reading. A gauge that has never been written has no
// series at all, and an alert on a backlog that never appeared reads "no data" - which everybody
// takes for a broken dashboard rather than for an empty queue.
func TestAKindWithAnEmptyQueueIsStillReported(t *testing.T) {
	signals := newSchedulerSignals()

	scheduler(&leadership{lock: &lock{}, name: "a"}, &depthQueue{}, signals).tick(t.Context(), false, now)

	depth, reported := signals.depths[queue.KindOutboxDispatch.String()]
	if !reported {
		t.Fatal("an empty queue produced no reading at all")
	}
	if depth != 0 {
		t.Errorf("depth = %d for an empty queue, want 0", depth)
	}
}

// The acceptance criterion of A-08: two instances, one active. Both tick, and the work happens
// once - the second one is a standby that costs nothing but a lock attempt.
func TestOfTwoSchedulersOnlyOneActs(t *testing.T) {
	contested := &lock{}
	jobs := &depthQueue{depths: []queue.Depth{{Kind: queue.KindOutboxDispatch, Pending: 4}}}

	firstSignals, secondSignals := newSchedulerSignals(), newSchedulerSignals()
	first := scheduler(&leadership{lock: contested, name: "a"}, jobs, firstSignals)
	second := scheduler(&leadership{lock: contested, name: "b"}, jobs, secondSignals)

	leadingFirst := first.tick(t.Context(), false, now)
	leadingSecond := second.tick(t.Context(), false, now)

	if !leadingFirst {
		t.Error("the first scheduler did not become the leader although the lock was free")
	}
	if leadingSecond {
		t.Error("both schedulers are leading")
	}
	if len(firstSignals.depths) == 0 {
		t.Error("the leader did no work")
	}
	if len(secondSignals.depths) != 0 {
		t.Error("the standby did work")
	}
}

// A leader whose connection is cut has lost the lock without being told. It has to notice on its
// next tick and stand by, because another instance has taken over by then - a former leader that
// keeps acting is the double execution the lock exists to prevent.
func TestALeaderThatLostItsLockStandsBy(t *testing.T) {
	contested := &lock{}
	held := &leadership{lock: contested, name: "a"}
	signals := newSchedulerSignals()
	leader := scheduler(held, &depthQueue{depths: []queue.Depth{{Kind: queue.KindOutboxDispatch, Pending: 1}}}, signals)

	if leading := leader.tick(t.Context(), false, now); !leading {
		t.Fatal("the scheduler did not take the free lock")
	}

	// The connection carrying the lock goes away, and somebody else takes it.
	contested.cut()
	other := &leadership{lock: contested, name: "b"}
	if taken, _ := other.Acquire(t.Context()); !taken {
		t.Fatal("the standby could not take the free lock")
	}

	signals.depths = map[string]int64{}
	if leading := leader.tick(t.Context(), true, now); leading {
		t.Error("the former leader believes it is still leading")
	}
	if len(signals.depths) != 0 {
		t.Error("the former leader did work after losing the lock")
	}
}

// The backlog per kind is a property of the installation, so exactly one process reports it - a
// gauge written by every replica would be summed across instances and read as N times the truth.
func TestTheLeaderPublishesTheBacklogPerKind(t *testing.T) {
	signals := newSchedulerSignals()
	jobs := &depthQueue{depths: []queue.Depth{
		{Kind: queue.KindOutboxDispatch, Pending: 7},
		{Kind: queue.Kind("reminder.fire"), Pending: 2},
	}}

	scheduler(&leadership{lock: &lock{}, name: "a"}, jobs, signals).tick(t.Context(), false, now)

	if signals.depths[queue.KindOutboxDispatch.String()] != 7 {
		t.Errorf("outbox depth = %d, want 7", signals.depths[queue.KindOutboxDispatch.String()])
	}
	if signals.depths["reminder.fire"] != 2 {
		t.Errorf("reminder depth = %d, want 2", signals.depths["reminder.fire"])
	}
}

// The lag is measured against when the tick was due. A tick that ran late and then on time again
// would otherwise look punctual, and drift is exactly what this gauge exists to show.
func TestTheTickLagIsMeasuredAgainstWhenItWasDue(t *testing.T) {
	signals := newSchedulerSignals()
	leader := scheduler(&leadership{lock: &lock{}, name: "a"}, &depthQueue{}, signals)

	leader.tick(t.Context(), false, now.Add(-4*time.Second))
	// A tick that is early - a clock that jumped - reports no lag rather than a negative one.
	leader.tick(t.Context(), true, now.Add(time.Second))

	want := []float64{4, 0}
	if len(signals.lags) != len(want) {
		t.Fatalf("%d lag readings, want %d", len(signals.lags), len(want))
	}
	for i, seconds := range signals.lags {
		if seconds != want[i] {
			t.Errorf("lag %d = %v, want %v", i+1, seconds, want[i])
		}
	}
}

// Leadership is given up on the way out, so that a standby takes over at once instead of waiting
// for a socket to time out (observability-reliability.md §9).
func TestLeadershipIsReleasedWhenTheLoopEnds(t *testing.T) {
	held := &leadership{lock: &lock{}, name: "a"}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	done := make(chan struct{})
	concurrency.Go(ctx, "test.scheduler.loop", func(context.Context) {
		scheduler(held, &depthQueue{}, newSchedulerSignals()).Run(ctx)
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the scheduler is still running after its context ended")
	}
	if !held.released {
		t.Error("the scheduler kept leadership after shutting down")
	}
}

func TestAContradictorySchedulerDoesNotStart(t *testing.T) {
	valid := scheduler(&leadership{lock: &lock{}, name: "a"}, &depthQueue{}, newSchedulerSignals())

	cases := []struct {
		name string
		with func(Scheduler) Scheduler
		want string
	}{
		{"without its ports", func(s Scheduler) Scheduler { s.Leadership = nil; return s }, "queue.scheduler_incomplete"},
		{"without a tick", func(s Scheduler) Scheduler { s.TickInterval = 0; return s }, "queue.tick_interval_invalid"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.with(valid).validate()
			if err == nil || shared.AsError(err).DetailCode != c.want {
				t.Errorf("error %v, want %s", err, c.want)
			}
		})
	}
}

// The partition duty (E-09, audit.md §3). A partition of `audit_log` inherits neither the parent's
// policy nor its revokes when it is addressed directly, so one created without them is a
// cross-tenant leak with a date on it - and the leader is what makes sure next month's exists
// before the first entry of it does.

type auditPartitions struct {
	asked   []time.Time
	missing bool
	err     error
}

func (p *auditPartitions) Ensure(_ context.Context, month time.Time) (string, error) {
	p.asked = append(p.asked, month)
	if p.err != nil {
		return "", p.err
	}
	if p.missing {
		return "", nil
	}
	return "audit_log_" + month.Format("2006_01"), nil
}

func TestTheLeaderEnsuresThisMonthAndTheNext(t *testing.T) {
	partitions := &auditPartitions{}
	leader := scheduler(&leadership{lock: &lock{}, name: "a"}, &depthQueue{}, newSchedulerSignals())
	leader.AuditPartitions = partitions

	leader.tick(t.Context(), false, now)

	if len(partitions.asked) != 2 {
		t.Fatalf("the duty asked for %d months", len(partitions.asked))
	}
	if partitions.asked[0].Format("2006-01") != now.UTC().Format("2006-01") {
		t.Errorf("the first month asked for was %s", partitions.asked[0])
	}
	// Next month as well, because a month whose entries have already gone into the default
	// partition cannot be split out afterwards.
	if partitions.asked[1].Format("2006-01") != now.UTC().AddDate(0, 1, 0).Format("2006-01") {
		t.Errorf("the second month asked for was %s", partitions.asked[1])
	}
}

// A standby does nothing, including this: a partition duty run by every replica would be a DDL
// statement per replica per tick.
func TestAStandbyEnsuresNoPartitions(t *testing.T) {
	contested := &lock{}
	partitions := &auditPartitions{}

	leader := scheduler(&leadership{lock: contested, name: "a"}, &depthQueue{}, newSchedulerSignals())
	leader.AuditPartitions = &auditPartitions{}
	leader.tick(t.Context(), false, now)

	standby := scheduler(&leadership{lock: contested, name: "b"}, &depthQueue{}, newSchedulerSignals())
	standby.AuditPartitions = partitions
	standby.tick(t.Context(), false, now)

	if len(partitions.asked) != 0 {
		t.Errorf("a standby ensured %d partitions", len(partitions.asked))
	}
}

// A month already in the default partition is not a failure of the duty. Saying so every minute
// would be a duty somebody switches off, and moving rows out of a default partition is an
// operator's decision about a table that must not be rewritten casually.
func TestAMonthAlreadyInTheDefaultPartitionIsNotAFailure(t *testing.T) {
	partitions := &auditPartitions{missing: true}
	leader := scheduler(&leadership{lock: &lock{}, name: "a"}, &depthQueue{}, newSchedulerSignals())
	leader.AuditPartitions = partitions

	if leading := leader.tick(t.Context(), false, now); !leading {
		t.Error("the leader gave up its tick over a partition it could not create")
	}
	if len(partitions.asked) != 2 {
		t.Errorf("the duty stopped after %d months", len(partitions.asked))
	}
}

// An installation without the duty keeps writing into the default partition, which carries the
// same policy and the same revokes - so the scheduler runs without one rather than refusing to.
func TestASchedulerWithoutThePartitionDutyStillTicks(t *testing.T) {
	leader := scheduler(&leadership{lock: &lock{}, name: "a"}, &depthQueue{}, newSchedulerSignals())

	if leading := leader.tick(t.Context(), false, now); !leading {
		t.Error("a scheduler with no partition duty did not tick")
	}
}

// streamPartitions answers the stream duty's asks, table by table.
type streamPartitions struct {
	asked map[string][]time.Time
}

func (p *streamPartitions) Ensure(_ context.Context, table string, month time.Time) (string, error) {
	if p.asked == nil {
		p.asked = map[string][]time.Time{}
	}
	p.asked[table] = append(p.asked[table], month)
	return table + "_" + month.Format("2006_01"), nil
}

func (p *streamPartitions) DropAged(context.Context, string, time.Time) ([]streams.Dropped, error) {
	return nil, nil
}

// The stream duty covers all three tables, this month and next - the audit duty's contract,
// three times over (H-09).
func TestTheLeaderEnsuresEveryStreamsComingMonths(t *testing.T) {
	partitions := &streamPartitions{}
	leader := scheduler(&leadership{lock: &lock{}, name: "a"}, &depthQueue{}, newSchedulerSignals())
	leader.StreamPartitions = partitions

	leader.tick(t.Context(), false, now)

	for _, table := range streams.Tables() {
		months := partitions.asked[table]
		if len(months) != 2 {
			t.Fatalf("%s was asked for %d months", table, len(months))
		}
		if months[1].Format("2006-01") != now.UTC().AddDate(0, 1, 0).Format("2006-01") {
			t.Errorf("%s's second month was %s", table, months[1])
		}
	}
}

// A standby runs no stream duty either.
func TestAStandbyEnsuresNoStreamPartitions(t *testing.T) {
	contested := &lock{}
	partitions := &streamPartitions{}
	holder := scheduler(&leadership{lock: contested, name: "holder"}, &depthQueue{}, newSchedulerSignals())
	standby := scheduler(&leadership{lock: contested, name: "standby"}, &depthQueue{}, newSchedulerSignals())
	standby.StreamPartitions = partitions

	holder.tick(t.Context(), false, now)
	standby.tick(t.Context(), false, now)

	if len(partitions.asked) != 0 {
		t.Errorf("a standby ensured %v", partitions.asked)
	}
}
