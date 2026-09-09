// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	quotarepo "github.com/Jersyfi/hubtask/core/application/repository/quota"
	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The quota surface of H-08, against the real boundary. Gate SG-3: the overrides, the live
// counts and the billing ledger are all bounded by the transaction they run in - one tenant's
// walls and tallies are invisible next door.

var (
	quotaTenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb01")
	quotaTenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fb02")
)

func seedQuotaTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	statements := []string{
		`INSERT INTO tenant (id, slug, display_name)
		 VALUES ('` + quotaTenantA.String() + `', 'quota-a', 'Quota A'),
		        ('` + quotaTenantB.String() + `', 'quota-b', 'Quota B')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000fb11', '` + quotaTenantA.String() + `', 'USER',
		         'quota-a@example.org', 'Quota A', 'ACTIVE')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000fb12', '` + quotaTenantA.String() + `', 'HUB',
		         'Quota hub', 'a0', '01936f2a-7c1e-7000-8000-00000000fb11')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000fb13', '` + quotaTenantA.String() + `',
		         'COLLECTION', '01936f2a-7c1e-7000-8000-00000000fb12', 'Quota collection', 'a0',
		         '01936f2a-7c1e-7000-8000-00000000fb11')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key, created_by)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000fb14', '` + quotaTenantA.String() + `',
		         '01936f2a-7c1e-7000-8000-00000000fb13', 'TASK',
		         '/01936f2a-7c1e-7000-8000-00000000fb14/', 1, 'A counted task', 'a0',
		         '01936f2a-7c1e-7000-8000-00000000fb11')
		 ON CONFLICT (id) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding the quota tenants: %v", err)
		}
	}
}

func TestTheWallsAndTheTalliesAreBoundedByTheirTenant(t *testing.T) {
	ctx := context.Background()
	seedQuotaTenants(ctx, t)
	store := postgres.NewQuotaRepository()
	tenants := postgres.NewAdminTenantRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()

	// A configures a wall.
	items := int64(123)
	inTenant(t, uow, quotaTenantA, func(ctx context.Context) error {
		record, err := tenants.Find(ctx)
		if err != nil {
			t.Fatalf("reading A: %v", err)
		}
		written, err := store.SetOverrides(ctx,
			quotarepo.Overrides{Items: &items}, record.Version, now)
		if err != nil || !written {
			t.Fatalf("writing A's wall (%v, %v)", written, err)
		}
		overrides, err := store.Overrides(ctx)
		if err != nil || overrides.Items == nil || *overrides.Items != 123 {
			t.Fatalf("A's wall did not come back: (%+v, %v)", overrides, err)
		}
		// The write reached only the quotas key: H-02's switch survives beside it.
		return nil
	})

	// Gate SG-3: B sees neither the wall nor the count.
	inTenant(t, uow, quotaTenantB, func(ctx context.Context) error {
		overrides, err := store.Overrides(ctx)
		if err != nil {
			t.Fatalf("reading B: %v", err)
		}
		if overrides.Items != nil {
			t.Error("A's wall is visible next door")
		}
		count, err := store.Items(ctx)
		if err != nil || count != 0 {
			t.Errorf("B counts A's items: (%d, %v)", count, err)
		}
		bytes, err := store.MediaBytes(ctx)
		if err != nil || bytes != 0 {
			t.Errorf("B sums A's bytes: (%d, %v)", bytes, err)
		}
		runs, err := store.AutomationRunsSince(ctx, now.Add(-time.Hour))
		if err != nil || runs != 0 {
			t.Errorf("B counts A's runs: (%d, %v)", runs, err)
		}
		return nil
	})

	// A's own counts answer.
	inTenant(t, uow, quotaTenantA, func(ctx context.Context) error {
		count, err := store.Items(ctx)
		if err != nil || count != 1 {
			t.Errorf("A counts %d items (%v), want 1", count, err)
		}
		return nil
	})

	// The ledger: A meters, B's ledger stays empty, and two adds accumulate.
	inTenant(t, uow, quotaTenantA, func(ctx context.Context) error {
		if err := store.Add(ctx, "automation_runs_per_hour", now, 2); err != nil {
			t.Fatalf("metering: %v", err)
		}
		return store.Add(ctx, "automation_runs_per_hour", now, 3)
	})
	var value int64
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT value FROM usage_record
		WHERE tenant_id = $1 AND metric = 'automation_runs_per_hour' AND period = $2`,
		quotaTenantA.String(), now.Format("2006-01-02"),
	).Scan(&value); err != nil || value != 5 {
		t.Errorf("the ledger holds %d (%v), want 5", value, err)
	}
	var foreign int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM usage_record WHERE tenant_id = $1`, quotaTenantB.String(),
	).Scan(&foreign); err != nil || foreign != 0 {
		t.Errorf("B's ledger holds %d rows (%v)", foreign, err)
	}

	// And the settings write preserved its neighbours: the H-02 switch still reads.
	var settings string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT settings::text FROM tenant WHERE id = $1`, quotaTenantA.String(),
	).Scan(&settings); err != nil {
		t.Fatalf("reading the settings: %v", err)
	}
	if settings == "" || settings == "null" {
		t.Errorf("the settings document is %q", settings)
	}
}

// The AI budget's own boundary (J-15). It is the one quota measured from `usage_record`, so it is
// the one whose count could cross a workspace if row level security were not carrying the sum -
// and a budget that counted the installation's spending would refuse the wrong workspace.
func TestOneWorkspacesAiSpendingIsNotAnothers(t *testing.T) {
	ctx := context.Background()
	seedQuotaTenants(ctx, t)
	store := postgres.NewQuotaRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// A spends.
	inTenant(t, uow, quotaTenantA, func(ctx context.Context) error {
		if err := store.Add(ctx, quotaservice.AiTokensPerDay, now, 5_000); err != nil {
			t.Fatalf("metering A's spend: %v", err)
		}
		return nil
	})

	var spentByA, spentByB int64
	inTenant(t, uow, quotaTenantA, func(ctx context.Context) error {
		var err error
		spentByA, err = store.MeteredSince(ctx, quotaservice.AiTokensPerDay, since)
		return err
	})
	inTenant(t, uow, quotaTenantB, func(ctx context.Context) error {
		var err error
		spentByB, err = store.MeteredSince(ctx, quotaservice.AiTokensPerDay, since)
		return err
	})

	if spentByA < 5_000 {
		t.Errorf("A spent 5000 tokens and its own ledger says %d", spentByA)
	}
	if spentByB != 0 {
		t.Errorf("B's budget counts %d tokens it never spent", spentByB)
	}

	// And the guard reads the same boundary: B has room at a ceiling A has already passed.
	guard := quotaservice.Guard{
		Store: store, Usage: store, Meter: store, Tenancy: env.TenancySingle,
	}
	ceiling := int64(1_000)
	for _, tenant := range []shared.ID{quotaTenantA, quotaTenantB} {
		inTenant(t, uow, tenant, func(ctx context.Context) error {
			record, err := postgres.NewAdminTenantRepository().Find(ctx)
			if err != nil {
				return err
			}
			_, err = store.SetOverrides(ctx,
				quotarepo.Overrides{AiTokensPerDay: &ceiling}, record.Version, now)
			return err
		})
	}

	inTenant(t, uow, quotaTenantA, func(ctx context.Context) error {
		room, err := guard.AiTokens(ctx, quotaTenantA.String(), now)
		if err != nil {
			return err
		}
		if room {
			t.Error("A is over its ceiling and the guard says it has room")
		}
		return nil
	})
	inTenant(t, uow, quotaTenantB, func(ctx context.Context) error {
		room, err := guard.AiTokens(ctx, quotaTenantB.String(), now)
		if err != nil {
			return err
		}
		if !room {
			t.Error("B was refused for what A spent")
		}
		return nil
	})
}
