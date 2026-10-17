// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// seedWorkspace writes one workspace as the superuser, with a short name unique to the run.
func seedWorkspace(ctx context.Context, t *testing.T, stem string) shared.ID {
	t.Helper()
	admin := adminPool(ctx, t)
	id := freshID(t)
	slug := fmt.Sprintf("%s-%09d", stem, time.Now().UnixNano()%1_000_000_000)
	if _, err := admin.Exec(ctx, `INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, $3)`,
		id.String(), slug, stem); err != nil {
		t.Fatalf("seeding %s: %v", stem, err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.WithoutCancel(ctx),
			`UPDATE tenant SET external_reference = NULL, plan_id = NULL, deleted_at = now() WHERE id = $1`, id.String())
	})
	return id
}

// A reference names one workspace across the installation, although each transaction sees only its
// own row; the refusal names neither the value nor the workspace that holds it (rule 10, P-01).
func TestAReferenceNamesOneWorkspace(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	tenants := postgres.NewAdminTenantRepository()
	reference := "cus_" + planName("ref")
	first, second := seedWorkspace(ctx, t, "first"), seedWorkspace(ctx, t, "second")

	set := func(id shared.ID) error {
		return uow.Within(ctx, persistence.Scope{TenantID: id}, func(ctx context.Context) error {
			record, err := tenants.Find(ctx)
			if err != nil {
				return err
			}
			written, err := tenants.SetReference(ctx, reference, record.Version, time.Now())
			if err == nil && !written {
				t.Error("the guarded write on the version read wrote nothing")
			}
			return err
		})
	}
	if err := set(first); err != nil {
		t.Fatalf("setting the first: %v", err)
	}
	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: first}, func(ctx context.Context) error {
		record, err := tenants.Find(ctx)
		if err == nil && record.ExternalReference != reference {
			t.Errorf("the control plane's own read answers the reference %q", record.ExternalReference)
		}
		return err
	}); err != nil {
		t.Fatalf("reading the first: %v", err)
	}
	err := set(second)
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "admin.reference_taken" {
		t.Fatalf("a second workspace with the reference answered %v", err)
	}
	if len(refusal.Params) != 0 || strings.Contains(err.Error(), reference) || errors.Unwrap(err) != nil {
		t.Errorf("the refusal carries the value or the database's detail: %v", err)
	}
}

// Provisioning's own path: the workspace written, then a reference another workspace holds - the
// refusal is the reference's, and the whole workspace rolls back with it.
func TestProvisioningWithATakenReferenceWritesNothing(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	tenants := postgres.NewAdminTenantRepository()
	reference := "cus_" + planName("prov")
	holder := seedWorkspace(ctx, t, "holder")
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE tenant SET external_reference = $1 WHERE id = $2`,
		reference, holder.String()); err != nil {
		t.Fatal(err)
	}

	fresh := freshID(t)
	err := uow.Within(ctx, persistence.Scope{TenantID: fresh}, func(ctx context.Context) error {
		if err := tenants.Insert(ctx, adminrepo.TenantRecord{
			ID: fresh, Slug: fmt.Sprintf("fresh-%09d", time.Now().UnixNano()%1_000_000_000), DisplayName: "Fresh",
			Status: identity.TenantActive, DefaultLocale: "en", DefaultTimeZone: "UTC", CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		_, err := tenants.SetReference(ctx, reference, 1, time.Now())
		return err
	})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "admin.reference_taken" {
		t.Fatalf("provisioning with a taken reference answered %v", err)
	}
	var written int
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT count(*) FROM tenant WHERE id = $1`, fresh.String()).Scan(&written); err != nil {
		t.Fatal(err)
	}
	if written != 0 {
		t.Error("the workspace stayed although its reference was refused")
	}
}

// A write in one workspace's scope reaches no other workspace's reference, and the query answers
// inside a workspace's own scope as the reference's write uses it (a cross-tenant negative test).
func TestAReferenceIsWrittenOnlyInItsOwnWorkspace(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	tenants := postgres.NewAdminTenantRepository()
	finder := postgres.NewTenantQueryRepository(pageCursors())
	mine, theirs := seedWorkspace(ctx, t, "mine"), seedWorkspace(ctx, t, "theirs")
	reference := "cus_" + planName("own")

	if err := uow.Within(ctx, persistence.Scope{TenantID: mine}, func(ctx context.Context) error {
		if _, err := tenants.SetReference(ctx, reference, 1, time.Now()); err != nil {
			return err
		}
		found, _, err := finder.Query(ctx, adminrepo.TenantQuery{ID: mine, Size: 1})
		if err == nil && (len(found) != 1 || found[0].ExternalReference != reference) {
			t.Errorf("the query in the workspace's own scope answered %+v", found)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var other *string
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT external_reference FROM tenant WHERE id = $1`, theirs.String()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if other != nil {
		t.Errorf("another workspace's reference became %q", *other)
	}
}

// A workspace leaving is still found, with its deletion dates: a platform reconciling a cancelled
// customer sees where the deletion stands.
func TestAWorkspaceLeavingIsFoundWithItsDates(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	finder := postgres.NewTenantQueryRepository(pageCursors())
	leaving := seedWorkspace(ctx, t, "leaving")
	reference := "cus_" + planName("leave")
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE tenant SET status = 'PENDING_DELETION', purge_after = now() + interval '30 days',
		purge_started_at = now(), external_reference = $1 WHERE id = $2`, reference, leaving.String()); err != nil {
		t.Fatal(err)
	}
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		found, _, err := finder.Query(ctx, adminrepo.TenantQuery{Reference: reference, Size: 1})
		if err == nil && (len(found) != 1 || found[0].PurgeAfter.IsZero() || found[0].PurgeStartedAt.IsZero()) {
			t.Errorf("a workspace leaving answered %+v", found)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// The reference is the provider's mapping, not the workspace's data: it is not in the archive.
func TestTheReferenceIsNotInTheWorkspacesArchive(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	workspace := seedWorkspace(ctx, t, "archived")
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE tenant SET external_reference = $1 WHERE id = $2`,
		"cus_"+planName("arch"), workspace.String()); err != nil {
		t.Fatal(err)
	}
	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: workspace}, func(ctx context.Context) error {
		tx, err := postgres.FromContext(ctx)
		if err != nil {
			return err
		}
		rows, err := sqlc.New(tx).ExportTenants(ctx, sqlc.ExportTenantsParams{
			AfterAt: pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true}, AfterID: pgtype.UUID{Valid: true}, Batch: 10,
		})
		if err != nil {
			return err
		}
		if len(rows) != 1 || strings.Contains(string(rows[0].Payload), "external_reference") {
			t.Errorf("the archive's tenant row: %d rows, %s", len(rows), rowsPayload(rows))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func rowsPayload(rows []sqlc.ExportTenantsRow) string {
	if len(rows) == 0 {
		return ""
	}
	return string(rows[0].Payload)
}

// UC-INS-13 check 10: a platform finds its workspaces by reference, short name, plan and status,
// one page at a time, through the one door that filters and pages across tenant_self.
func TestThePlatformFindsItsWorkspaces(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	finder := postgres.NewTenantQueryRepository(pageCursors())
	plans := postgres.NewPlanRepository()
	family := newPlan(t, planName("find"))
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, family, "") })

	ids := []shared.ID{seedWorkspace(ctx, t, "find-a"), seedWorkspace(ctx, t, "find-b"), seedWorkspace(ctx, t, "find-c")}
	reference := "cus_" + planName("lost")
	for i, id := range ids {
		if _, err := admin.Exec(ctx, `UPDATE tenant SET plan_id = $1 WHERE id = $2`, family.ID.String(), id.String()); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if _, err := admin.Exec(ctx, `UPDATE tenant SET external_reference = $1, status = 'SUSPENDED' WHERE id = $2`,
				reference, id.String()); err != nil {
				t.Fatal(err)
			}
		}
	}

	query := func(q adminrepo.TenantQuery) ([]adminrepo.TenantRecord, adminrepo.PageInfo) {
		t.Helper()
		var records []adminrepo.TenantRecord
		var page adminrepo.PageInfo
		if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			var err error
			records, page, err = finder.Query(ctx, q)
			return err
		}); err != nil {
			t.Fatalf("querying %+v: %v", q, err)
		}
		return records, page
	}

	// A provisioning answer lost after its key expired: the platform finds the workspace by its own
	// reference.
	found, _ := query(adminrepo.TenantQuery{Reference: reference, Size: 10})
	if len(found) != 1 || found[0].ID != ids[1] || found[0].ExternalReference != reference || found[0].PlanName != family.Name {
		t.Fatalf("by reference: %+v", found)
	}
	if suspended, _ := query(adminrepo.TenantQuery{Plan: family.Name, Status: identity.TenantSuspended, Size: 10}); len(suspended) != 1 {
		t.Errorf("by plan and status: %d", len(suspended))
	}

	// Page by page, oldest first, every workspace once.
	first, page := query(adminrepo.TenantQuery{Plan: family.Name, Size: 2})
	if len(first) != 2 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("first page: %d rows, %+v", len(first), page)
	}
	second, last := query(adminrepo.TenantQuery{Plan: family.Name, Size: 2, Cursor: page.NextCursor})
	if len(second) != 1 || last.HasMore || second[0].ID != ids[2] || first[0].ID != ids[0] {
		t.Errorf("second page: %+v, %+v", second, last)
	}

	var refusal *shared.Error
	err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		_, _, err := finder.Query(ctx, adminrepo.TenantQuery{Size: 2, Cursor: "not-a-cursor"})
		return err
	})
	if !errors.As(err, &refusal) || refusal.DetailCode != "shared.cursor_invalid" {
		t.Errorf("a cursor not this server's answered %v", err)
	}
}
