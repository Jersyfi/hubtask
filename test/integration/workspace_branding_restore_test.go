// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
)

// A workspace with its logo, its symbol and the logo alone chosen, put back the way a restore writes
// an archive: entity by entity in the restore's order, the workspace's row before the media and the
// pictures after them. What the archive carries comes back - the two pictures and the choice - and
// the workspace reads them as before.
func TestAWorkspaceWithALogoIsRestoredWithItsLookAndItsChoice(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	t.Cleanup(func() { clearBranding(ctx, t, tenantA) })
	logo := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageWorkspaceLogo))
	symbol := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageWorkspaceLogo))
	want := identity.Branding{Header: identity.HeaderLogo, LogoLight: logo.ID, Symbol: symbol.ID}
	if err := setBranding(ctx, t, tenantA, want); err != nil {
		t.Fatalf("setting: %v", err)
	}

	// What the archive carries of the workspace, its two media and its pictures.
	held := map[string][]map[string]any{}
	for _, row := range exported(ctx, t, tenantA, 100, "tenant", time.Time{}) {
		if row.ID == tenantA.String() {
			held["tenant"] = append(held["tenant"], row.Data)
		}
	}
	for _, row := range exported(ctx, t, tenantA, 100, "media_object", time.Time{}) {
		if row.ID == logo.ID.String() || row.ID == symbol.ID.String() {
			held["media_object"] = append(held["media_object"], row.Data)
		}
	}
	for _, row := range exported(ctx, t, tenantA, 100, "workspace_picture", time.Time{}) {
		held["workspace_picture"] = append(held["workspace_picture"], row.Data)
	}
	if len(held["tenant"]) != 1 || len(held["media_object"]) != 2 || len(held["workspace_picture"]) != 2 {
		t.Fatalf("the archive carries %d workspace rows, %d media and %d pictures; want 1, 2 and 2",
			len(held["tenant"]), len(held["media_object"]), len(held["workspace_picture"]))
	}

	// All of it gone, as after a replace's clear of what this workspace holds of it.
	clearBranding(ctx, t, tenantA)
	for _, id := range []string{logo.ID.String(), symbol.ID.String()} {
		if _, err := admin.Exec(ctx, `DELETE FROM media_object WHERE tenant_id = $1 AND id = $2`, tenantA.String(), id); err != nil {
			t.Fatalf("removing a medium: %v", err)
		}
	}
	if found := workspaceOf(ctx, t, tenantA); found.Branding.HasPicture() || found.Branding.Header != identity.HeaderProduct {
		t.Fatalf("before the restore the workspace reads %+v", found.Branding)
	}

	for _, entity := range archive.RestoredEntities() {
		for _, row := range held[entity.Table] {
			if err := write(ctx, t, tenantA, func(ctx context.Context) error {
				_, err := importRepo().Write(ctx, entity.Table, row, true)
				return err
			}); err != nil {
				t.Fatalf("restoring %s: %v", entity.Table, err)
			}
		}
	}

	if found := workspaceOf(ctx, t, tenantA); found.Branding != want {
		t.Errorf("the restored workspace reads %+v, want %+v", found.Branding, want)
	}
}
