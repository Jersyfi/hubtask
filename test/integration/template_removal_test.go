// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	lifecyclerepo "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A deleted template's end, against a real database (data-retention.md §4): a deletion no hold
// covers removes the row, a held one stays deleted until the retention pass removes it, and the
// tenant boundary holds per new method (gate SG-3).

func seedTemplate(ctx context.Context, t *testing.T, tenant, collection shared.ID) work.Template {
	t.Helper()
	template := templateFor(t, tenant, collection, freshName(t))
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return templateRepo().Insert(ctx, template)
	}); err != nil {
		t.Fatalf("seeding the template: %v", err)
	}
	return template
}

func softDeleteTemplate(ctx context.Context, t *testing.T, tenant shared.ID, template work.Template) {
	t.Helper()
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return templateRepo().SetDeleted(ctx, template.Removed(created.Add(time.Hour)), template.Version)
	}); err != nil {
		t.Fatalf("deleting under a hold: %v", err)
	}
}

func templateRows(ctx context.Context, t *testing.T, id shared.ID) int {
	t.Helper()
	return countIn(ctx, t, `SELECT count(*) FROM template WHERE id = $1`, id.String())
}

func deletedTemplates(
	ctx context.Context, t *testing.T, tenant shared.ID,
) map[shared.ID]lifecyclerepo.DeletedTemplate {
	t.Helper()
	var page []lifecyclerepo.DeletedTemplate
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		page, err = postgres.DeletedTemplateRepository{}.Deleted(ctx, "", 1000)
		return err
	}); err != nil {
		t.Fatalf("reading the deleted templates: %v", err)
	}
	byID := make(map[shared.ID]lifecyclerepo.DeletedTemplate, len(page))
	for _, entry := range page {
		byID[entry.ID] = entry
	}
	return byID
}

// A deletion no hold covers removes the row: under the lock, and never a row already deleted -
// that one is a hold's.
func TestARemovedTemplateIsGone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	template := seedTemplate(ctx, t, tenantA, collection)

	stale := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().Remove(ctx, template, template.Version+1)
	})
	if !errors.Is(stale, shared.ErrVersionConflict) || templateRows(ctx, t, template.ID) != 1 {
		t.Fatalf("a stale removal answered %v", stale)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().Remove(ctx, template, template.Version)
	}); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if templateRows(ctx, t, template.ID) != 0 {
		t.Error("the removed template is still stored")
	}

	held := seedTemplate(ctx, t, tenantA, collection)
	softDeleteTemplate(ctx, t, tenantA, held)
	again := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().Remove(ctx, held, held.Version+1)
	})
	if !errors.Is(again, shared.ErrVersionConflict) || templateRows(ctx, t, held.ID) != 1 {
		t.Errorf("a held deleted template was removed by a person's deletion: %v", again)
	}
}

// The retention pass's half: it finds a held deleted template with its scope and the hub above,
// passes over a living one, and removes only what is still deleted.
func TestADeletedTemplateIsFoundAndRemoved(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection := hubWithCollection(ctx, t, tenantA, authorA)
	held := seedTemplate(ctx, t, tenantA, collection)
	living := seedTemplate(ctx, t, tenantA, collection)
	softDeleteTemplate(ctx, t, tenantA, held)

	found := deletedTemplates(ctx, t, tenantA)
	entry, ok := found[held.ID]
	if !ok {
		t.Fatalf("the deleted template is not listed: %+v", found)
	}
	if entry.ScopeID != collection || entry.HubID != hub {
		t.Errorf("listed %+v, want the collection %s under the hub %s", entry, collection, hub)
	}
	if _, listed := found[living.ID]; listed {
		t.Error("a living template is listed")
	}

	var removed []shared.ID
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		removed, err = postgres.DeletedTemplateRepository{}.Remove(ctx, []shared.ID{held.ID, living.ID})
		return err
	}); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if len(removed) != 1 || removed[0] != held.ID {
		t.Errorf("removed %v, want only the deleted template", removed)
	}
	if templateRows(ctx, t, held.ID) != 0 || templateRows(ctx, t, living.ID) != 1 {
		t.Error("the pass's removal took the wrong rows")
	}
}

// Tenant B neither removes tenant A's template, nor sees nor removes tenant A's deleted one.
func TestTemplateRemovalsAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	living := seedTemplate(ctx, t, tenantA, collection)
	held := seedTemplate(ctx, t, tenantA, collection)
	softDeleteTemplate(ctx, t, tenantA, held)

	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return templateRepo().Remove(ctx, living, living.Version)
	})
	if !errors.Is(err, shared.ErrVersionConflict) || templateRows(ctx, t, living.ID) != 1 {
		t.Errorf("tenant B removed tenant A's template: %v", err)
	}
	if _, listed := deletedTemplates(ctx, t, tenantB)[held.ID]; listed {
		t.Error("tenant B listed tenant A's deleted template")
	}
	var removed []shared.ID
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		removed, err = postgres.DeletedTemplateRepository{}.Remove(ctx, []shared.ID{held.ID})
		return err
	}); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if len(removed) != 0 || templateRows(ctx, t, held.ID) != 1 {
		t.Errorf("tenant B removed tenant A's deleted template (%v)", removed)
	}
}
