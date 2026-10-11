// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/test/dbtest"
)

// UC-BAK-08 check 3, the holds half, against a real database: the operator places the holds of a
// rewound period again through the registry; the workspace reads them back as the application
// role, in force, with their own identifier, placer and moment; a second run changes nothing; the
// other workspace is untouched (backup-restore.md §8.5 step 5).
func TestTheOperatorPlacesTheHoldsOfARewoundPeriodAgain(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	recoveredTo := created.Add(-24 * time.Hour)
	placedAt := created.Add(2 * time.Hour)
	inForce, released := freshID(t), freshID(t)

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created.Add(72 * time.Hour))
	holds := lifecycle.Holds{
		Holds: holdRepo(), Audit: postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed)), UnitOfWork: unitOfWork,
		Clock: fixed, Text: text.Composing{},
	}
	registry, err := usecase.NewRegistry(nil, lifecycle.ReplaceLegalHolds{
		Holds: holds, Workspaces: postgres.NewAdminTenantRepository(),
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantB, AccountID: authorB, AccountName: "Operator",
		Scopes: []string{"admin:tenants"},
	}
	input := usecase.Input{
		"tenant_id":      tenantA.String(),
		"recovery_point": recoveredTo.Format(time.RFC3339),
		"holds": []any{
			map[string]any{
				"id": inForce.String(), "scope": map[string]any{"kind": "CONTAINER", "id": collection.String()},
				"reason": "Dispute with the supplier", "placed_by": authorA.String(),
				"placed_at": placedAt.Format(time.RFC3339Nano),
			},
			map[string]any{
				"id": released.String(), "scope": map[string]any{"kind": "TENANT"},
				"reason": "Inspection", "placed_by": authorA.String(),
				"placed_at":   placedAt.Format(time.RFC3339Nano),
				"released_at": placedAt.Add(time.Hour).Format(time.RFC3339Nano),
			},
		},
	}

	if _, err := registry.Invoke(ctx, lifecycle.ReplaceLegalHoldsName, operator, input); err != nil {
		t.Fatalf("placing again: %v", err)
	}
	var active domain.Holds
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		active, err = lifecycleRepo().Active(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the holds: %v", err)
	}
	for _, id := range []shared.ID{inForce, released} {
		if !containsHold(active, id) {
			t.Errorf("hold %s is not in force", id)
		}
	}
	var found domain.LegalHold
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = holdRepo().Find(ctx, inForce)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if found.PlacedBy != authorA || !found.PlacedAt.Equal(placedAt) || found.ScopeID != collection {
		t.Errorf("placed again as %+v", found)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = $2 AND target_id = $3`,
		tenantA.String(), string(lifecycle.HoldReplacedAction), inForce.String()); n != 1 {
		t.Errorf("%d replacement entries in the workspace's trail", n)
	}

	// A second run changes nothing, and the other workspace never had any of it.
	if _, err := registry.Invoke(ctx, lifecycle.ReplaceLegalHoldsName, operator, input); err != nil {
		t.Fatalf("the second run: %v", err)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM legal_hold WHERE id = ANY($1::uuid[])`,
		[]string{inForce.String(), released.String()}); n != 2 {
		t.Errorf("%d hold rows after two runs", n)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM legal_hold WHERE tenant_id = $1 AND id = ANY($2::uuid[])`,
		tenantB.String(), []string{inForce.String(), released.String()}); n != 0 {
		t.Errorf("%d of the holds landed in the other workspace", n)
	}
}

// UC-BAK-08 check 3, the temporary cluster's half: the holds are read from a second database - the
// stand-in for the cluster recovered to the end of the WAL archive - for one workspace only, as the
// application role and as a superuser that row level security does not stop, and placed in the
// recovered workspace through the operation.
func TestTheHoldsOfARewoundPeriodAreReadFromTheTemporaryCluster(t *testing.T) {
	ctx := context.Background()
	const name = "hubtask_recovered_holds"
	adminDSN, appDSN := recoveredCluster(ctx, t, name)

	collection := collectionFor(ctx, t, tenantA, authorA)
	placedAt := created.Add(3 * time.Hour)
	mine, mineReleased, theirs := freshID(t), freshID(t), freshID(t)
	cluster, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Close()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'recovered-a', 'A'), ($2, 'recovered-b', 'B')`,
			[]any{tenantA.String(), tenantB.String()}},
		{`INSERT INTO legal_hold (id, tenant_id, scope_kind, scope_id, reason, placed_by, placed_at)
		  VALUES ($1, $2, 'CONTAINER', $3, 'Dispute with the supplier', $4, $5)`,
			[]any{mine.String(), tenantA.String(), collection.String(), authorA.String(), placedAt}},
		{`INSERT INTO legal_hold (id, tenant_id, scope_kind, reason, placed_by, placed_at, released_by, released_at)
		  VALUES ($1, $2, 'TENANT', 'Inspection', $3, $4, $3, $5)`,
			[]any{mineReleased.String(), tenantA.String(), authorA.String(), placedAt, placedAt.Add(time.Hour)}},
		{`INSERT INTO legal_hold (id, tenant_id, scope_kind, reason, placed_by, placed_at)
		  VALUES ($1, $2, 'TENANT', 'Somebody else''s case', $3, $4)`,
			[]any{theirs.String(), tenantB.String(), authorB.String(), placedAt}},
	} {
		if _, err := cluster.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the temporary cluster: %v", err)
		}
	}

	var holds []domain.LegalHold
	for role, dsn := range map[string]string{"the application role": appDSN, "a superuser": adminDSN} {
		holds, err = postgres.ReadRecoveredHolds(ctx, dsn, tenantA, 30*time.Second)
		if err != nil {
			t.Fatalf("reading as %s: %v", role, err)
		}
		if len(holds) != 2 || holds[0].ID == theirs || holds[1].ID == theirs {
			t.Fatalf("as %s the workspace's holds are %+v", role, holds)
		}
	}

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created.Add(96 * time.Hour))
	registry, err := usecase.NewRegistry(nil, lifecycle.ReplaceLegalHolds{
		Holds: lifecycle.Holds{
			Holds: holdRepo(), Audit: postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed)),
			UnitOfWork: unitOfWork, Clock: fixed, Text: text.Composing{},
		},
		Workspaces: postgres.NewAdminTenantRepository(),
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	records := make([]any, 0, len(holds))
	for _, hold := range holds {
		scope := map[string]any{"kind": string(hold.Scope)}
		if !hold.ScopeID.IsZero() {
			scope["id"] = hold.ScopeID.String()
		}
		record := map[string]any{
			"id": hold.ID.String(), "scope": scope, "reason": hold.Reason,
			"placed_by": hold.PlacedBy.String(), "placed_at": hold.PlacedAt.Format(time.RFC3339Nano),
		}
		if !hold.ReleasedAt.IsZero() {
			record["released_at"] = hold.ReleasedAt.Format(time.RFC3339Nano)
		}
		records = append(records, record)
	}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantB, AccountID: authorB, AccountName: "Operator",
		Scopes: []string{"admin:tenants"},
	}
	if _, err := registry.Invoke(ctx, lifecycle.ReplaceLegalHoldsName, operator, usecase.Input{
		"tenant_id": tenantA.String(), "recovery_point": created.Format(time.RFC3339), "holds": records,
	}); err != nil {
		t.Fatalf("placing again: %v", err)
	}

	var active domain.Holds
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		active, err = lifecycleRepo().Active(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !containsHold(active, mine) || !containsHold(active, mineReleased) {
		t.Error("a hold of the temporary cluster is not in force in the recovered workspace")
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM legal_hold WHERE id = $1`, theirs.String()); n != 0 {
		t.Error("another workspace's hold was placed")
	}
}

// recoveredCluster is a second database on the test server, migrated like the suite's own: the
// temporary cluster's stand-in. It answers its DSNs as the superuser and as the application role,
// and is dropped with the test.
func recoveredCluster(ctx context.Context, t *testing.T, name string) (adminDSN, appDSN string) {
	t.Helper()
	db := dbtest.Start(t)
	admin := adminPool(ctx, t)
	// The name is a constant of the caller's, not anything a request supplies; CREATE DATABASE
	// takes no parameters.
	if _, err := admin.Exec(ctx, `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`); err != nil {
		t.Fatalf("dropping %s: %v", name, err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		pool, err := pgxpool.New(cleanup, db.AdminDSN)
		if err != nil {
			return
		}
		defer pool.Close()
		_, _ = pool.Exec(cleanup, `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`)
	})

	adminDSN = regexp.MustCompile(`/[^/?]+(\?|$)`).ReplaceAllString(db.AdminDSN, "/"+name+"$1")
	if err := dbtest.Migrate(ctx, adminDSN); err != nil {
		t.Fatalf("migrating %s: %v", name, err)
	}
	return adminDSN, rewrite(t, adminDSN, "hubtask_app", dbtest.AppPassword, name)
}
