// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The type registry against the real boundary (UC-FLW-01 checks 1 and 5, gate SG-3): a workspace
// stores, changes and archives its own types, a key is free per workspace, and the workspace next
// door neither reads nor changes nor deletes them.

var (
	etTenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe01")
	etTenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe02")
)

func TestAWorkspacesTypesAreItsOwn(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + etTenantA.String() + `', 'et-a', 'Types A') ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + etTenantB.String() + `', 'et-b', 'Types B') ON CONFLICT (id) DO NOTHING`,
		`DELETE FROM entry_type WHERE tenant_id IN ('` + etTenantA.String() + `', '` + etTenantB.String() + `')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	types := postgres.NewEntryTypeRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC().Truncate(time.Microsecond)
	bug, err := domain.NewEntryType(domain.NewEntryTypeInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe11"), TenantID: etTenantA,
		Key: "bug", Level: domain.ItemTask, Name: "Bug", Icon: "bug", ColorToken: "red",
		Behaviours: domain.EntryBehaviours{Completable: true, Schedulable: true},
		Now:        now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}

	inTenant(t, uow, etTenantA, func(ctx context.Context) error {
		if err := types.Insert(ctx, bug); err != nil {
			t.Fatalf("storing A's type: %v", err)
		}
		return nil
	})
	// The refused insert aborts its transaction, so it is the transaction's answer.
	if err := uow.Within(ctx, persistence.Scope{TenantID: etTenantA}, func(ctx context.Context) error {
		again := bug
		again.ID = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe12")
		return types.Insert(ctx, again)
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a taken key was stored twice: %v", err)
	}

	archive := true
	archived, _, err := bug.Changed(domain.EntryTypeChange{Archived: &archive}, now.Add(time.Minute), text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	inTenant(t, uow, etTenantA, func(ctx context.Context) error {
		if err := types.Update(ctx, archived, bug.Version); err != nil {
			t.Fatalf("archiving A's type: %v", err)
		}
		if err := types.Update(ctx, archived, bug.Version); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("a stale version overwrote the type: %v", err)
		}
		read, err := types.Find(ctx, "bug")
		if err != nil || !read.IsArchived() || read.Version != 2 {
			t.Errorf("A read back %+v, %v", read, err)
		}
		return nil
	})

	// The workspace next door: the same key is free there, and A's row is out of reach.
	inTenant(t, uow, etTenantB, func(ctx context.Context) error {
		if rows, err := types.List(ctx); err != nil || len(rows) != 0 {
			t.Errorf("B listed %v, %v", rows, err)
		}
		if _, err := types.Find(ctx, "bug"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("B found A's type: %v", err)
		}
		if err := types.Update(ctx, archived, 2); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("B changed A's type: %v", err)
		}
		if err := types.Delete(ctx, "bug"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("B deleted A's type: %v", err)
		}
		own := bug
		own.ID, own.TenantID = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe13"), etTenantB
		if err := types.Insert(ctx, own); err != nil {
			t.Errorf("the key is not free next door: %v", err)
		}
		return nil
	})

	inTenant(t, uow, etTenantA, func(ctx context.Context) error {
		if err := types.Delete(ctx, "bug"); err != nil {
			t.Errorf("A could not delete a type nobody carries: %v", err)
		}
		return nil
	})
}
