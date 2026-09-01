// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

var purgeJobID = shared.ID("018f2a1b-0000-7000-8000-0000000000c1")

type purgeFake struct {
	footprint    adminrepo.Footprint
	keys         []string
	structure    bool
	outboxRounds int
	idempotency  bool
	jobsKeep     shared.ID
	trailRemoved int64
	trailCalls   int
	deleteOK     bool
	deleteCalls  int
	sequence     []string
}

func (p *purgeFake) Footprint(context.Context) (adminrepo.Footprint, error) {
	return p.footprint, nil
}

func (p *purgeFake) StorageKeys(_ context.Context, after string, _ int) ([]string, error) {
	remaining := make([]string, 0, len(p.keys))
	for _, key := range p.keys {
		if key > after {
			remaining = append(remaining, key)
		}
	}
	return remaining, nil
}

func (p *purgeFake) DropStructure(context.Context) (int64, error) {
	p.structure = true
	p.sequence = append(p.sequence, "structure")
	return 42, nil
}

func (p *purgeFake) DeleteOutbox(context.Context, int) (int, error) {
	p.sequence = append(p.sequence, "outbox")
	if p.outboxRounds == 0 {
		return 0, nil
	}
	p.outboxRounds--
	return 7, nil
}

func (p *purgeFake) DeleteIdempotency(context.Context) (int, error) {
	p.idempotency = true
	return 1, nil
}

func (p *purgeFake) DeleteJobs(_ context.Context, keep shared.ID) (int, error) {
	p.jobsKeep = keep
	return 2, nil
}

func (p *purgeFake) PurgeTrail(context.Context) (int64, error) {
	p.trailCalls++
	p.sequence = append(p.sequence, "trail")
	return p.trailRemoved, nil
}

func (p *purgeFake) HardDelete(context.Context, time.Time) (bool, error) {
	p.deleteCalls++
	p.sequence = append(p.sequence, "tenant_row")
	return p.deleteOK, nil
}

type storeFake struct{ deleted []string }

func (s *storeFake) Put(context.Context, storage.Upload) error { return nil }

func (s *storeFake) Get(context.Context, string) (storage.Object, error) {
	return storage.Object{}, shared.ErrNotFound.WithDetail("storage.object_missing")
}

func (s *storeFake) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

type hardDeleteFixture struct {
	handler HardDeleteTenant
	tenants *tenantsStore
	purge   *purgeFake
	journal *journalStore
	store   *storeFake
	work    *unitOfWork
}

func newHardDeleteFixture(status domain.TenantStatus, purgeAfter time.Time) *hardDeleteFixture {
	f := &hardDeleteFixture{
		tenants: &tenantsStore{record: adminrepo.TenantRecord{
			ID: lifecycleTenant, Slug: "acme", DisplayName: "Acme GmbH",
			Status: status, PurgeAfter: purgeAfter,
		}},
		purge: &purgeFake{
			footprint: adminrepo.Footprint{
				Items: 100, Containers: 5, MediaObjects: 2, MediaBytes: 4096,
				OutboxEvents: 14, AuditEntries: 900,
			},
			keys: []string{"media/t/a", "media/t/b"}, outboxRounds: 2,
			trailRemoved: 900, deleteOK: true,
		},
		journal: &journalStore{}, store: &storeFake{}, work: &unitOfWork{},
	}
	f.handler = HardDeleteTenant{
		Tenants: f.tenants, Purge: f.purge, Journal: f.journal, Store: f.store,
		UnitOfWork: f.work, Clock: clock.Fixed(now), IDs: &sequentialIDs{},
	}
	return f
}

func TestTheHardDeleteFellsEveryStoreAndLeavesTheEvidence(t *testing.T) {
	f := newHardDeleteFixture(domain.TenantPendingDeletion, now.Add(-time.Hour))

	outcome, err := f.handler.Execute(t.Context(), lifecycleTenant, purgeJobID)
	if err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if !outcome.Deleted {
		t.Fatal("the guard held a due deletion back")
	}

	// Bytes store-first, both objects.
	if len(f.store.deleted) != 2 || outcome.BytesObjects != 2 {
		t.Errorf("bytes %v (%d)", f.store.deleted, outcome.BytesObjects)
	}
	if !f.purge.structure || !f.purge.idempotency || f.purge.jobsKeep != purgeJobID {
		t.Errorf("purge state %+v", f.purge)
	}
	if f.purge.trailCalls != 1 || outcome.TrailEntries != 900 {
		t.Errorf("trail %d calls, %d entries", f.purge.trailCalls, outcome.TrailEntries)
	}

	// The evidence entry, with the counts of every store, committed with the act.
	if len(f.journal.entries) != 1 {
		t.Fatalf("journal %+v", f.journal.entries)
	}
	entry := f.journal.entries[0]
	if entry.Action != "tenant.hard_deleted" || entry.TenantID != lifecycleTenant ||
		entry.TenantSlug != "acme" {
		t.Errorf("entry %+v", entry)
	}
	for field, want := range map[string]any{
		"items": int64(100), "media_bytes": int64(4096), "trail_entries": int64(900),
	} {
		if entry.Details[field] != want {
			t.Errorf("evidence %s = %v, want %v", field, entry.Details[field], want)
		}
	}

	// The trail falls after the evidence is written and before the row: an order in which no
	// committed state has a deletion without evidence.
	sequence := f.purge.sequence
	if sequence[len(sequence)-1] != "tenant_row" || sequence[0] != "structure" {
		t.Errorf("sequence %v", sequence)
	}
}

func TestTheGuardHoldsAnUnduePurgeBack(t *testing.T) {
	cases := []struct {
		name   string
		status domain.TenantStatus
		after  time.Time
	}{
		{"a resumed tenant", domain.TenantActive, now.Add(-time.Hour)},
		{"a deadline still running", domain.TenantPendingDeletion, now.Add(time.Hour)},
		{"a deadline nobody stamped", domain.TenantPendingDeletion, time.Time{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newHardDeleteFixture(c.status, c.after)

			outcome, err := f.handler.Execute(t.Context(), lifecycleTenant, purgeJobID)
			if err != nil {
				t.Fatalf("the guard errored: %v", err)
			}
			if outcome.Deleted || len(f.store.deleted) != 0 || f.purge.structure ||
				len(f.journal.entries) != 0 {
				t.Error("something fell for an undue purge")
			}
		})
	}
}

func TestAnAlreadyGoneTenantIsAQuietSecondPass(t *testing.T) {
	f := newHardDeleteFixture(domain.TenantPendingDeletion, now.Add(-time.Hour))
	f.tenants.findErr = shared.ErrNotFound.WithDetail("admin.tenant_not_found")

	outcome, err := f.handler.Execute(t.Context(), lifecycleTenant, purgeJobID)
	if err != nil || outcome.Deleted {
		t.Errorf("the second pass answered (%+v, %v), want a quiet nothing", outcome, err)
	}
}

func TestAStateThatMovedRollsTheWholeActBack(t *testing.T) {
	f := newHardDeleteFixture(domain.TenantPendingDeletion, now.Add(-time.Hour))
	f.purge.deleteOK = false

	_, err := f.handler.Execute(t.Context(), lifecycleTenant, purgeJobID)
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("answer %v, want the conflict that rolls the transaction back", err)
	}
}
