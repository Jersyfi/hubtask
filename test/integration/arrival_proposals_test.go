// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Proposals prepared on arrival (UC-JUM-07 check 10) against PostgreSQL as the application role.

// The switch is stored as written: on reads on, off after on reads off rather than leaving true in
// the merged document, and a document without the key - a fresh workspace, a restore or an import
// of an archive written before the key existed - reads off.
func TestTheArrivalSwitchSurvivesTheSettingsDocument(t *testing.T) {
	ctx := context.Background()
	w := seedJumbleWorld(ctx, t)
	workspaces := postgres.NewWorkspaceSettingsRepository()
	now := time.Now().UTC()

	switchTo := func(prepare bool) identity.Workspace {
		t.Helper()
		var read identity.Workspace
		if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
			stored, err := workspaces.Find(ctx)
			if err != nil {
				return err
			}
			stored.Settings.AiArrivalProposals = prepare
			if written, err := workspaces.Update(ctx, stored, stored.Version, now); err != nil || !written {
				t.Fatalf("writing %v: written %v, %v", prepare, written, err)
			}
			read, err = workspaces.Find(ctx)
			return err
		}); err != nil {
			t.Fatalf("switching to %v: %v", prepare, err)
		}
		return read
	}

	var fresh identity.Workspace
	if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
		var err error
		fresh, err = workspaces.Find(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if fresh.Settings.AiArrivalProposals {
		t.Error("a workspace without the key reads on")
	}
	if !switchTo(true).Settings.AiArrivalProposals {
		t.Error("switched on, it reads off")
	}
	if switchTo(false).Settings.AiArrivalProposals {
		t.Error("switched off after on, it reads on")
	}
	var stored string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT settings->>'ai_arrival_proposals' FROM tenant WHERE id = $1`, w.tenant.String()).Scan(&stored); err != nil {
		t.Fatalf("reading the document: %v", err)
	}
	if stored != "false" {
		t.Errorf("the document holds %q after switching off, want an explicit false", stored)
	}
}
