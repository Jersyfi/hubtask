// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The slim entry (F-10, versioning-release.md §4): every writer writes the side tables of 0234;
// the entry's own columns for these values are gone (0445).

type retentionRow struct {
	exists          bool
	pending         *time.Time
	rule            *string
	action, blocked *string
}

func retentionOf(ctx context.Context, t *testing.T, item shared.ID) retentionRow {
	t.Helper()
	var row retentionRow
	err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT pending_until, rule_id::text, action, blocked_by FROM work_item_retention WHERE item_id = $1`,
		item.String()).Scan(&row.pending, &row.rule, &row.action, &row.blocked)
	if errors.Is(err, pgx.ErrNoRows) {
		return row
	}
	if err != nil {
		t.Fatalf("reading the retention row: %v", err)
	}
	row.exists = true
	return row
}

func versionOf(ctx context.Context, t *testing.T, item shared.ID) int {
	t.Helper()
	var v int
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT version FROM work_item WHERE id = $1`, item.String()).
		Scan(&v); err != nil {
		t.Fatalf("reading the version: %v", err)
	}
	return v
}

// The retention statements mean what they meant on the columns: a marking counts down and is
// not marked again, a block holds only an unmarked entry and is not rewritten unchanged, a
// marking lifts a block, a clearing keeps the rule only for a stage that acted and spends a
// version, and a withdrawn rule takes its markings.
func TestTheRetentionWritersKeepTheirMeaningOnTheSideTable(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	clearRules(ctx, t, tenantA)
	rule := aRule(t, tenantA, domain.Scope{Kind: domain.ScopeTenant}, 1)
	other := aRule(t, tenantA, domain.Scope{Kind: domain.ScopeCollection, ID: collection}, 2)
	marked := completedItem(ctx, t, tenantA, collection, time.Now().UTC().Add(-48*time.Hour))
	held := completedItem(ctx, t, tenantA, collection, time.Now().UTC().Add(-48*time.Hour))
	now := time.Now().UTC().Truncate(time.Microsecond)

	count := func(step string, want int, fn func(context.Context) (int, error)) {
		t.Helper()
		var got int
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			got, err = fn(ctx)
			return err
		}); err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if got != want {
			t.Errorf("%s touched %d entries, want %d", step, got, want)
		}
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := ruleRepo().Insert(ctx, rule); err != nil {
			return err
		}
		return ruleRepo().Insert(ctx, other)
	}); err != nil {
		t.Fatalf("seeding the rules: %v", err)
	}

	count("marking", 1, func(ctx context.Context) (int, error) {
		return markingRepo().Mark(ctx, []shared.ID{marked}, rule.ID, domain.ActionArchive, now)
	})
	count("marking again", 0, func(ctx context.Context) (int, error) {
		return markingRepo().Mark(ctx, []shared.ID{marked}, other.ID, domain.ActionTrash, now.Add(time.Hour))
	})
	if row := retentionOf(ctx, t, marked); !row.exists || row.pending == nil || !row.pending.Equal(now) ||
		row.rule == nil || *row.rule != rule.ID.String() || row.action == nil || *row.action != string(domain.ActionArchive) {
		t.Errorf("the marking is %+v, want the first one's", row)
	}
	count("blocking a marked entry", 0, func(ctx context.Context) (int, error) {
		return markingRepo().Block(ctx, []shared.ID{marked}, rule.ID, domain.ActionArchive, "legal_hold")
	})

	count("blocking", 1, func(ctx context.Context) (int, error) {
		return markingRepo().Block(ctx, []shared.ID{held}, rule.ID, domain.ActionArchive, "legal_hold")
	})
	count("blocking unchanged", 0, func(ctx context.Context) (int, error) {
		return markingRepo().Block(ctx, []shared.ID{held}, rule.ID, domain.ActionArchive, "legal_hold")
	})
	if row := retentionOf(ctx, t, held); row.pending != nil || row.blocked == nil || *row.blocked != "legal_hold" {
		t.Errorf("the block is %+v", row)
	}
	count("marking the held entry", 1, func(ctx context.Context) (int, error) {
		return markingRepo().Mark(ctx, []shared.ID{held}, rule.ID, domain.ActionArchive, now)
	})
	if row := retentionOf(ctx, t, held); row.pending == nil || row.blocked != nil {
		t.Errorf("a marking left the block standing: %+v", row)
	}

	before := versionOf(ctx, t, marked)
	count("clearing a stage that acted", 1, func(ctx context.Context) (int, error) {
		return markingRepo().Clear(ctx, []shared.ID{marked}, true, now)
	})
	if row := retentionOf(ctx, t, marked); !row.exists || row.pending != nil || row.action != nil ||
		row.rule == nil || *row.rule != rule.ID.String() {
		t.Errorf("a stage's clearing left %+v, want the rule alone", row)
	}
	if after := versionOf(ctx, t, marked); after != before+1 {
		t.Errorf("the clearing moved the version from %d to %d", before, after)
	}
	count("clearing what does not count down", 0, func(ctx context.Context) (int, error) {
		return markingRepo().Clear(ctx, []shared.ID{marked}, false, now)
	})

	count("withdrawing the rule", 2, func(ctx context.Context) (int, error) {
		return ruleRepo().ClearMarks(ctx, rule.ID, now)
	})
	for _, id := range []shared.ID{marked, held} {
		if row := retentionOf(ctx, t, id); row.exists {
			t.Errorf("a withdrawn rule left %+v on %s", row, id)
		}
	}

	count("marking for a person to take out", 1, func(ctx context.Context) (int, error) {
		return markingRepo().Mark(ctx, []shared.ID{marked}, rule.ID, domain.ActionArchive, now)
	})
	count("a person taking it out", 1, func(ctx context.Context) (int, error) {
		return markingRepo().Clear(ctx, []shared.ID{marked}, false, now)
	})
	if row := retentionOf(ctx, t, marked); row.exists {
		t.Errorf("taking an entry out left %+v", row)
	}
}

// Gate SG-3 for the marking, the block and a withdrawn rule's clearing (the entry's own clearing
// has its negative in TestARetentionRuleAndItsMarkingsAreInvisibleFromAnotherTenant): another
// tenant's writes reach none of tenant A's side rows.
func TestTheRetentionWritersStopAtTheTenantBoundary(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	clearRules(ctx, t, tenantA)
	clearRules(ctx, t, tenantB)
	rule := aRule(t, tenantA, domain.Scope{Kind: domain.ScopeTenant}, 1)
	marked := completedItem(ctx, t, tenantA, collection, time.Now().UTC().Add(-48*time.Hour))
	bare := completedItem(ctx, t, tenantA, collection, time.Now().UTC().Add(-48*time.Hour))
	now := time.Now().UTC()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := ruleRepo().Insert(ctx, rule); err != nil {
			return err
		}
		_, err := markingRepo().Mark(ctx, []shared.ID{marked}, rule.ID, domain.ActionArchive, now)
		return err
	}); err != nil {
		t.Fatalf("seeding A: %v", err)
	}
	before := retentionOf(ctx, t, marked)

	var marks, blocks, withdrawn int
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		if marks, err = markingRepo().Mark(ctx, []shared.ID{bare, marked}, rule.ID, domain.ActionTrash, now); err != nil {
			return err
		}
		if blocks, err = markingRepo().Block(ctx, []shared.ID{bare, marked}, rule.ID, domain.ActionTrash, "legal_hold"); err != nil {
			return err
		}
		withdrawn, err = ruleRepo().ClearMarks(ctx, rule.ID, now)
		return err
	}); err != nil {
		t.Fatalf("writing from B: %v", err)
	}
	if marks+blocks+withdrawn != 0 {
		t.Errorf("tenant B marked %d, blocked %d and withdrew %d of tenant A's entries", marks, blocks, withdrawn)
	}
	if row := retentionOf(ctx, t, bare); row.exists {
		t.Errorf("tenant B left %+v on tenant A's entry", row)
	}
	if after := retentionOf(ctx, t, marked); !after.exists || after.action == nil ||
		*after.action != *before.action || !after.pending.Equal(*before.pending) {
		t.Errorf("tenant A's marking changed from %+v to %+v", before, after)
	}
}
