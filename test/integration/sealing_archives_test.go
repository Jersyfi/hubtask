// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	sealingrepo "github.com/Jersyfi/hubtask/core/application/repository/sealing"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// seedArchiveRun writes a backup run in the given status whose manifest names keyID, under a
// target of its own, and removes both when the test ends.
func seedArchiveRun(ctx context.Context, t *testing.T, tenant shared.ID, status, keyID string) {
	t.Helper()
	admin := adminPool(ctx, t)
	target, run := freshID(t), freshID(t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO backup_target (id, tenant_id, name, kind, config, encryption_mode, created_at)
		VALUES ($1, $2, $3, 'S3', '{"bucket":"census"}'::jsonb, 'AES256_GCM', now())`,
		target.String(), tenant.String(), freshName(t)); err != nil {
		t.Fatalf("seeding the target: %v", err)
	}
	manifest := `{"encryption":{"mode":"NONE"}}`
	if keyID != "" {
		manifest = `{"encryption":{"mode":"AES256_GCM","key_id":"` + keyID + `","archive_salt":"AAAAAAAAAAAAAAAAAAAAAA=="}}`
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO backup_run (id, target_id, tenant_id, trigger, mode, status, manifest)
		VALUES ($1, $2, $3, 'MANUAL', 'FULL', $4, $5::jsonb)`,
		run.String(), target.String(), tenant.String(), status, manifest); err != nil {
		t.Fatalf("seeding the run: %v", err)
	}
	t.Cleanup(func() {
		background := context.WithoutCancel(ctx)
		_, _ = admin.Exec(background, `DELETE FROM backup_run WHERE id = $1`, run.String())
		_, _ = admin.Exec(background, `DELETE FROM backup_target WHERE id = $1`, target.String())
	})
}

// UC-BAK-04 check 6: an archive this installation keeps names the master key it was written under,
// and the census counts it - only while the archive is kept, only in its own workspace - so that
// retiring the key says first what it would leave unreadable.
func TestTheCensusCountsTheArchivesStillKept(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	const keyID = "census-archive-key"
	for _, status := range []string{"SUCCEEDED", "SUCCEEDED", "EXPIRED", "FAILED", "RUNNING"} {
		seedArchiveRun(ctx, t, tenantA, status, keyID)
	}
	seedArchiveRun(ctx, t, tenantA, "SUCCEEDED", "")
	census := postgres.NewSealingRepository()

	count := func(tenant shared.ID) map[string]int64 {
		t.Helper()
		var got map[string]int64
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			counts, err := census.Count(ctx)
			got = counts[keyID]
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return got
	}

	if got := count(tenantB); got[sealingrepo.StoreBackupArchive] != 0 {
		t.Errorf("another tenant's archives were counted: %v", got)
	}
	// The two kept archives; not the expired, failed or running ones, not the unencrypted one.
	if got := count(tenantA); got[sealingrepo.StoreBackupArchive] != 2 || len(got) != 1 {
		t.Errorf("the census under %s is %v, want 2 kept archives and nothing else", keyID, got)
	}
}
