// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// An inventory object's files (migration 0434, UC-AST-01 check 12): linked once, listed oldest
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

// An inventory object's history (migration 0435, UC-AST-01 check 13): appended, read newest first
// a page at a time with the change set as written, gone with the object; the workspace next door
// neither reads nor appends a step.
func TestAnObjectsHistoryStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	_, _, vm := seedCIs(ctx, t)
	repo := cmdbHistoryRepo()
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	step := func(verb cmdb.Verb, minutes int, changes map[string]cmdb.Moved) cmdb.Step {
		s, err := cmdb.NewStep(freshID(t), vm, shared.ActorUser, authorA, verb, changes, at.Add(time.Duration(minutes)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, s := range []cmdb.Step{
			step(cmdb.VerbCreated, 0, nil),
			step(cmdb.VerbStageChanged, 1, map[string]cmdb.Moved{"stage": {From: "IN_STOCK", To: "IN_USE"}}),
			step(cmdb.VerbChanged, 2, map[string]cmdb.Moved{"model": {Changed: true}, "source": {To: "netbox"}}),
		} {
			if err := repo.AppendCIHistory(ctx, s); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		first, err := repo.CIHistory(ctx, vm, work.Page{Size: 2})
		if err != nil {
			return err
		}
		if len(first.Steps) != 2 || first.Steps[0].Verb != cmdb.VerbChanged || !first.Info.HasMore ||
			first.Steps[0].Changes["model"] != (cmdb.Moved{Changed: true}) || first.Steps[0].Changes["source"].To != "netbox" ||
			first.Steps[1].Changes["stage"] != (cmdb.Moved{From: "IN_STOCK", To: "IN_USE"}) || first.Steps[0].ActorID != authorA {
			t.Errorf("the first page: %+v", first)
		}
		rest, err := repo.CIHistory(ctx, vm, work.Page{Size: 2, Cursor: first.Info.NextCursor})
		if err != nil || len(rest.Steps) != 1 || rest.Steps[0].Verb != cmdb.VerbCreated || rest.Info.HasMore {
			t.Errorf("the second page: %+v %v", rest, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if page, _ := repo.CIHistory(ctx, vm, work.Page{Size: 10}); len(page.Steps) != 0 {
			t.Errorf("tenant B read the history: %+v", page.Steps)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.AppendCIHistory(ctx, step(cmdb.VerbChanged, 3, nil))
	}); err == nil {
		t.Error("tenant B wrote a step into the object's history")
	}
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `DELETE FROM cmdb_ci WHERE id = $1`, vm.String()); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM cmdb_ci_history WHERE ci_id = $1`, vm.String()).Scan(&left); err != nil || left != 0 {
		t.Errorf("the history outlived its object: %d %v", left, err)
	}
}

func cmdbHistoryRepo() postgres.CmdbHistoryRepository {
	return postgres.NewCmdbHistoryRepository(pageCursors())
}
