// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	auditrepo "github.com/Jersyfi/hubtask/core/application/repository/audit"
	auditservice "github.com/Jersyfi/hubtask/core/application/service/audit"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// trailRetention is the trail's partition drop, as the leader sees it.
type trailRetention struct {
	calls int
	days  int
	drops []auditrepo.TrailDrop
	err   error
}

func (r *trailRetention) DropAged(_ context.Context, defaultDays int) ([]auditrepo.TrailDrop, error) {
	r.calls++
	r.days = defaultDays
	return r.drops, r.err
}

// enqueueingQueue keeps what the leader queued.
type enqueueingQueue struct {
	depthQueue
	requests []queue.Request
}

func (q *enqueueingQueue) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	q.requests = append(q.requests, request)
	return "", nil
}

// droppedPartitions keeps what the instance journal was told.
type droppedPartitions struct{ rows map[string]int64 }

func (d *droppedPartitions) PartitionDropped(_ context.Context, table, partition string, rows int64) error {
	if d.rows == nil {
		d.rows = map[string]int64{}
	}
	d.rows[table+"/"+partition] += rows
	return nil
}

// The trail's aged months, kept by the leader once an hour with the catalogue's period as floor:
// each workspace in a month that went gets its own entry queued, the instance journal gets the
// month's total, and the held months and the default's aged entries are reported.
func TestTheLeaderDropsTheTrailsAgedMonthsHourly(t *testing.T) {
	first, second := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000a1"),
		shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000a2")
	month := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	trail := &trailRetention{drops: []auditrepo.TrailDrop{
		{Partition: "audit_log_2025_05", Month: month, Workspace: first, Entries: 4, Outcome: auditrepo.TrailDropped},
		{Partition: "audit_log_2025_05", Month: month, Workspace: second, Entries: 1, Outcome: auditrepo.TrailDropped},
		{Partition: "audit_log_2025_06", Month: month.AddDate(0, 1, 0), Outcome: auditrepo.TrailHeld},
		{Partition: "audit_log_default", Entries: 9, Outcome: auditrepo.TrailDefaultAged},
	}}
	jobs := &enqueueingQueue{}
	journal := &droppedPartitions{}
	signals := newSchedulerSignals()
	leader := scheduler(&leadership{lock: &lock{}, name: "a"}, jobs, signals)
	leader.AuditTrail = trail
	leader.StreamEvidence = journal

	leader.tick(t.Context(), false, now) // 12:00:00, the hour's first tick
	audit, _ := lifecycle.FindKind(lifecycle.KindAudit)
	if trail.calls != 1 || trail.days != audit.DefaultDays {
		t.Fatalf("dropped %d times with a floor of %d days, want once with %d", trail.calls, trail.days, audit.DefaultDays)
	}
	if len(jobs.requests) != 2 {
		t.Fatalf("queued %d entries, want one per workspace", len(jobs.requests))
	}
	for i, workspace := range []shared.ID{first, second} {
		request := jobs.requests[i]
		removal, err := auditservice.TrailRemovalOf(request.Payload, request.TenantID)
		if request.Kind != queue.KindAuditTrailRemoved || request.TenantID != workspace || err != nil ||
			removal.Month != "2025-05" || removal.Partition != "audit_log_2025_05" {
			t.Errorf("queued %+v (%v)", request, err)
		}
	}
	if jobs.requests[0].DedupeKey == jobs.requests[1].DedupeKey {
		t.Error("two workspaces share one dedupe key")
	}
	if journal.rows["audit_log/audit_log_2025_05"] != 5 || len(journal.rows) != 1 {
		t.Errorf("the instance journal was told %v", journal.rows)
	}
	if len(signals.auditHeld) != 1 || signals.auditHeld[0] != 1 ||
		len(signals.auditDefaultAged) != 1 || signals.auditDefaultAged[0] != 9 {
		t.Errorf("held %v, default aged %v", signals.auditHeld, signals.auditDefaultAged)
	}

	leader.tick(t.Context(), true, now.Add(10*time.Second)) // the same hour
	if trail.calls != 1 {
		t.Errorf("dropped %d times within one hour", trail.calls)
	}
}

// A drop that fails queues nothing, tells the journal nothing and costs no leadership; the next
// hour tries again.
func TestAFailedTrailDropDoesNotStopTheLeader(t *testing.T) {
	jobs := &enqueueingQueue{}
	journal := &droppedPartitions{}
	signals := newSchedulerSignals()
	leader := scheduler(&leadership{lock: &lock{}, name: "a"}, jobs, signals)
	leader.AuditTrail = &trailRetention{err: shared.ErrUnavailable.WithDetail("postgres.query_failed")}
	leader.StreamEvidence = journal

	if leading := leader.tick(t.Context(), false, now); !leading {
		t.Fatal("a failed drop cost the leadership")
	}
	if len(jobs.requests) != 0 || len(journal.rows) != 0 || len(signals.auditHeld) != 0 {
		t.Errorf("a failed drop left traces: %v %v %v", jobs.requests, journal.rows, signals.auditHeld)
	}
}

// An owner that cannot read across workspaces drops nothing, and the leader reports nothing as
// dropped.
func TestAnUnreadableTrailIsNotADrop(t *testing.T) {
	jobs := &enqueueingQueue{}
	journal := &droppedPartitions{}
	leader := scheduler(&leadership{lock: &lock{}, name: "a"}, jobs, newSchedulerSignals())
	leader.AuditTrail = &trailRetention{drops: []auditrepo.TrailDrop{{Outcome: auditrepo.TrailUnreadable}}}
	leader.StreamEvidence = journal

	leader.tick(t.Context(), false, now)
	if len(jobs.requests) != 0 || len(journal.rows) != 0 {
		t.Errorf("an unreadable trail was treated as dropped: %v %v", jobs.requests, journal.rows)
	}
}
