// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
)

// An inventory object's files (migration 0434, UC-AST-01 check 11): linked once, listed oldest
// first, counted, answered as the hub that holds them, counted by the reconciliation, and gone
// with the object; the workspace next door neither reads, adds nor removes one.
func TestAnObjectsFilesStayInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	hub, app, vm := seedCIs(ctx, t)
	file := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageAttachment))
	repo := mediaRepo()

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for i, want := range []bool{true, false} {
			added, err := repo.AttachToCi(ctx, vm, file.ID)
			if err != nil || added != want {
				t.Errorf("attach %d: %v %v", i, added, err)
			}
		}
		_, err := repo.AttachToCi(ctx, app, file.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		ids, err := repo.CiMediaIDs(ctx, vm)
		if err != nil || len(ids) != 1 || ids[0] != file.ID {
			t.Errorf("the ids: %v %v", ids, err)
		}
		if n, err := repo.CountCiMedia(ctx, vm); err != nil || n != 1 {
			t.Errorf("the count: %d %v", n, err)
		}
		page, err := repo.ListForCi(ctx, vm, work.Page{Size: 10})
		if err != nil || len(page.Objects) != 1 || page.Objects[0].ID != file.ID || page.Objects[0].FileName != "report.pdf" {
			t.Errorf("the page: %+v %v", page, err)
		}
		hubs, err := repo.ReferencingCis(ctx, file.ID)
		if err != nil || len(hubs) != 1 || hubs[0] != hub {
			t.Errorf("the hubs: %v %v", hubs, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Recount(ctx, created)
	}); err != nil {
		t.Fatal(err)
	}
	if got := findMedia(ctx, t, tenantA, file.ID).RefCount; got != 2 {
		t.Errorf("the reconciliation counted %d references, want 2", got)
	}

	// The workspace next door: nothing to read, nothing to add, nothing to remove.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if ids, _ := repo.CiMediaIDs(ctx, vm); len(ids) != 0 {
			t.Errorf("tenant B read the ids: %v", ids)
		}
		if n, _ := repo.CountCiMedia(ctx, vm); n != 0 {
			t.Errorf("tenant B counted %d", n)
		}
		if page, _ := repo.ListForCi(ctx, vm, work.Page{Size: 10}); len(page.Objects) != 0 {
			t.Errorf("tenant B listed %+v", page.Objects)
		}
		if hubs, _ := repo.ReferencingCis(ctx, file.ID); len(hubs) != 0 {
			t.Errorf("tenant B found the hubs %v", hubs)
		}
		if removed, _ := repo.DetachFromCi(ctx, vm, file.ID); removed {
			t.Error("tenant B removed a file")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := repo.AttachToCi(ctx, vm, file.ID)
		return err
	}); err == nil {
		t.Error("tenant B put a file on the object")
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		removed, err := repo.DetachFromCi(ctx, app, file.ID)
		if err != nil || !removed {
			t.Errorf("detach: %v %v", removed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `DELETE FROM cmdb_ci WHERE id = $1`, vm.String()); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM cmdb_ci_media WHERE media_id = $1`, file.ID.String()).Scan(&left); err != nil || left != 0 {
		t.Errorf("the files outlived their object: %d %v", left, err)
	}
}
