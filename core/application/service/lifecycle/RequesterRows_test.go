// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"slices"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type clearedLines struct {
	cleared []string
	kept    int
}

func (l clearedLines) BeforeExpiry(context.Context, shared.ID, time.Time, int) ([]string, int, error) {
	return l.cleared, l.kept, nil
}

type requesterTable struct {
	among   []string
	batched bool
	due     int
}

func (r *requesterTable) CountExpired(_ context.Context, _ time.Time, ceiling int) (int, error) {
	if r.due == 0 {
		return 3, nil
	}
	return min(r.due, ceiling), nil
}

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

// A REQUESTER batch whose requesters all stay is held, not a full batch: the job does not come
// straight back to find the same kept requesters again (RunRetention.Exhausted).
func TestKeptRequestersDoNotBringTheSweepStraightBack(t *testing.T) {
	cases := []struct {
		name      string
		cleared   []string
		kept      int
		exhausted bool
	}{
		{name: "every requester kept", kept: 4, exhausted: true},
		{name: "some went, the rest kept", cleared: []string{"a@shop.test"}, kept: 3, exhausted: true},
		{name: "a full batch went past kept ones", cleared: []string{"a@shop.test", "b@shop.test", "c@shop.test", "d@shop.test"}, kept: 2, exhausted: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newRunHarness().run
			h.Purger.BatchSize = 4
			table := &requesterTable{due: 10}
			rows := requesterRows{rows: table, lines: clearedLines{cleared: tc.cleared, kept: tc.kept}}
			outcome, err := h.sweepHeldUntil(context.Background(), now, domain.KindRequester, now, rows)
			if err != nil {
				t.Fatalf("sweep: %v", err)
			}
			if outcome.Removed != len(tc.cleared) {
				t.Errorf("removed %d, want %d", outcome.Removed, len(tc.cleared))
			}
			if got := h.Exhausted(outcome); got != tc.exhausted {
				t.Errorf("exhausted %v, want %v (outcome %+v)", got, tc.exhausted, outcome)
			}
		})
	}
}
