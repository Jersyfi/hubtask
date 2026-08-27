// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/media"
	lifecycledomain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	mediadomain "github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

// The handler's own job is small and worth pinning down: the tenant it runs for, when it comes
// back, and that it declares itself detached. Everything the pass does is the application layer's,
// and is tested there.

// reclaimable is the media record store as one pass sees it. Only the methods a pass calls do
// anything; the rest exist because the port is one interface.
type reclaimable struct{ orphans []mediarepo.Orphan }

func (r *reclaimable) Recount(context.Context, time.Time) error { return nil }
func (r *reclaimable) MarkOrphans(
	context.Context, time.Time, mediarepo.Thresholds,
) (int, error) {
	return len(r.orphans), nil
}

func (r *reclaimable) TakeOrphans(
	_ context.Context, _ time.Time, batch int,
) ([]mediarepo.Orphan, error) {
	if len(r.orphans) > batch {
		return r.orphans[:batch], nil
	}
	return r.orphans, nil
}

func (r *reclaimable) RemoveRows(_ context.Context, ids []shared.ID) (int, error) {
	r.orphans = nil
	return len(ids), nil
}

func (r *reclaimable) Insert(context.Context, mediadomain.Object) error { return nil }
func (r *reclaimable) Find(context.Context, shared.ID) (mediadomain.Object, error) {
	return mediadomain.Object{}, shared.ErrNotFound
}
func (r *reclaimable) Seal(context.Context, mediadomain.Object) error       { return nil }
func (r *reclaimable) AdjustRefCount(context.Context, shared.ID, int) error { return nil }
func (r *reclaimable) MarkDeleted(context.Context, shared.ID, time.Time) (bool, error) {
	return false, nil
}
func (r *reclaimable) ReferencingItems(
	context.Context, shared.ID,
) ([]mediarepo.ItemRef, error) {
	return nil, nil
}
func (r *reclaimable) ListForItem(
	context.Context, shared.ID, workrepo.Page,
) (mediarepo.ObjectPage, error) {
	return mediarepo.ObjectPage{}, nil
}

// forgivingStore lets go of everything it is asked about, which is also what a real one does for
// bytes that are not there.
type forgivingStore struct{}

func (forgivingStore) Put(context.Context, storage.Upload) error { return nil }
func (forgivingStore) Get(context.Context, string) (storage.Object, error) {
	return storage.Object{}, shared.ErrNotFound
}
func (forgivingStore) Delete(context.Context, string) error { return nil }

type quietRemovals struct{}

func (quietRemovals) Record(
	context.Context, []lifecycledomain.Removal, time.Time, time.Time,
) error {
	return nil
}

// directWork runs the callback without a transaction, which is enough for a handler test: whether
// the pass opens the right ones is the application layer's test.
type directWork struct{ opened int }

func (d *directWork) Within(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	d.opened++
	return fn(ctx)
}

func (d *directWork) WithinReadOnly(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	return fn(ctx)
}

func reconciliationHandler(orphans int) (MediaReconciliation, *reclaimable) {
	records := &reclaimable{}
	for i := range orphans {
		records.orphans = append(records.orphans, mediarepo.Orphan{
			ID: shared.ID(sweepTenant.String()), StorageKey: "media/x/" + string(rune('a'+i)),
		})
	}

	return MediaReconciliation{
		Reconciliation: media.ReconcileMedia{
			Objects: records, Store: forgivingStore{}, Removals: quietRemovals{},
			UnitOfWork: &directWork{}, Clock: clock.Fixed(time.Unix(0, 0).UTC()),
			Config: env.MediaConfig{
				StagingGrace: 24 * time.Hour, OrphanGrace: time.Hour, BatchSize: 2,
			},
		},
		Interval:     time.Hour,
		Continuation: time.Second,
	}, records
}

// The interface is the declaration, and the runner reads it by type assertion. A handler that
// stopped satisfying it would silently start running inside a transaction it must not be in.
func TestTheReconciliationDeclaresItselfDetached(t *testing.T) {
	handler, _ := reconciliationHandler(0)

	if _, detached := any(handler).(queue.Detached); !detached {
		t.Error("the reconciliation no longer declares that it owns its transactions")
	}
}

func TestAQuietTenantComesBackAtTheInterval(t *testing.T) {
	handler, _ := reconciliationHandler(0)

	result, err := handler.Run(t.Context(), queue.Job{
		TenantID: sweepTenant, Kind: queue.KindMediaReconcile,
	})
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	// Never finished for good: the next abandoned staging is always coming, and a row that removed
	// itself would leave the tenant with no reconciliation until its next upload.
	if !result.Repeat || result.RepeatAfter != time.Hour {
		t.Errorf("the job comes back as %+v", result)
	}
}

func TestAFullBatchComesBackAtOnce(t *testing.T) {
	handler, _ := reconciliationHandler(2)

	result, err := handler.Run(t.Context(), queue.Job{
		TenantID: sweepTenant, Kind: queue.KindMediaReconcile,
	})
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if result.RepeatAfter != time.Second {
		t.Errorf("a pass with known work left waits %v", result.RepeatAfter)
	}
}

// Every transaction the pass opens is opened for the tenant the job names. Without one there is
// nothing to reconcile, and running anyway would be running as nobody.
func TestAJobWithoutATenantIsADefect(t *testing.T) {
	handler, _ := reconciliationHandler(0)

	_, err := handler.Run(t.Context(), queue.Job{Kind: queue.KindMediaReconcile})
	if detail := shared.AsError(err).DetailCode; detail != "media.reconcile_without_tenant" {
		t.Fatalf("detail %q, want media.reconcile_without_tenant", detail)
	}
}
