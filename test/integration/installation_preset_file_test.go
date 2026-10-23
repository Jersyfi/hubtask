// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/instancefile"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The instance file's presets against PostgreSQL (UC-INS-18 check 1, the third door): read from a
// file, seeded once, enforced as the whole set - the default and its locks included - and skipped
// in a single installation.

func presetsOfFile(t *testing.T, document string) []map[string]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "instance.json")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	body, _, err := instancefile.Read(path)
	if err != nil {
		t.Fatalf("reading the file: %v", err)
	}
	presets, _, err := instancefile.Presets(body, path)
	if err != nil {
		t.Fatalf("reading its presets: %v", err)
	}
	return presets
}

func installationPresetsByName(ctx context.Context, t *testing.T) map[string]domain.InstallationPreset {
	t.Helper()
	byName := map[string]domain.InstallationPreset{}
	if err := postgres.NewUnitOfWork(appPool(ctx, t)).WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		listed, err := postgres.NewInstallationPresetRepository().List(ctx)
		for _, preset := range listed {
			byName[preset.Name] = preset
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return byName
}

func TestTheInstanceFileSeedsAndEnforcesTheInstallationsPresets(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	writer := installationPresetWriter(t, env.TenancyMulti, postgres.NewInstanceSettingRepository())
	t.Cleanup(func() {
		for _, preset := range installationPresetsByName(context.WithoutCancel(ctx), t) {
			_ = asInstallation(context.WithoutCancel(ctx), t, true, func(ctx context.Context) error {
				return postgres.NewInstallationPresetRepository().Delete(ctx, preset.ID, nil)
			})
		}
	})
	const source = "/etc/hubtask/instance.json"

	first := presetsOfFile(t, `{"presets": [
	  {"name": "Craft order", "preset": "FLOW", "is_default": true, "locks": ["views"]},
	  {"name": "Practice appointments", "preset": "LIST"}
	]}`)
	if single, err := installationPresetWriter(t, env.TenancySingle, postgres.NewInstanceSettingRepository()).
		ApplyFile(ctx, first, true, source); err != nil || !single.Skipped {
		t.Fatalf("a single installation answered %+v, %v", single, err)
	}
	if outcome, err := writer.ApplyFile(ctx, first, false, source); err != nil || outcome.Created != 2 {
		t.Fatalf("seeding answered %+v, %v", outcome, err)
	}
	seeded := installationPresetsByName(ctx, t)
	craft := seeded["Craft order"]
	if !craft.IsDefault || len(craft.Locks) != 1 || craft.Values.Views == nil || craft.Values.Views.Default != domain.ViewKanban {
		t.Fatalf("the seeded default reads %+v", craft)
	}

	// Seeded once: a second start with another file leaves what stands.
	other := presetsOfFile(t, `{"presets": [{"name": "Something else"}]}`)
	if outcome, err := writer.ApplyFile(ctx, other, false, source); err != nil || outcome.Created != 0 {
		t.Fatalf("seeding again answered %+v, %v", outcome, err)
	}

	// Enforced: unchanged is no write, a changed default moves with its locks, an unnamed one goes.
	if outcome, err := writer.ApplyFile(ctx, first, true, source); err != nil || outcome.Changed != 0 || outcome.Created != 0 {
		t.Fatalf("enforcing the same file answered %+v, %v", outcome, err)
	}
	second := presetsOfFile(t, `{"presets": [
	  {"name": "Practice appointments", "preset": "LIST", "is_default": true, "locks": ["fields.TASK.DUE_DATE"]}
	]}`)
	outcome, err := writer.ApplyFile(ctx, second, true, source)
	if err != nil || outcome.Changed != 1 || outcome.Withdrawn != 1 {
		t.Fatalf("enforcing the second file answered %+v, %v", outcome, err)
	}
	enforced := installationPresetsByName(ctx, t)
	practice, kept := enforced["Practice appointments"]
	if _, gone := enforced["Craft order"]; gone || !kept || !practice.IsDefault ||
		len(practice.Locks) != 1 || practice.Locks[0] != "fields.TASK.DUE_DATE" {
		t.Errorf("after enforcing: %+v", enforced)
	}

	// While it enforces, the file is still a writer the level lets through; an empty list withdraws all.
	byFile := installationPresetWriter(t, env.TenancyMulti, postgres.FromFile(source, true))
	if outcome, err := byFile.ApplyFile(ctx, presetsOfFile(t, `{"presets": []}`), true, source); err != nil || outcome.Withdrawn != 1 {
		t.Errorf("an empty enforcing list answered %+v, %v", outcome, err)
	}
}
