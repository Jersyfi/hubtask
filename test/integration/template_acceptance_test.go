// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// What D-06 promises once real rows are involved: a relative date becomes an absolute one against
// the anchor the request named, read in the caller's own zone and stored as the day it is; the tree
// arrives whole and parented; and the role that may read a collection may not stamp a template out
// into it.

// seedTemplateTenant gives this file a tenant of its own, with an administrator, a hub and a
// collection. Its own rather than the shared one, because tenant A carries a deliberately narrowed
// TASK profile that another file seeds (capability_profile_test.go): a tree of a task with work
// packages under it is exactly what that override forbids, and a test whose outcome depends on
// which file ran first is not a test.
func seedTemplateTenant(ctx context.Context, t *testing.T) (tenant, author, collection shared.ID) {
	t.Helper()
	admin := adminPool(ctx, t)

	tenant, author = freshID(t), freshID(t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Templates')`,
		tenant.String(), "templates-"+tenant.String()[24:]); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	if _, err := admin.Exec(ctx,
		`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Anna')`,
		author.String(), tenant.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	if _, err := admin.Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		 VALUES ($1, $2, $3, 'TENANT', 'ADMIN')`,
		freshID(t).String(), tenant.String(), author.String()); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}

	_, collection = hubWithCollection(ctx, t, tenant, author)
	return tenant, author, collection
}

type templateAcceptance struct {
	define      work.CreateTemplate
	remove      work.DeleteTemplate
	instantiate work.InstantiateTemplate
}

func templateAcceptanceHarness(ctx context.Context, t *testing.T) templateAcceptance {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	cursors := pageCursors()
	items := postgres.NewItemRepository(cursors)
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	hlc, err := clockadapter.NewHybridClock(fixed, "template-acceptance")
	if err != nil {
		t.Fatalf("the hybrid clock refused to start: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  unitOfWork, Audit: sink, Clock: fixed,
	}

	writer := work.TemplateWriter{
		Templates: templateRepo(), Containers: containerRepo(),
		Profiles:   postgres.NewCapabilityProfileRepository(),
		Authorizer: authorizer, Changes: postgres.NewChangeLog(), Audit: sink,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hlc,
	}
	return templateAcceptance{
		define: work.CreateTemplate{Writer: writer},
		remove: work.DeleteTemplate{Writer: writer},
		instantiate: work.InstantiateTemplate{
			Writer: writer, Items: items, ItemMembers: itemMemberRepo(),
			Visibility: authorizer, Events: postgres.NewOutbox(jobQueue(t)),
			Activity: work.ActivityJournal{Entries: historyRepo(), IDs: ids},
		},
	}
}

// templateActor is an administrator at the tenant, who may therefore both define a template and
// stamp it out.
func templateActor(tenant, account shared.ID, zone string) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account,
		AccountName: "Anna", TimeZone: zone,
		Scopes: []string{"templates:read", "templates:write", "items:read", "items:write"},
	}
}

// childrenOfItem reads the entries hanging directly under one, through the admin pool: the
// repository has no "list the children" statement - a client reads a collection rather than a
// parent - and inventing one for a test would be adding a query nothing else needs.
func childrenOfItem(
	ctx context.Context, t *testing.T, tenant, parent shared.ID,
) []domain.WorkItem {
	t.Helper()

	rows, err := adminPool(ctx, t).Query(ctx,
		`SELECT id FROM work_item WHERE tenant_id = $1 AND parent_id = $2 ORDER BY order_key`,
		tenant.String(), parent.String())
	if err != nil {
		t.Fatalf("reading the children: %v", err)
	}
	defer rows.Close()

	var children []domain.WorkItem
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("reading a child: %v", err)
		}
		children = append(children, findItem(ctx, t, tenant, shared.MustParseID(id)))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the children: %v", err)
	}
	return children
}

// countItemsIn is the "nothing was written" check, which needs a count rather than a page.
func countItemsIn(ctx context.Context, t *testing.T, tenant, collection shared.ID) int {
	t.Helper()

	var count int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM work_item WHERE tenant_id = $1 AND collection_id = $2`,
		tenant.String(), collection.String()).Scan(&count); err != nil {
		t.Fatalf("counting the entries: %v", err)
	}
	return count
}

func TestATemplateStampsOutATreeWithAbsoluteDates(t *testing.T) {
	ctx := context.Background()
	tenant, author, collection := seedTemplateTenant(ctx, t)
	harness := templateAcceptanceHarness(ctx, t)

	offset, err := domain.ParseTemplateOffset("P3D")
	if err != nil {
		t.Fatalf("the offset was refused: %v", err)
	}
	template, err := harness.define.Execute(ctx, templateActor(tenant, author, "Europe/Berlin"),
		work.CreateTemplateCommand{Spec: domain.TemplateSpec{
			Scope: string(domain.TemplateScopeCollection), ScopeID: collection,
			Name: "Move house " + freshName(t), RootType: string(domain.ItemTask),
			Root: domain.TemplateNode{
				Type: domain.ItemTask, Title: "Move house",
				Children: []domain.TemplateNode{
					{Type: domain.ItemWorkPackage, Title: "Book the van"},
					{
						Type: domain.ItemWorkPackage, Title: "Pack the kitchen",
						DueOffset: &offset, DueDateOnly: true,
					},
				},
			},
		}})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	// Monday the 7th of September, named by the request. The relative date is three days in, and
	// which day that is depends on where the person asking is standing.
	anchor := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	result, err := harness.instantiate.Execute(ctx, templateActor(tenant, author, "Europe/Berlin"),
		work.InstantiateTemplateCommand{
			TemplateID: template.ID, CollectionID: collection, Anchor: anchor,
		})
	if err != nil {
		t.Fatalf("instantiating failed: %v", err)
	}
	if result.Created != 3 {
		t.Fatalf("the tree came out with %d entries", result.Created)
	}
	if len(result.DroppedReferences) != 0 {
		t.Errorf("something was dropped: %v", result.DroppedReferences)
	}

	root := findItem(ctx, t, tenant, result.Root.ID)
	if root.Title != "Move house" || !root.ParentID.IsZero() {
		t.Errorf("the root is %q under %v", root.Title, root.ParentID)
	}

	children := childrenOfItem(ctx, t, tenant, root.ID)
	if len(children) != 2 {
		t.Fatalf("the root carries %d children", len(children))
	}
	var dated domain.WorkItem
	for _, child := range children {
		if child.ParentID != root.ID {
			t.Errorf("%q hangs under %v rather than the root", child.Title, child.ParentID)
		}
		if child.Title == "Pack the kitchen" {
			dated = child
		} else if child.Due != nil {
			t.Errorf("%q came out dated although the template gives it no offset", child.Title)
		}
	}

	// Berlin, three days on from the anchor, kept as a day rather than a moment.
	if dated.Due == nil || !dated.Due.DateOnly {
		t.Fatalf("the dated node came out as %+v", dated.Due)
	}
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("Europe/Berlin is unknown here: %v", err)
	}
	if day := dated.Due.At.In(berlin).Format("2006-01-02"); day != "2026-09-10" {
		t.Errorf("the resolved date is %s, and the anchor plus P3D is 2026-09-10", day)
	}

	// The deletion is soft, and what it stamped out stays where it is.
	if err := harness.remove.Execute(ctx, templateActor(tenant, author, "Europe/Berlin"),
		work.ChangeTemplateCommand{TemplateID: template.ID}); err != nil {
		t.Fatalf("deleting the template failed: %v", err)
	}
	if standing := findItem(ctx, t, tenant, root.ID); standing.Title != "Move house" {
		t.Error("the deletion took the tree it had stamped out with it")
	}
	if _, err := harness.instantiate.Execute(ctx, templateActor(tenant, author, "Europe/Berlin"),
		work.InstantiateTemplateCommand{
			TemplateID: template.ID, CollectionID: collection,
		}); err == nil {
		t.Error("a deleted template was stamped out again")
	}
}

// The role negative, against real memberships: a viewer may read the collection and may not write
// entries into it, so the instantiation is refused before anything is written.
func TestAViewerMayNotStampATemplateOut(t *testing.T) {
	ctx := context.Background()
	tenant, author, collection := seedTemplateTenant(ctx, t)
	harness := templateAcceptanceHarness(ctx, t)

	template, err := harness.define.Execute(ctx, templateActor(tenant, author, "UTC"),
		work.CreateTemplateCommand{Spec: domain.TemplateSpec{
			Scope: string(domain.TemplateScopeCollection), ScopeID: collection,
			Name: "Onboarding " + freshName(t), RootType: string(domain.ItemTask),
			Root: domain.TemplateNode{Type: domain.ItemTask, Title: "Onboard"},
		}})
	if err != nil {
		t.Fatalf("defining the template failed: %v", err)
	}

	viewer := seedAccount(ctx, t, tenant)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		 VALUES ($1, $2, $3, 'COLLECTION', $4, 'VIEWER')`,
		freshID(t).String(), tenant.String(), viewer.String(), collection.String()); err != nil {
		t.Fatalf("seeding the viewer: %v", err)
	}
	reading := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: viewer,
		AccountName: "Viewer", TimeZone: "UTC",
		Scopes: []string{"templates:read", "templates:write", "items:read", "items:write"},
	}

	if _, err := harness.instantiate.Execute(ctx, reading, work.InstantiateTemplateCommand{
		TemplateID: template.ID, CollectionID: collection,
	}); err == nil {
		t.Fatal("a viewer stamped a template out into a collection")
	}
	// And nothing was written on the way to the refusal.
	if count := countItemsIn(ctx, t, tenant, collection); count != 0 {
		t.Errorf("%d entries were written before the refusal", count)
	}
}
