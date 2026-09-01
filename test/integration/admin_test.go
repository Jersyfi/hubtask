// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The control plane of H-06, against the real boundary. Gate SG-3: the admin surface joins the
// cross-tenant suite - every statement it runs is bounded by the transaction it was opened for,
// and the hard delete leaves nothing countable while the tenant beside it keeps everything.

var (
	doomedTenant    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f001")
	bystanderTenant = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f002")
	doomedAccount   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f011")
	bystanderRule   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f021")
)

// purgeStoreFake records the keys the byte deletion asked the object store to remove.
type purgeStoreFake struct{ deleted []string }

func (s *purgeStoreFake) Put(context.Context, storage.Upload) error { return nil }

func (s *purgeStoreFake) Get(context.Context, string) (storage.Object, error) {
	return storage.Object{}, shared.ErrNotFound.WithDetail("storage.object_missing")
}

func (s *purgeStoreFake) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

// seedLifecycleTenants writes a doomed workspace with a footprint in every §5 store, and a
// bystander with one row in each of the stores the purge touches explicitly.
func seedLifecycleTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)

	statements := []string{
		`INSERT INTO tenant (id, slug, display_name)
		 VALUES ('` + doomedTenant.String() + `', 'doomed', 'Doomed'),
		        ('` + bystanderTenant.String() + `', 'bystander', 'Bystander')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		 VALUES ('` + doomedAccount.String() + `', '` + doomedTenant.String() + `', 'USER',
		         'doomed@example.org', 'Doomed', 'ACTIVE'),
		        ('01936f2a-7c1e-7000-8000-00000000f012', '` + bystanderTenant.String() + `', 'USER',
		         'bystander@example.org', 'Bystander', 'ACTIVE')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000f013', '` + doomedTenant.String() + `',
		         '` + doomedAccount.String() + `', now(), now() + interval '30 days')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000f014', '` + doomedTenant.String() + `', 'HUB',
		         'Doomed hub', 'a0', '` + doomedAccount.String() + `')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000f015', '` + doomedTenant.String() + `',
		         'COLLECTION', '01936f2a-7c1e-7000-8000-00000000f014', 'Doomed collection', 'a0',
		         '` + doomedAccount.String() + `')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key, created_by)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000f016', '` + doomedTenant.String() + `',
		         '01936f2a-7c1e-7000-8000-00000000f015', 'TASK',
		         '/01936f2a-7c1e-7000-8000-00000000f016/', 1, 'A doomed task', 'a0',
		         '` + doomedAccount.String() + `')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, usage, status)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000f017', '` + doomedTenant.String() + `',
		         'media/doomed/a', 'text/plain', 11, 'ATTACHMENT', 'READY'),
		        ('01936f2a-7c1e-7000-8000-00000000f018', '` + doomedTenant.String() + `',
		         'media/doomed/b', 'text/plain', 22, 'ATTACHMENT', 'READY')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO outbox_event (id, tenant_id, event_type, payload, actor_type)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000f019', '` + doomedTenant.String() + `',
		         'de.hubtask.test.v1', '{}'::jsonb, 'SYSTEM')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO audit_log (id, tenant_id, seq, occurred_at, action, outcome, actor_type, hash)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000f01a', '` + doomedTenant.String() + `', 9001,
		         '2026-08-15T12:00:00Z', 'seeded.doomed', 'SUCCESS', 'SYSTEM', '\x00'),
		        ('01936f2a-7c1e-7000-8000-00000000f01b', '` + bystanderTenant.String() + `', 9001,
		         '2026-08-15T12:00:00Z', 'seeded.bystander', 'SUCCESS', 'SYSTEM', '\x00')
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO job (id, tenant_id, kind, payload, run_at)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000f01c', '` + doomedTenant.String() + `',
		         'outbox.dispatch', '{}'::jsonb, now() + interval '1 hour')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO idempotency_key (tenant_id, key, endpoint, request_hash)
		 VALUES ('` + doomedTenant.String() + `', 'k1', 'POST /x', '\x01')
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO automation_rule
		   (id, tenant_id, scope_type, name, run_as, trigger, actions, created_by, enabled)
		 VALUES ('01936f2a-7c1e-7000-8000-00000000f01d', '` + doomedTenant.String() + `',
		         'TENANT', 'doomed rule', '` + doomedAccount.String() + `',
		         '{"kind":"EVENT"}'::jsonb, '[]'::jsonb, '` + doomedAccount.String() + `', true),
		        ('` + bystanderRule.String() + `', '` + bystanderTenant.String() + `',
		         'TENANT', 'bystander rule', '01936f2a-7c1e-7000-8000-00000000f012',
		         '{"kind":"EVENT"}'::jsonb, '[]'::jsonb,
		         '01936f2a-7c1e-7000-8000-00000000f012', true)
		 ON CONFLICT (id) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding the lifecycle tenants: %v\n%s", err, statement)
		}
	}
}

// Gate SG-3 for the admin surface: every statement is bounded by its transaction's tenant, and
// the enumerator is the one deliberate exception, reachable only under the installation scope.
func TestTheAdminSurfaceIsBoundedLikeEverythingElse(t *testing.T) {
	ctx := context.Background()
	seedLifecycleTenants(ctx, t)
	tenants := postgres.NewAdminTenantRepository()
	automations := postgres.NewAutomationSwitch()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	// The enumerator answers under the installation scope, and both workspaces are in it.
	seen := map[string]bool{}
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		records, err := tenants.List(ctx)
		for _, record := range records {
			seen[record.Slug] = true
		}
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if !seen["doomed"] || !seen["bystander"] {
		t.Fatalf("the enumerator answered %v", seen)
	}

	// Find and the lifecycle writes reach only the transaction's own tenant.
	inTenant(t, uow, bystanderTenant, func(ctx context.Context) error {
		record, err := tenants.Find(ctx)
		if err != nil || record.ID != bystanderTenant {
			t.Fatalf("find answered (%+v, %v)", record, err)
		}
		return nil
	})

	// Disabling the automations in one workspace leaves the neighbour's rule running.
	inTenant(t, uow, doomedTenant, func(ctx context.Context) error {
		if _, err := automations.DisableAll(ctx, time.Now().UTC()); err != nil {
			t.Fatalf("disabling: %v", err)
		}
		return nil
	})
	var enabled bool
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT enabled FROM automation_rule WHERE id = $1`, bystanderRule.String(),
	).Scan(&enabled); err != nil || !enabled {
		t.Errorf("the bystander's rule was touched (enabled=%v, %v)", enabled, err)
	}

	// The footprint counts only the transaction's tenant.
	purge := postgres.NewTenantPurge()
	inTenant(t, uow, bystanderTenant, func(ctx context.Context) error {
		footprint, err := purge.Footprint(ctx)
		if err != nil {
			t.Fatalf("counting: %v", err)
		}
		if footprint.MediaObjects != 0 || footprint.Items != 0 {
			t.Errorf("the bystander counts the doomed tenant's footprint: %+v", footprint)
		}
		keys, err := purge.StorageKeys(ctx, "", 10)
		if err != nil || len(keys) != 0 {
			t.Errorf("the bystander pages the doomed tenant's keys: %v (%v)", keys, err)
		}
		return nil
	})
}

// The §5 acceptance: after the grace, everything falls - rows, media bytes, outbox, queue,
// search (the items' own columns), the trail - counted in each store, with the evidence entry
// where the task decided it lives. The workspace next door keeps every row it had.
func TestTheHardDeleteLeavesNothingCountableAndTheNeighbourEverything(t *testing.T) {
	ctx := context.Background()
	seedLifecycleTenants(ctx, t)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	tenants := postgres.NewAdminTenantRepository()
	store := &purgeStoreFake{}

	// The deletion request's write, with a deadline already in the past so the job is due.
	inTenant(t, uow, doomedTenant, func(ctx context.Context) error {
		moved, err := tenants.RequestDeletion(ctx, time.Now().Add(-time.Hour).UTC(), time.Now().UTC())
		if err != nil || !moved {
			t.Fatalf("requesting the deletion (%v, %v)", moved, err)
		}
		return nil
	})

	deletion := adminservice.HardDeleteTenant{
		Tenants: tenants, Purge: postgres.NewTenantPurge(),
		Journal: postgres.NewInstanceJournal(), Store: store,
		UnitOfWork: uow, Clock: systemClock{}, IDs: &entropyIDs{},
	}
	outcome, err := deletion.Execute(ctx, doomedTenant,
		shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f0ff"))
	if err != nil {
		t.Fatalf("the hard delete: %v", err)
	}
	if !outcome.Deleted {
		t.Fatal("the guard held a due deletion back")
	}
	if outcome.Footprint.Items != 1 || outcome.Footprint.MediaObjects != 2 ||
		outcome.Footprint.MediaBytes != 33 {
		t.Errorf("footprint %+v", outcome.Footprint)
	}
	if len(store.deleted) != 2 {
		t.Errorf("bytes deleted: %v", store.deleted)
	}

	// Counted in each store: nothing of the doomed workspace remains.
	admin := adminPool(ctx, t)
	for table, where := range map[string]string{
		"tenant":          "id",
		"account":         "tenant_id",
		"session":         "tenant_id",
		"container":       "tenant_id",
		"work_item":       "tenant_id",
		"media_object":    "tenant_id",
		"outbox_event":    "tenant_id",
		"audit_log":       "tenant_id",
		"job":             "tenant_id",
		"idempotency_key": "tenant_id",
		"automation_rule": "tenant_id",
	} {
		var count int
		if err := admin.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE `+where+` = $1`, doomedTenant.String(),
		).Scan(&count); err != nil {
			t.Fatalf("counting %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("%s still holds %d rows of the deleted workspace", table, count)
		}
	}

	// The neighbour kept everything.
	for table, want := range map[string]int{
		"tenant": 1, "account": 1, "audit_log": 1, "automation_rule": 1,
	} {
		column := "tenant_id"
		if table == "tenant" {
			column = "id"
		}
		var count int
		if err := admin.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE `+column+` = $1`, bystanderTenant.String(),
		).Scan(&count); err != nil {
			t.Fatalf("counting the neighbour's %s: %v", table, err)
		}
		if count != want {
			t.Errorf("the neighbour's %s holds %d rows, want %d", table, count, want)
		}
	}

	// The evidence entry, where the task decided it lives: the instance journal, outliving the
	// tenant it names.
	var evidence int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM instance_event
		WHERE tenant_id = $1 AND action = 'tenant.hard_deleted'
		  AND (details->>'items')::bigint = 1
		  AND (details->>'media_bytes')::bigint = 33`,
		doomedTenant.String(),
	).Scan(&evidence); err != nil || evidence != 1 {
		t.Errorf("evidence entries: %d (%v)", evidence, err)
	}

	// A second pass is a quiet nothing: the tenant is gone, and nothing errors.
	again, err := deletion.Execute(ctx, doomedTenant,
		shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f0fe"))
	if err != nil || again.Deleted {
		t.Errorf("the second pass answered (%+v, %v)", again, err)
	}
}

// systemClock and entropyIDs are the two smallest fakes that satisfy the ports against a real
// database: the clock is real time (the guard compares against a stored deadline), and the
// identifiers only have to be unique.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type entropyIDs struct{ count int }

func (i *entropyIDs) NewID() shared.ID {
	i.count++
	suffix := []byte{'0', '0'}
	suffix[0] = "0123456789ab"[i.count/10%12]
	suffix[1] = "0123456789ab"[i.count%10]
	return shared.MustParseID("01936f2a-eeee-7000-8000-0000000000" + string(suffix))
}

var _ clock.Clock = systemClock{}

// Sanity: suspension rides with the credential reads (H-06 §3) - the row the middleware judges
// carries the standing this surface writes.
func TestSuspensionRidesWithTheCredentialRead(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	seedLifecycleTenants(ctx, t)
	// The suite's earlier tests move this workspace through its lifecycle; this one is about the
	// credential read, so it starts from a known standing.
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE tenant SET status = 'ACTIVE', purge_after = NULL WHERE id = $1`,
		doomedTenant.String()); err != nil {
		t.Fatalf("resetting the standing: %v", err)
	}
	tenants := postgres.NewAdminTenantRepository()
	sessions := postgres.NewSessionRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	inTenant(t, uow, doomedTenant, func(ctx context.Context) error {
		if _, err := tenants.SetStatus(ctx,
			identity.TenantActive, identity.TenantSuspended, time.Now().UTC()); err != nil {
			t.Fatalf("suspending: %v", err)
		}
		credential, err := sessions.FindForAuth(ctx,
			shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f013"))
		if err != nil {
			t.Fatalf("reading the session: %v", err)
		}
		if credential.TenantStatus != identity.TenantSuspended || credential.TenantSlug != "doomed" {
			t.Errorf("the standing did not ride: %+v", credential)
		}
		// Back, so the ordering of this suite does not decide the other tests.
		if _, err := tenants.SetStatus(ctx,
			identity.TenantSuspended, identity.TenantActive, time.Now().UTC()); err != nil {
			t.Fatalf("resuming: %v", err)
		}
		return nil
	})
}
