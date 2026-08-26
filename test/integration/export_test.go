// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The export against real rows (D-08), which is where it was found that the audit entry had no
// transaction to be written in: the selection reads through the query's own read-only
// transactions, one per page, and they have all closed by the time the export is decided. A fake
// unit of work cannot show that, and the end-to-end session did - so the check lives here now.

func exportHarness(ctx context.Context, t *testing.T) work.ExportView {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	cursors := pageCursors()
	fixed := portclock.Fixed(created)
	sink := postgres.NewAuditSink(clockadapter.NewUUIDv7(clockadapter.System{}))
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  unitOfWork, Audit: sink, Clock: fixed,
	}

	return work.ExportView{
		Views: postgres.NewSavedViewRepository(), Containers: containerRepo(),
		Permits: authorizer,
		Query: work.QueryItems{
			Items: postgres.NewItemRepository(cursors), ItemLabels: itemLabelRepo(),
			Containers: containerRepo(), Authorizer: authorizer,
			UnitOfWork: unitOfWork, Clock: fixed,
		},
		ItemLabels: itemLabelRepo(), Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
	}
}

// datedView saves a view over everything in the collection that carries a due date.
func datedView(ctx context.Context, t *testing.T, tenant, owner, collection shared.ID) view.SavedView {
	t.Helper()

	saved, err := view.NewSavedView(view.NewSavedViewInput{
		ID: freshID(t), TenantID: tenant, OwnerID: owner,
		ScopeType: view.ViewScopeCollection, ScopeID: collection,
		Name: "Everything dated", Layout: "LIST_EXPANDED",
		Query: map[string]any{
			"scope_container_id": collection.String(),
			"filter": map[string]any{
				"op": "NOT", "nodes": []any{map[string]any{"field": "due_at", "op": "IS_NULL"}},
			},
		},
		Sharing: view.SharingPrivate, Now: created,
	})
	if err != nil {
		t.Fatalf("the view was refused: %v", err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewSavedViewRepository().Insert(ctx, saved)
	}); err != nil {
		t.Fatalf("writing the view: %v", err)
	}
	return saved
}

func TestAnExportAnswersTheViewsRowsAndRecordsThatItHappened(t *testing.T) {
	ctx := context.Background()
	tenant, owner, collection := seedTemplateTenant(ctx, t)
	saved := datedView(ctx, t, tenant, owner, collection)

	// One dated entry and one without, so the stored filter has something to decide.
	dated := findItem(ctx, t, tenant, seedTask(ctx, t, tenant, owner, collection))
	dated.Due = &workmodel.DueDate{At: created.Add(24 * time.Hour)}
	dated.UpdatedAt = created.Add(time.Hour)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, dated, dated.Version)
	}); err != nil {
		t.Fatalf("seeding the due date: %v", err)
	}
	seedTask(ctx, t, tenant, owner, collection)

	exported, err := exportHarness(ctx, t).Execute(ctx, subscriber(tenant, owner), saved.ID)
	if err != nil {
		t.Fatalf("exporting failed: %v", err)
	}
	if len(exported.Items) != 1 || exported.Items[0].ID != dated.ID {
		t.Fatalf("the export is %d rows: %+v", len(exported.Items), exported.Items)
	}
	if exported.Truncated {
		t.Error("an export of one row reported itself truncated")
	}
	if exported.View.Name != "Everything dated" {
		t.Errorf("the export names the view %q", exported.View.Name)
	}

	// The entry that says a bulk read happened - written in a transaction of its own, which is
	// the whole point of this test.
	var entries int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM audit_log
		 WHERE tenant_id = $1 AND action = 'view.exported' AND target_id = $2`,
		tenant.String(), saved.ID.String()).Scan(&entries); err != nil {
		t.Fatalf("reading the audit trail: %v", err)
	}
	if entries != 1 {
		t.Errorf("the trail holds %d entries for the export", entries)
	}
}

// A view the caller cannot see is not found, and nothing is recorded on the way to saying so.
func TestExportingSomebodyElsesPrivateViewIsNotFound(t *testing.T) {
	ctx := context.Background()
	tenant, owner, collection := seedTemplateTenant(ctx, t)
	saved := datedView(ctx, t, tenant, owner, collection)

	stranger := seedAccount(ctx, t, tenant)
	_, err := exportHarness(ctx, t).Execute(ctx, subscriber(tenant, stranger), saved.ID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("exporting somebody else's private view answered %v", err)
	}
}
