// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The statements the templates run on, against a real database (D-06): the tree round-trips as the
// document it is, a name is free again once a template is deleted, and the tenant boundary holds
// per method (gate SG-3).

func templateRepo() postgres.TemplateRepository {
	return postgres.NewTemplateRepository(pageCursors())
}

func templateFor(
	t *testing.T, tenant, collection shared.ID, name string,
) work.Template {
	t.Helper()

	offset, err := work.ParseTemplateOffset("P3D")
	if err != nil {
		t.Fatalf("the offset was refused: %v", err)
	}
	template, err := work.NewTemplate(work.NewTemplateInput{
		ID: freshID(t), TenantID: tenant,
		Spec: work.TemplateSpec{
			Scope: string(work.TemplateScopeCollection), ScopeID: collection,
			Name: name, Description: "Everything a move needs",
			RootType: string(work.ItemTask),
			Root: work.TemplateNode{
				Type: work.ItemTask, Title: "Move house",
				Children: []work.TemplateNode{
					{Type: work.ItemWorkPackage, Title: "Book the van"},
					{
						Type: work.ItemWorkPackage, Title: "Pack the kitchen",
						DueOffset: &offset, DueDateOnly: true,
					},
				},
			},
		},
		Now: created,
	})
	if err != nil {
		t.Fatalf("the template was refused: %v", err)
	}
	return template
}

func TestATemplateRoundTripsAsTheTreeItIs(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	template := templateFor(t, tenantA, collection, freshName(t))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().Insert(ctx, template)
	}); err != nil {
		t.Fatalf("writing the template: %v", err)
	}

	var stored work.Template
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = templateRepo().Find(ctx, template.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the template: %v", err)
	}

	if stored.Name != template.Name || stored.RootType != work.ItemTask ||
		stored.Scope != work.TemplateScopeCollection || stored.Version != 1 {
		t.Errorf("the template came back as %+v", stored)
	}
	if stored.NodeCount() != 3 {
		t.Fatalf("the tree came back with %d nodes", stored.NodeCount())
	}
	packing := stored.Root.Children[1]
	if packing.DueOffset == nil || *packing.DueOffset != 72*time.Hour || !packing.DueDateOnly {
		t.Errorf("the relative date came back as %+v", packing)
	}

	// The whole document, under the lock.
	renamed := "Move flat"
	changed, _, err := stored.Changed(work.TemplatePatch{Name: &renamed}, created.Add(time.Hour))
	if err != nil {
		t.Fatalf("the change was refused: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().Update(ctx, changed, stored.Version)
	}); err != nil {
		t.Fatalf("writing the change: %v", err)
	}

	conflict := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().Update(ctx, changed, stored.Version)
	})
	if !errors.Is(conflict, shared.ErrVersionConflict) {
		t.Errorf("a stale change answered %v", conflict)
	}
}

// The name rule, and the C-07 lesson with it: two live templates in one scope may not share a
// name, a deleted one frees it, and what comes back under that name is the new template rather
// than the old one.
func TestADeletedTemplateFreesItsNameAndDoesNotComeBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	name := freshName(t)
	first := templateFor(t, tenantA, collection, name)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().Insert(ctx, first)
	}); err != nil {
		t.Fatalf("writing the template: %v", err)
	}

	taken := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().Insert(ctx, templateFor(t, tenantA, collection, name))
	})
	if !errors.Is(taken, shared.ErrConflict) {
		t.Fatalf("a second template under the same name answered %v", taken)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().SetDeleted(ctx, first.Removed(created.Add(time.Hour)), first.Version)
	}); err != nil {
		t.Fatalf("deleting the template: %v", err)
	}

	second := templateFor(t, tenantA, collection, name)
	second.Description = "A different tree entirely"
	second.Root.Children = second.Root.Children[:1]
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().Insert(ctx, second)
	}); err != nil {
		t.Fatalf("writing the second template: %v", err)
	}

	var stored work.Template
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = templateRepo().Find(ctx, second.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if stored.NodeCount() != 2 || stored.Description != "A different tree entirely" {
		t.Errorf("the recreated name answered the old template: %+v", stored)
	}

	// And the deleted one is out of the list rather than merely renamed.
	listed := listTemplates(ctx, t, tenantA, collection)
	for _, template := range listed {
		if template.ID == first.ID {
			t.Error("a deleted template is still in the list")
		}
	}
}

// The list is the path's: what is defined on the container and what is workspace-wide.
func TestTheListAnswersThePathAndTheWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection := hubWithCollection(ctx, t, tenantA, authorA)

	here := templateFor(t, tenantA, collection, freshName(t))
	above := templateFor(t, tenantA, hub, freshName(t))
	above.Scope = work.TemplateScopeHub
	everywhere := templateFor(t, tenantA, collection, freshName(t))
	everywhere.Scope, everywhere.ScopeID = work.TemplateScopeTenant, ""
	_, elsewhere := hubWithCollection(ctx, t, tenantA, authorA)
	other := templateFor(t, tenantA, elsewhere, freshName(t))

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, template := range []work.Template{here, above, everywhere, other} {
			if err := templateRepo().Insert(ctx, template); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the templates: %v", err)
	}

	found := map[shared.ID]bool{}
	for _, template := range listTemplates(ctx, t, tenantA, collection, hub) {
		found[template.ID] = true
	}
	if !found[here.ID] || !found[above.ID] || !found[everywhere.ID] {
		t.Errorf("the list misses what it owes: %v", found)
	}
	if found[other.ID] {
		t.Error("a template defined in another collection is in the list")
	}
}

func listTemplates(
	ctx context.Context, t *testing.T, tenant shared.ID, scopes ...shared.ID,
) []work.Template {
	t.Helper()

	var page repository.TemplatePage
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		page, err = templateRepo().ListInScopes(ctx, scopes, repository.Page{Size: 50})
		return err
	}); err != nil {
		t.Fatalf("listing the templates: %v", err)
	}
	return page.Templates
}

// Gate SG-3: one negative per port method.
func TestTemplatesAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	template := templateFor(t, tenantA, collection, freshName(t))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().Insert(ctx, template)
	}); err != nil {
		t.Fatalf("seeding the template: %v", err)
	}

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := templateRepo().Find(ctx, template.ID)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("tenant B's find answered %v", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		if listed := listTemplates(ctx, t, tenantB, collection); len(listed) != 0 {
			t.Errorf("tenant B listed %d of tenant A's templates", len(listed))
		}
	})

	t.Run("insert", func(t *testing.T) {
		// scope_id carries no foreign key - one column, three referents - so the insert does not
		// fail: it lands in tenant B, because the statement writes current_tenant_id() and never
		// the caller's claim. What the boundary owes is that the row is B's and invisible to A.
		foreign := templateFor(t, tenantA, collection, freshName(t))
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return templateRepo().Insert(ctx, foreign)
		}); err != nil {
			t.Fatalf("tenant B's insert answered %v", err)
		}
		err := read(ctx, t, tenantA, func(ctx context.Context) error {
			_, err := templateRepo().Find(ctx, foreign.ID)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant A can see tenant B's template: %v", err)
		}
	})

	t.Run("update", func(t *testing.T) {
		renamed := "Stolen"
		changed, _, err := template.Changed(
			work.TemplatePatch{Name: &renamed}, created.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		writeErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			return templateRepo().Update(ctx, changed, template.Version)
		})
		if !errors.Is(writeErr, shared.ErrVersionConflict) {
			t.Fatalf("tenant B's change answered %v", writeErr)
		}
	})

	t.Run("delete", func(t *testing.T) {
		writeErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			return templateRepo().SetDeleted(ctx, template.Removed(created), template.Version)
		})
		if !errors.Is(writeErr, shared.ErrVersionConflict) {
			t.Fatalf("tenant B's deletion answered %v", writeErr)
		}
	})
}
