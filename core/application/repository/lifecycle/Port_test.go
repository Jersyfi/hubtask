// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// What this file proves is a shape rather than a behaviour: the two interfaces are implementable,
// and they are implementable in the way the deletion path needs them to be. The behaviour is proven
// against a real database in test/integration - a fake that agreed with the port would prove only
// that the fake agrees with the port.

// double is the smallest thing that satisfies both.
type double struct {
	holds    domain.Holds
	recorded []domain.Removal
	window   time.Duration
}

func (d *double) Active(context.Context) (domain.Holds, error) { return d.holds, nil }

func (d *double) Record(
	_ context.Context, removals []domain.Removal, deletedAt, purgeAfter time.Time,
) error {
	d.recorded = append(d.recorded, removals...)
	d.window = purgeAfter.Sub(deletedAt)
	return nil
}

var (
	_ LegalHolds = (*double)(nil)
	_ Removals   = (*double)(nil)
)

// One call covers both tables, because a purge of a hub removes containers and entries in one act.
// A port that took one table per call would put the grouping in every caller.
func TestOneRecordCoversBothTables(t *testing.T) {
	store := &double{}
	at := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)

	err := store.Record(t.Context(), []domain.Removal{
		{Entity: "container", EntityID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
			Reason: domain.DeletedByRetention},
		{Entity: "work_item", EntityID: shared.MustParseID("0192f000-0000-7000-8000-000000000001"),
			Reason: domain.DeletedByRetention},
	}, at, at.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("recording: %v", err)
	}

	if len(store.recorded) != 2 {
		t.Errorf("%d removals recorded, want 2", len(store.recorded))
	}
	// The window is expressed as the two instants rather than as a duration, so that the moment of
	// the removal and the moment the marker may go are the same reading of the clock - two readings
	// would let a slow batch date its own tombstones inconsistently.
	if store.window != 90*24*time.Hour {
		t.Errorf("the tombstone window is %v, want 90 days", store.window)
	}
}

// The holds come back as the domain's set type, so that the decision is taken once, in the domain,
// rather than in each caller that reads them.
func TestTheHoldsComeBackAsTheDomainsSet(t *testing.T) {
	store := &double{holds: domain.Holds{{Scope: domain.HoldTenant}}}

	holds, err := store.Active(t.Context())
	if err != nil {
		t.Fatalf("reading the holds: %v", err)
	}
	if _, blocked := holds.Blocking(domain.Target{}); !blocked {
		t.Error("a tenant-wide hold read back through the port did not block")
	}
}
