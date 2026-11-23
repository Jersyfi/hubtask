// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// clearBranding puts the shared tenant's row back as the other suites expect it.
func clearBranding(ctx context.Context, t *testing.T, tenant shared.ID) {
	t.Helper()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `DELETE FROM workspace_picture WHERE tenant_id = $1`, tenant.String()); err != nil {
		t.Fatalf("clearing the pictures: %v", err)
	}
	if _, err := admin.Exec(ctx, `UPDATE tenant SET settings = settings - 'header_shows' WHERE id = $1`,
		tenant.String()); err != nil {
		t.Fatalf("clearing the choice: %v", err)
	}
}

func lockBranding(ctx context.Context, t *testing.T, tenant shared.ID) (identity.Branding, error) {
	t.Helper()
	var held identity.Branding
	err := write(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		held, err = postgres.WorkspaceBrandingRepository{}.Lock(ctx)
		return err
	})
	return held, err
}

func setBranding(ctx context.Context, t *testing.T, tenant shared.ID, branding identity.Branding) error {
	t.Helper()
	return write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.WorkspaceBrandingRepository{}.Set(ctx, branding, created.Add(time.Hour))
	})
}

func workspaceOf(ctx context.Context, t *testing.T, tenant shared.ID) identity.Workspace {
	t.Helper()
	var found identity.Workspace
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		found, err = postgres.NewWorkspaceSettingsRepository().Find(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the workspace: %v", err)
	}
	return found
}

// UC-INS-22 check 1 against a real database, under the application role (migration 0449): the
// three pictures and the choice are written and read back; another workspace can neither name this
// one's picture (gate SG-3 for Set) nor read this one's branding (Lock, ListWorkspacePictures through
// Find), and does not see the picture served (ServesWorkspace).
func TestAWorkspacesBrandingIsWrittenAndReadInItsOwnWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	t.Cleanup(func() { clearBranding(ctx, t, tenantA); clearBranding(ctx, t, tenantB) })
	light := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageWorkspaceLogo))
	symbol := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageWorkspaceLogo))

	want := identity.Branding{Header: identity.HeaderLogo, LogoLight: light.ID, Symbol: symbol.ID}
	if err := setBranding(ctx, t, tenantA, want); err != nil {
		t.Fatalf("setting: %v", err)
	}
	if held, err := lockBranding(ctx, t, tenantA); err != nil || held != want {
		t.Fatalf("locked %+v, %v; want %+v", held, err, want)
	}
	if found := workspaceOf(ctx, t, tenantA); found.Branding != want {
		t.Errorf("the workspace reads %+v, want %+v", found.Branding, want)
	}

	// Another workspace naming this one's picture: refused as missing, and nothing written there.
	err := setBranding(ctx, t, tenantB, identity.Branding{Header: identity.HeaderLogo, LogoDark: light.ID})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("tenant B set tenant A's picture: %v, want not found", err)
	}
	if held, err := lockBranding(ctx, t, tenantB); err != nil || held != (identity.Branding{Header: identity.HeaderProduct}) {
		t.Errorf("tenant B holds %+v, %v; want its own, untouched", held, err)
	}
	if found := workspaceOf(ctx, t, tenantB); found.Branding.HasPicture() {
		t.Errorf("tenant B reads %+v", found.Branding)
	}
	for tenant, want := range map[shared.ID]bool{tenantA: true, tenantB: false} {
		var serves bool
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			serves, err = mediaRepo().ServesWorkspace(ctx, light.ID)
			return err
		}); err != nil || serves != want {
			t.Errorf("tenant %s: serves %v, %v; want %v", tenant, serves, err, want)
		}
	}

	// The workspace's own write of its settings merges past the choice.
	workspace := workspaceOf(ctx, t, tenantA)
	workspace.DisplayName = "A, renamed"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := postgres.NewWorkspaceSettingsRepository().Update(ctx, workspace, workspace.Version, created.Add(2*time.Hour))
		return err
	}); err != nil {
		t.Fatalf("renaming: %v", err)
	}
	if found := workspaceOf(ctx, t, tenantA); found.Branding != want {
		t.Errorf("after the workspace's own write it reads %+v, want %+v", found.Branding, want)
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE tenant SET display_name = 'A' WHERE id = $1`, tenantA.String()); err != nil {
		t.Fatalf("naming it back: %v", err)
	}
}

// The media reconciliation counts the workspace's pictures (Media.sql's recount): a picture the
// workspace shows is never stamped unreferenced, a replaced one is, and a picture that goes clears
// its row rather than failing.
func TestTheSweepCountsTheWorkspacesPictures(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	t.Cleanup(func() { clearBranding(ctx, t, tenantA) })
	first := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageWorkspaceLogo))
	second := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageWorkspaceLogo))
	recount := func(at time.Time) {
		t.Helper()
		if err := write(ctx, t, tenantA, func(ctx context.Context) error { return mediaRepo().Recount(ctx, at) }); err != nil {
			t.Fatalf("recounting: %v", err)
		}
	}

	// One object as both logos counts twice, once per column that names it.
	if err := setBranding(ctx, t, tenantA, identity.Branding{
		Header: identity.HeaderProduct, LogoLight: first.ID, LogoDark: first.ID,
	}); err != nil {
		t.Fatalf("setting: %v", err)
	}
	recount(created.Add(time.Hour))
	if stored := findMedia(ctx, t, tenantA, first.ID); stored.RefCount != 2 {
		t.Errorf("ref_count %d, want 2", stored.RefCount)
	}
	if stamp := unreferencedSince(ctx, t, first.ID); stamp != nil {
		t.Errorf("the workspace's logo was stamped unreferenced at %s", stamp)
	}

	if err := setBranding(ctx, t, tenantA, identity.Branding{Header: identity.HeaderProduct, Symbol: second.ID}); err != nil {
		t.Fatalf("replacing: %v", err)
	}
	recount(created.Add(2 * time.Hour))
	if stamp := unreferencedSince(ctx, t, first.ID); stamp == nil {
		t.Error("the replaced logo was not stamped unreferenced")
	}
	if stamp := unreferencedSince(ctx, t, second.ID); stamp != nil {
		t.Errorf("the symbol in use was stamped at %s", stamp)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := mediaRepo().RemoveRows(ctx, []shared.ID{second.ID})
		return err
	}); err != nil {
		t.Fatalf("removing the symbol: %v", err)
	}
	if held, err := lockBranding(ctx, t, tenantA); err != nil || !held.Symbol.IsZero() {
		t.Errorf("after the picture went the workspace holds %+v, %v", held, err)
	}
}
