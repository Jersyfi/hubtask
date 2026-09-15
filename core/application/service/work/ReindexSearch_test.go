// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// searchIndex stands in for the rows' bookkeeping: how many are stale, and how a batch moves it.
type searchIndex struct {
	stale    int64
	rebuilds []int
}

func (s *searchIndex) Stale(context.Context) (int64, error) { return s.stale, nil }

func (s *searchIndex) Rebuild(_ context.Context, batch int) (int64, error) {
	s.rebuilds = append(s.rebuilds, batch)
	rewritten := min(int64(batch), s.stale)
	s.stale -= rewritten
	return rewritten, nil
}

func reindexFixture(stale int64) (ReindexSearch, *searchIndex, *jobs, *sink, *authorizer, *unitOfWork) {
	index := &searchIndex{stale: stale}
	queue := &jobs{}
	audit := &sink{}
	guard := &authorizer{}
	uow := &unitOfWork{}
	return ReindexSearch{
		Index: index, Jobs: queue, Authorizer: guard, Audit: audit, UnitOfWork: uow,
		Clock: clock.Fixed(now),
	}, index, queue, audit, guard, uow
}

// The ask: the permission that shapes the workspace, the count, one job per workspace, and the
// audit entry in the job's transaction - so an ask that was accepted and left no record cannot
// happen.
func TestAReindexCountsQueuesAndRecords(t *testing.T) {
	handler, _, queued, audit, guard, uow := reindexFixture(42)

	accepted, err := handler.Execute(t.Context(), actorFixture())
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if accepted.Stale != 42 || accepted.JobID.IsZero() {
		t.Errorf("answered %+v", accepted)
	}

	if len(guard.requests) != 1 {
		t.Fatalf("%d permission questions, want one", len(guard.requests))
	}
	request := guard.requests[0]
	if request.Permission != service.PermissionStructure || request.TokenScope != workspaceManage {
		t.Errorf("asked for %s / %q", request.Permission, request.TokenScope)
	}
	assertPath(t, request.Path, []identity.Scope{identity.TenantScope()})

	if len(queued.enqueued) != 1 {
		t.Fatalf("%d jobs queued, want one", len(queued.enqueued))
	}
	job := queued.enqueued[0]
	if job.Kind != queue.KindSearchReindex || job.TenantID != tenantID {
		t.Errorf("queued %+v", job)
	}
	if job.DedupeKey != "search-reindex:"+tenantID.String() {
		t.Errorf("the job is not deduplicated per workspace: %q", job.DedupeKey)
	}
	if stale, _ := job.Payload["stale"].(int64); stale != 42 {
		t.Errorf("the payload carries %v, want the count the progress is measured against", job.Payload["stale"])
	}

	if len(audit.entries) != 1 || audit.entries[0].Action != ReindexAskedAction {
		t.Errorf("audit entries %+v", audit.entries)
	}
	if uow.writes != 1 {
		t.Errorf("the ask opened %d write transactions, want one holding the job and the entry", uow.writes)
	}
}

// A member without the permission is refused before the count, the job and the entry.
func TestAReindexIsRefusedWithoutThePermission(t *testing.T) {
	handler, _, queued, audit, guard, _ := reindexFixture(7)
	guard.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := handler.Execute(t.Context(), actorFixture())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("answered %v", err)
	}
	if len(queued.enqueued) != 0 || len(audit.entries) != 0 {
		t.Error("a refused ask queued a job or recorded itself")
	}
}

// Nothing stale still queues the job: a walk that finds nothing left is the honest answer to
// "is it current", and its finished status is what a client polls for.
func TestNothingStaleStillQueuesTheJob(t *testing.T) {
	handler, _, queued, _, _, _ := reindexFixture(0)
	accepted, err := handler.Execute(t.Context(), actorFixture())
	if err != nil || accepted.Stale != 0 || len(queued.enqueued) != 1 {
		t.Errorf("%v %+v %d jobs", err, accepted, len(queued.enqueued))
	}
}

// The pass behind the job: one batch in its own transaction, and what is left.
func TestTheRebuildPassRewritesOneBatchAndCountsTheRest(t *testing.T) {
	index := &searchIndex{stale: 1200}
	uow := &unitOfWork{}
	pass := RebuildSearchIndex{Index: index, UnitOfWork: uow}

	first, err := pass.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the first pass: %v", err)
	}
	if first.Rewritten != defaultRebuildBatch || first.Remaining != 1200-defaultRebuildBatch {
		t.Errorf("the first pass: %+v", first)
	}
	if index.rebuilds[0] != defaultRebuildBatch {
		t.Errorf("the batch was %d", index.rebuilds[0])
	}

	pass.BatchSize = 1000
	second, err := pass.Execute(t.Context(), systemActor())
	if err != nil || second.Rewritten != 700 || second.Remaining != 0 {
		t.Errorf("the second pass: %v %+v", err, second)
	}
	if uow.writes != 2 {
		t.Errorf("%d write transactions, want one per pass", uow.writes)
	}
}
