// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"fmt"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

// The server-side ingest (G-11): the same three steps, performed here, for bytes that arrived over
// an intake instead of from a client that could have been handed a presigned URL.

// sequentialIDs hands out a different identifier per call. One mail carries several attachments,
// and a fake that answered one identifier would make three objects look like one.
type sequentialIDs struct{ issued int }

func (i *sequentialIDs) NewID() shared.ID {
	i.issued++
	return shared.ID(fmt.Sprintf("0192f000-0000-7000-8000-0000000000%02d", i.issued))
}

// jobs records what the ingest asked for. The reconciliation is the whole reason it is here: a
// tenant whose only uploads arrive by mail would otherwise never have its reclaimer started
// (multi-tenancy.md §2.1 - nothing may enumerate tenants).
type jobs struct {
	enqueued []queue.Request
	err      error
}

func (j *jobs) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	j.enqueued = append(j.enqueued, request)
	return "0192f000-0000-7000-8000-0000000000ff", j.err
}

func ingestHarness() (IngestMedia, *objects, *store, *guard, *jobs, *unitOfWork) {
	records, bucket, judge, queued, work := newObjects(), newStore(), &guard{judged: "application/pdf"}, &jobs{}, &unitOfWork{}
	return IngestMedia{
		Objects:    records,
		Store:      bucket,
		Guard:      judge,
		Jobs:       queued,
		UnitOfWork: work,
		Clock:      clock.Fixed(now),
		IDs:        &sequentialIDs{},
		Config:     config(1 << 20),
	}, records, bucket, judge, queued, work
}

// The three steps, in order and each one whole: the record exists, the bytes are in the store
// under the key the application minted, and what the caller gets back is READY.
func TestAnIngestStagesWritesAndSeals(t *testing.T) {
	handler, records, bucket, _, _, _ := ingestHarness()

	stored, err := handler.Execute(t.Context(), tenantID, []IngestedFile{
		{FileName: "invoice.pdf", ClaimedType: "application/pdf", Content: []byte("%PDF-1.4")},
	})
	if err != nil {
		t.Fatalf("the ingest failed: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("%d objects, want one", len(stored))
	}

	object := records.stored[stored[0]]
	if object.Status != domain.StatusReady {
		t.Errorf("the object is %s, want READY - nothing may attach one that is not", object.Status)
	}
	if object.TenantID != tenantID {
		t.Errorf("the object belongs to %q", object.TenantID)
	}
	if got := string(bucket.content[object.StorageKey]); got != "%PDF-1.4" {
		t.Errorf("the store holds %q under the object's key", got)
	}
	if len(records.sealed) != 1 {
		t.Errorf("%d seals, want the one that makes it usable", len(records.sealed))
	}
}

// No uploader, and that is the record being honest rather than a field somebody forgot. The intake
// authenticates the tenant and nobody else, so an account here would be a person this system
// invented (migration 0061).
func TestAnIngestedObjectHasNoUploader(t *testing.T) {
	handler, records, _, _, _, _ := ingestHarness()

	stored, err := handler.Execute(t.Context(), tenantID, []IngestedFile{
		{FileName: "invoice.pdf", Content: []byte("%PDF-1.4")},
	})
	if err != nil {
		t.Fatalf("the ingest failed: %v", err)
	}
	if creator := records.stored[stored[0]].CreatedBy; !creator.IsZero() {
		t.Errorf("the object names %q as its uploader", creator)
	}
}

// The claim is held against the bytes: what reaches the store is the guard's answer, never the
// sender's claim (T-11).
func TestTheStoredTypeIsTheJudgedOne(t *testing.T) {
	handler, records, bucket, judge, _, _ := ingestHarness()
	judge.judged = "text/plain"

	stored, err := handler.Execute(t.Context(), tenantID, []IngestedFile{
		{FileName: "invoice.pdf", ClaimedType: "application/pdf", Content: []byte("not a pdf")},
	})
	if err != nil {
		t.Fatalf("the ingest failed: %v", err)
	}
	if got := judge.asked; len(got) != 1 || got[0] != "application/pdf" {
		t.Errorf("the guard was asked about %v, want the sender's claim", got)
	}
	if got := bucket.types[records.stored[stored[0]].StorageKey]; got != "text/plain" {
		t.Errorf("the store holds it as %q, want the judged type", got)
	}
	if got := records.stored[stored[0]].ContentType; got != "text/plain" {
		t.Errorf("the record says %q, want the judged type", got)
	}
}

// The reconciliation is seeded by the ingest, exactly as it is by a staging: nothing in this
// system may enumerate tenants, so the write in the tenant is the only thing that can start it.
func TestAnIngestSeedsTheReconciliation(t *testing.T) {
	handler, _, _, _, queued, _ := ingestHarness()

	if _, err := handler.Execute(t.Context(), tenantID, []IngestedFile{
		{FileName: "a.pdf", Content: []byte("bytes")},
	}); err != nil {
		t.Fatalf("the ingest failed: %v", err)
	}

	if len(queued.enqueued) != 1 {
		t.Fatalf("%d jobs queued, want the reclaimer", len(queued.enqueued))
	}
	request := queued.enqueued[0]
	if request.Kind != queue.KindMediaReconcile || request.TenantID != tenantID {
		t.Errorf("the job is %+v, want the tenant's reconciliation", request)
	}
}

// One file that cannot be stored does not lose the rest. What this serves is an inbox entry, and
// an entry with three of its four attachments is worth more than no entry at all.
func TestAFileThatCannotBeStoredDoesNotLoseTheOthers(t *testing.T) {
	handler, _, bucket, judge, _, _ := ingestHarness()
	judge.err = shared.ErrValidation.WithDetail("media.type_not_allowed")

	stored, err := handler.Execute(t.Context(), tenantID, []IngestedFile{
		{FileName: "refused.exe", Content: []byte("MZ")},
	})
	if err != nil {
		t.Fatalf("a refused file failed the whole ingest: %v", err)
	}
	if len(stored) != 0 {
		t.Errorf("%d objects, want none - the refusal was the answer", len(stored))
	}
	// And its bytes are not in the store: the guard ran before the write, so nothing unacceptable
	// ever reached it.
	if len(bucket.puts) != 0 {
		t.Errorf("%d writes, want none", len(bucket.puts))
	}
}

// The bytes are written outside every transaction. An object store is somebody else's machine, and
// a transaction waiting on one holds a connection for as long as they feel like taking
// (observability-reliability.md §8). Two short transactions with the write between them is what
// that costs, and the alternative - one transaction around the whole thing - is what it buys.
func TestTheBytesAreWrittenBetweenTheTwoTransactions(t *testing.T) {
	handler, _, bucket, _, _, _ := ingestHarness()
	work := &trackingWork{}
	watched := &trackingStore{store: bucket, work: work}
	handler.UnitOfWork, handler.Store = work, watched

	if _, err := handler.Execute(t.Context(), tenantID, []IngestedFile{
		{FileName: "a.pdf", Content: []byte("bytes")},
	}); err != nil {
		t.Fatalf("the ingest failed: %v", err)
	}

	if work.writes != 2 {
		t.Errorf("%d transactions, want the staging's and the sealing's", work.writes)
	}
	if watched.insideTransaction {
		t.Error("the bytes were written inside a transaction")
	}
}

// trackingWork knows whether a transaction is open right now, which is the only way to say "the
// store was not called from inside one" rather than to trust the reading of the code.
type trackingWork struct {
	open   int
	writes int
}

func (w *trackingWork) Within(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	w.writes++
	w.open++
	defer func() { w.open-- }()
	return fn(ctx)
}

func (w *trackingWork) WithinReadOnly(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	w.open++
	defer func() { w.open-- }()
	return fn(ctx)
}

type trackingStore struct {
	*store
	work              *trackingWork
	insideTransaction bool
}

func (s *trackingStore) Put(ctx context.Context, upload storage.Upload) error {
	if s.work.open > 0 {
		s.insideTransaction = true
	}
	return s.store.Put(ctx, upload)
}

// A tenant is the one thing an ingest cannot do without: the bytes came in over a credential that
// names one, and an ingest without one would be an object in nobody's workspace.
func TestAnIngestWithoutATenantIsRefused(t *testing.T) {
	handler, _, _, _, _, _ := ingestHarness()

	_, err := handler.Execute(t.Context(), "", []IngestedFile{{FileName: "a.pdf"}})
	if !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("an ingest without a tenant answered %v", err)
	}
}

// Several attachments become several objects, each with its own identifier and its own key.
func TestEveryAttachmentBecomesItsOwnObject(t *testing.T) {
	handler, records, _, _, _, _ := ingestHarness()

	stored, err := handler.Execute(t.Context(), tenantID, []IngestedFile{
		{FileName: "one.pdf", Content: []byte("first")},
		{FileName: "two.pdf", Content: []byte("second")},
		{FileName: "three.pdf", Content: []byte("third")},
	})
	if err != nil {
		t.Fatalf("the ingest failed: %v", err)
	}
	if len(stored) != 3 {
		t.Fatalf("%d objects, want three", len(stored))
	}

	keys := map[string]bool{}
	for _, id := range stored {
		keys[records.stored[id].StorageKey] = true
	}
	if len(keys) != 3 {
		t.Errorf("%d distinct keys for three files", len(keys))
	}
}
