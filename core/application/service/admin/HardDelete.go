// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

const journalHardDeleted = "tenant.hard_deleted"

// purgeBatch bounds one page of keys and one outbox delete. Small enough that no single
// statement holds a lock worth talking about; the loops around it do the volume.
const purgeBatch = 500

// HardDeleteOutcome is what one pass reports: counts, for the job's own log line and nothing
// else - the durable record is the evidence entry.
type HardDeleteOutcome struct {
	// Deleted is false when the guard held the act back: the deadline moved, the status is not
	// PENDING_DELETION any more, or the tenant is already gone. Nothing was changed then.
	Deleted      bool
	Footprint    adminrepo.Footprint
	BytesObjects int
	TrailEntries int64
	JobsRemoved  int
}

// HardDeleteTenant is §5's final act, run by the grace job the deletion request seeded (H-06).
//
// Not a use case in the registry: no credential can ask for it, only the clock. It deletes the
// media bytes store-first (ReconcileMedia's order), then in one transaction fells the structure,
// clears what no cascade reaches, writes the evidence entry, purges the trail through migration
// 0067's narrow act, and lets the tenant row's cascade take the rest - evidence and act commit
// together, so there is never a deletion without evidence.
//
// It is safe to run twice: the byte deletion tolerates absent objects, and the final guard
// re-reads the two facts the grace could have changed before anything falls.
type HardDeleteTenant struct {
	Tenants    adminrepo.Tenants
	Purge      adminrepo.Purge
	Journal    adminrepo.Journal
	Store      storage.ObjectStore
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// Execute performs one hard delete. jobID names the queue row running this pass, which survives
// to report its own completion.
func (h HardDeleteTenant) Execute(
	ctx context.Context, tenantID, jobID shared.ID,
) (HardDeleteOutcome, error) {
	scope := persistence.Scope{TenantID: tenantID}
	outcome := HardDeleteOutcome{}

	// The guard, and the counts the evidence will record. Read first and separately: the byte
	// deletion between this and the final transaction must not run for a tenant the operator's
	// clock has not released.
	var record adminrepo.TenantRecord
	err := h.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		found, err := h.Tenants.Find(ctx)
		if err != nil {
			return err
		}
		record = found
		footprint, err := h.Purge.Footprint(ctx)
		outcome.Footprint = footprint
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// Already gone - the second run of a pass that died between its commit and its
			// completion report. The deletion happened; this pass has nothing to add.
			return HardDeleteOutcome{}, nil
		}
		return outcome, err
	}
	now := h.Clock.Now()
	if record.Status != domain.TenantPendingDeletion ||
		record.PurgeAfter.IsZero() || record.PurgeAfter.After(now) {
		return outcome, nil
	}

	// The bytes, store-first and outside any transaction (observability-reliability.md §8
	// forbids holding one across a bucket call). Deleting what is absent succeeds, which is what
	// makes a second pass safe.
	deleted, err := h.discardBytes(ctx, scope)
	outcome.BytesObjects = deleted
	if err != nil {
		return outcome, err
	}

	// The fall, in one transaction: evidence and act commit together.
	err = h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if _, err := h.Purge.DropStructure(ctx); err != nil {
			return err
		}
		for {
			removed, err := h.Purge.DeleteOutbox(ctx, purgeBatch)
			if err != nil {
				return err
			}
			if removed == 0 {
				break
			}
		}
		if _, err := h.Purge.DeleteIdempotency(ctx); err != nil {
			return err
		}
		jobsRemoved, err := h.Purge.DeleteJobs(ctx, jobID)
		if err != nil {
			return err
		}
		outcome.JobsRemoved = jobsRemoved

		trailEntries, err := h.Purge.PurgeTrail(ctx)
		if err != nil {
			return err
		}
		outcome.TrailEntries = trailEntries

		if err := h.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: h.IDs.NewID(), OccurredAt: now,
			Action: journalHardDeleted, TenantID: tenantID, TenantSlug: record.Slug,
			Details: map[string]any{
				"items":         outcome.Footprint.Items,
				"containers":    outcome.Footprint.Containers,
				"media_objects": outcome.Footprint.MediaObjects,
				"media_bytes":   outcome.Footprint.MediaBytes,
				"outbox_events": outcome.Footprint.OutboxEvents,
				"trail_entries": trailEntries,
				"jobs_removed":  jobsRemoved,
				"purge_after":   record.PurgeAfter.Format(time.RFC3339),
			},
		}); err != nil {
			return err
		}

		removed, err := h.Purge.HardDelete(ctx, now)
		if err != nil {
			return err
		}
		if !removed {
			// The state moved between the guard and this write. Roll the whole act back -
			// including the evidence, which would otherwise attest a deletion that did not
			// happen.
			return shared.ErrConflict.WithDetail("admin.tenant_leaving")
		}
		outcome.Deleted = true
		return nil
	})
	if err != nil {
		return outcome, err
	}
	return outcome, nil
}

// discardBytes walks the tenant's object keys page by page and deletes store-first. Each page is
// its own read transaction; the store calls sit between them.
func (h HardDeleteTenant) discardBytes(
	ctx context.Context, scope persistence.Scope,
) (int, error) {
	deleted := 0
	after := ""
	for {
		var keys []string
		err := h.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
			page, err := h.Purge.StorageKeys(ctx, after, purgeBatch)
			keys = page
			return err
		})
		if err != nil {
			return deleted, err
		}
		if len(keys) == 0 {
			return deleted, nil
		}
		for _, key := range keys {
			if err := h.Store.Delete(ctx, key); err != nil {
				return deleted, err
			}
			deleted++
		}
		after = keys[len(keys)-1]
	}
}
