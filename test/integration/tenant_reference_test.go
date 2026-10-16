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

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
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
	err := set(second)
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "admin.reference_taken" {
		t.Fatalf("a second workspace with the reference answered %v", err)
	}
	if len(refusal.Params) != 0 || strings.Contains(err.Error(), reference) || errors.Unwrap(err) != nil {
		t.Errorf("the refusal carries the value or the database's detail: %v", err)
	}
}

// Provisioning with a reference another workspace holds is refused as the reference, not as the slug.
func TestProvisioningWithATakenReferenceSaysSo(t *testing.T) {
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
		return tenants.Insert(ctx, adminrepo.TenantRecord{
			ID: fresh, Slug: fmt.Sprintf("fresh-%09d", time.Now().UnixNano()%1_000_000_000), DisplayName: "Fresh", Status: identity.TenantActive,
			DefaultLocale: "en", DefaultTimeZone: "UTC", CreatedAt: time.Now().UTC(), ExternalReference: reference,
		})
	})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "admin.reference_taken" {
		t.Errorf("provisioning with a taken reference answered %v", err)
	}
}
