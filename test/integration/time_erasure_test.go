// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A person's erasure keeps their time and removes its words (accepted.md B), against the real
// statements: another workspace's erasure touches nothing of this one's; this one's clears the note
// and the rejection's reason, removes the running timer and keeps the minutes.
func TestAnErasureKeepsTheTimeAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	subject := freshID(t)
	record, week, timer := freshID(t), freshID(t), freshID(t)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO time_record (id, tenant_id, account_id, day, minutes, note, source, created_at, updated_at)
		  VALUES ($1, $2, $3, DATE '2026-10-26', 90, 'Called Dr. Weber about my back', 'MANUAL', now(), now())`,
			[]any{record.String(), tenantA.String(), subject.String()}},
		{`INSERT INTO timesheet (id, tenant_id, account_id, week_start, state, minutes, reason, updated_at)
		  VALUES ($1, $2, $3, DATE '2026-10-26', 'REJECTED', 90, 'Too much sick leave', now())`,
			[]any{week.String(), tenantA.String(), subject.String()}},
		{`INSERT INTO time_timer (id, tenant_id, account_id, started_at) VALUES ($1, $2, $3, now())`,
			[]any{timer.String(), tenantA.String(), subject.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	privacy := postgres.NewPrivacyRepository(pageCursors())

	var changed int
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		changed, err = privacy.PseudonymiseTime(ctx, subject)
		return err
	}); err != nil || changed != 0 {
		t.Fatalf("another workspace's erasure changed %d (%v)", changed, err)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM time_record WHERE id = $1 AND note <> ''`, record.String()); n != 1 {
		t.Fatal("another workspace's erasure reached the note")
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		changed, err = privacy.PseudonymiseTime(ctx, subject)
		return err
	}); err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if changed != 3 {
		t.Errorf("changed %d, want the note, the reason and the timer", changed)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM time_record WHERE id = $1 AND note = '' AND minutes = 90`, record.String()); n != 1 {
		t.Error("the record was not kept with its minutes and without its note")
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM timesheet WHERE id = $1 AND reason = '' AND state = 'REJECTED'`, week.String()); n != 1 {
		t.Error("the week was not kept without its reason")
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM time_timer WHERE id = $1`, timer.String()); n != 0 {
		t.Error("the running timer outlived the person")
	}
}
