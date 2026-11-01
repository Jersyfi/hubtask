// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type clearedLines struct{ cleared []string }

func (l clearedLines) BeforeExpiry(context.Context, shared.ID, time.Time, int) ([]string, error) {
	return l.cleared, nil
}

type requesterTable struct {
	among   []string
	batched bool
}

func (r *requesterTable) CountExpired(context.Context, time.Time, int) (int, error) { return 3, nil }

func (r *requesterTable) DeleteExpired(context.Context, time.Time, int) (int, error) {
	r.batched = true
	return 3, nil
}

func (r *requesterTable) DeleteExpiredAmong(_ context.Context, _ time.Time, addresses []string) (int, error) {
	r.among = append(r.among, addresses...)
	return len(addresses), nil
}

// A REQUESTER batch removes only the requesters whose intake lines all went: one still named on an
// entry that cannot be changed now stays, so its address is never left in a note with no record.
func TestOnlyRequestersWhoseLinesWentAreRemoved(t *testing.T) {
	table := &requesterTable{}
	rows := requesterRows{rows: table, lines: clearedLines{cleared: []string{"a@shop.test"}}}
	removed, err := rows.DeleteExpired(context.Background(), time.Now(), 3)
	if err != nil || removed != 1 {
		t.Fatalf("removed %d, %v; want 1", removed, err)
	}
	if table.batched || !slices.Equal(table.among, []string{"a@shop.test"}) {
		t.Errorf("deleted the whole batch (%v) or others than the cleared: %v", table.batched, table.among)
	}

	table = &requesterTable{}
	rows = requesterRows{rows: table, lines: clearedLines{}}
	if removed, err := rows.DeleteExpired(context.Background(), time.Now(), 3); err != nil || removed != 0 || table.batched {
		t.Errorf("with every line kept, removed %d (batch %v), %v", removed, table.batched, err)
	}
}
