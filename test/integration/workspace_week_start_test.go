// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A workspace's own week start lives in its settings document, so it travels with the document
// through the archive and every restore, and survives a write by a server that does not know it.

var weekStartTenant = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000a2b01")

func seedWeekStartTenant(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'week-start', 'Week start')
		 ON CONFLICT (id) DO NOTHING`, weekStartTenant.String()); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
}

func TestAWorkspacesWeekStartTravelsThroughARestore(t *testing.T) {
	ctx := context.Background()
	seedWeekStartTenant(ctx, t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`UPDATE tenant SET settings = settings || '{"week_start": "SATURDAY"}'::jsonb WHERE id = $1`,
		weekStartTenant.String()); err != nil {
		t.Fatalf("setting the week start: %v", err)
	}
	row := exportedRowOf(ctx, t, weekStartTenant, "tenant", weekStartTenant)

	if _, err := admin.Exec(ctx,
		`UPDATE tenant SET settings = settings - 'week_start' WHERE id = $1`, weekStartTenant.String()); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if err := write(ctx, t, weekStartTenant, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "tenant", row, true)
		return err
	}); err != nil {
		t.Fatalf("restoring the tenant row: %v", err)
	}

	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	inTenant(t, uow, weekStartTenant, func(ctx context.Context) error {
		workspace, err := postgres.NewWorkspaceSettingsRepository().Find(ctx)
		if err != nil {
			return err
		}
		if workspace.Settings.WeekStart != identity.WeekStartSaturday {
			t.Errorf("the restored week start is %q, want SATURDAY", workspace.Settings.WeekStart)
		}
		return nil
	})
}

// During a rolling update an older server writes the workspace beside a newer one. Its write merges
// the keys it models into the document, so a key only the newer one knows - here the week start's
// stand-in, a key this build does not model at all - is still there afterwards.
func TestAWorkspaceWriteKeepsTheKeysItDoesNotModel(t *testing.T) {
	ctx := context.Background()
	seedWeekStartTenant(ctx, t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`UPDATE tenant SET settings = settings || '{"a_later_key": "kept"}'::jsonb WHERE id = $1`,
		weekStartTenant.String()); err != nil {
		t.Fatalf("seeding a later key: %v", err)
	}

	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	repository := postgres.NewWorkspaceSettingsRepository()
	inTenant(t, uow, weekStartTenant, func(ctx context.Context) error {
		workspace, err := repository.Find(ctx)
		if err != nil {
			return err
		}
		workspace.DisplayName = "Week start, renamed"
		workspace.Settings.WeekStart = identity.WeekStartSunday
		written, err := repository.Update(ctx, workspace, workspace.Version, time.Now().UTC())
		if err != nil || !written {
			t.Fatalf("writing: %v (written %v)", err, written)
		}
		return nil
	})

	var kept, day string
	if err := admin.QueryRow(ctx,
		`SELECT coalesce(settings ->> 'a_later_key', ''), coalesce(settings ->> 'week_start', '') FROM tenant WHERE id = $1`,
		weekStartTenant.String()).Scan(&kept, &day); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if kept != "kept" {
		t.Errorf("a key the write does not model was lost: %q", kept)
	}
	if day != "SUNDAY" {
		t.Errorf("the week start is stored as %q", day)
	}
}
