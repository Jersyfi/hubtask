// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// An erasure removes the arrangements in the person's name alone - their digests, the dashboards
// only they see, the delegations by and to them - and nothing of a colleague's or of another
// workspace's (PG-2, data-protection.md §4.1).
func TestAnErasureTakesThePersonsArrangementsAndNoOneElses(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	admin := adminPool(ctx, t)
	subject, colleague, other := seedAccount(ctx, t, tenantB), seedAccount(ctx, t, tenantB), seedAccount(ctx, t, tenantB)
	private, shared, theirs := freshID(t), freshID(t), freshID(t)

	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO dashboard (id, tenant_id, owner_id, name, sharing, created_at, updated_at)
		  VALUES ($1, $4, $5, 'mine', 'PRIVATE', now(), now()),
		         ($2, $4, $5, 'the team''s', 'SCOPE', now(), now()),
		         ($3, $4, $6, 'theirs', 'PRIVATE', now(), now())`,
			[]any{private.String(), shared.String(), theirs.String(), tenantB.String(), subject.String(), colleague.String()}},
		{`INSERT INTO dashboard_digest (id, tenant_id, dashboard_id, account_id, cadence, hour, time_zone,
		    next_at, created_at, updated_at)
		  VALUES ($1, $3, $4, $5, 'DAILY', 8, 'Europe/Berlin', now(), now(), now()),
		         ($2, $3, $4, $6, 'DAILY', 8, 'Europe/Berlin', now(), now(), now())`,
			[]any{freshID(t).String(), freshID(t).String(), tenantB.String(), shared.String(),
				subject.String(), colleague.String()}},
		{`INSERT INTO approval_delegation (tenant_id, account_id, delegate_id, starts_at, ends_at)
		  VALUES ($1, $2, $3, now(), now() + interval '7 days'),
		         ($1, $3, $2, now(), now() + interval '7 days'),
		         ($1, $4, $3, now(), now() + interval '7 days')`,
			[]any{tenantB.String(), subject.String(), colleague.String(), other.String()}},
		// Another workspace naming the same identifier: never reached from tenant B.
		{`INSERT INTO approval_delegation (tenant_id, account_id, delegate_id, starts_at, ends_at)
		  VALUES ($1, $2, $3, now(), now() + interval '7 days')`,
			[]any{tenantA.String(), subject.String(), freshID(t).String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, statement.sql)
		}
	}

	var removed int
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		removed, err = postgres.NewPrivacyRepository(pageCursors()).DiscardArrangements(ctx, subject)
		return err
	}); err != nil {
		t.Fatalf("discarding: %v", err)
	}
	// One private dashboard, one digest, two delegations.
	if removed != 4 {
		t.Errorf("%d arrangements removed, want 4", removed)
	}

	count := func(sql string, args ...any) int {
		var n int
		if err := admin.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("counting: %v", err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM dashboard WHERE id = ANY($1::uuid[])`,
		[]string{shared.String(), theirs.String()}); n != 2 {
		t.Errorf("the shared dashboard or a colleague's went too: %d of 2 left", n)
	}
	if n := count(`SELECT count(*) FROM dashboard WHERE id = $1`, private.String()); n != 0 {
		t.Error("the dashboard only the person saw is still there")
	}
	if n := count(`SELECT count(*) FROM dashboard_digest WHERE account_id = $1`, colleague.String()); n != 1 {
		t.Error("a colleague's digest went too")
	}
	if n := count(`SELECT count(*) FROM approval_delegation WHERE tenant_id = $1 AND account_id = $2`,
		tenantB.String(), other.String()); n != 1 {
		t.Error("a delegation between two other people went too")
	}
	if n := count(`SELECT count(*) FROM approval_delegation WHERE tenant_id = $1 AND account_id = $2`,
		tenantA.String(), subject.String()); n != 1 {
		t.Error("an erasure in tenant B reached tenant A's delegation")
	}
}
