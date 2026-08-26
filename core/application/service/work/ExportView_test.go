// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type exportHarness struct {
	export     ExportView
	views      *savedViews
	items      *items
	containers *containers
	authorizer *authorizer
	permits    *permitting
	audit      *sink
}

func newExportHarness(t *testing.T) *exportHarness {
	t.Helper()

	h := &exportHarness{
		views:      &savedViews{stored: map[shared.ID]view.SavedView{}},
		items:      &items{stored: map[shared.ID]domain.WorkItem{}, lastKey: "a0"},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		authorizer: &authorizer{},
		permits:    &permitting{},
		audit:      &sink{},
	}

	h.export = ExportView{
		Views: h.views, Containers: h.containers, Permits: h.permits,
		Query: QueryItems{
			Items: h.items, ItemLabels: &itemLabels{}, Containers: h.containers,
			Authorizer: h.authorizer, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
		},
		ItemLabels: &itemLabels{}, Audit: h.audit, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now),
	}

	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	return h
}

// withCollectionView stores a view whose query names the collection, which is what makes the
// export's walk resolvable.
func (h *exportHarness) withCollectionView(owner shared.ID) view.SavedView {
	saved, err := view.NewSavedView(view.NewSavedViewInput{
		ID: savedViewID, TenantID: tenantID, OwnerID: owner,
		ScopeType: view.ViewScopeCollection, ScopeID: collectionID,
		Name: "Due this week", Layout: "LIST_EXPANDED",
		Query: map[string]any{
			"scope_container_id": collectionID.String(),
			"filter": map[string]any{
				"field": "due_at", "op": "LTE", "value": "@today+P7D",
			},
		},
		Sharing: view.SharingPrivate,
		Now:     now,
	})
	if err != nil {
		t := err
		panic(t)
	}
	h.views.stored[saved.ID] = saved
	return saved
}

func exportedItems(count int) []domain.WorkItem {
	rows := make([]domain.WorkItem, 0, count)
	for i := range count {
		rows = append(rows, domain.WorkItem{
			ID: shared.ID(itemIdentifier(i)), TenantID: tenantID, CollectionID: collectionID,
			Type: domain.ItemTask, Title: "Pack the kitchen", OrderKey: "a0",
			CreatedBy: accountID, CreatedAt: now, Version: 1,
		})
	}
	return rows
}

// itemIdentifier makes as many distinct identifiers as a test needs, in the shape the parser
// accepts.
func itemIdentifier(i int) string {
	const digits = "0123456789abcdef"
	return "0192f000-0000-7000-8000-00000000" +
		string([]byte{digits[(i>>12)&0xf], digits[(i>>8)&0xf], digits[(i>>4)&0xf], digits[i&0xf]})
}

// The plain case: the caller's own view, one page, an audit entry that counts rather than quotes.
func TestExportingAViewAnswersItsRowsAndRecordsTheRead(t *testing.T) {
	h := newExportHarness(t)
	saved := h.withCollectionView(accountID)
	h.items.result = repository.ItemQueryResult{Items: exportedItems(3)}

	exported, err := h.export.Execute(t.Context(), feedActor(), saved.ID)
	if err != nil {
		t.Fatalf("exporting failed: %v", err)
	}
	if len(exported.Items) != 3 || exported.Truncated {
		t.Fatalf("the export is %d rows (truncated=%v)", len(exported.Items), exported.Truncated)
	}
	if exported.View.ID != saved.ID {
		t.Errorf("the export names view %s", exported.View.ID)
	}

	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ViewExportedAction {
		t.Fatalf("the audit trail is %+v", h.audit.entries)
	}
	// What was read, not what was in it: a count is not content (rule 10).
	recorded, _ := h.audit.entries[0].Changes["rows"].(map[string]any)
	if recorded["to"] != "3" {
		t.Errorf("the entry records %+v", h.audit.entries[0].Changes)
	}
	for _, key := range []string{"title", "notes"} {
		if _, carried := h.audit.entries[0].Changes[key]; carried {
			t.Errorf("the audit entry carries %s", key)
		}
	}
}

// The walk: an export asks for page after page and stops at the cap, saying that it did.
func TestAnExportWalksThePagesAndStopsAtTheCap(t *testing.T) {
	h := newExportHarness(t)
	saved := h.withCollectionView(accountID)

	// Full pages, more than the cap between them.
	pages := make([]repository.ItemQueryResult, 0)
	for written := 0; written < view.MaxExportRows+MaxPageSize; written += MaxPageSize {
		pages = append(pages, repository.ItemQueryResult{
			Items: exportedItems(MaxPageSize),
			Info:  repository.PageInfo{HasMore: true, NextCursor: "next"},
		})
	}
	h.items.queryPages = pages

	exported, err := h.export.Execute(t.Context(), feedActor(), saved.ID)
	if err != nil {
		t.Fatalf("exporting failed: %v", err)
	}
	if len(exported.Items) != view.MaxExportRows {
		t.Errorf("the export is %d rows, and the cap is %d",
			len(exported.Items), view.MaxExportRows)
	}
	if !exported.Truncated {
		t.Error("the export reached the cap and did not say so")
	}
	// And the walk carried the cursor rather than asking for page one over and over.
	if len(h.items.searched) < 2 || h.items.searched[1].Spec.Cursor != "next" {
		t.Errorf("the walk asked %d times, the second for cursor %q",
			len(h.items.searched), h.items.searched[1].Spec.Cursor)
	}
}

// An export that fits answers everything and says it was not truncated.
func TestAnExportThatFitsIsNotTruncated(t *testing.T) {
	h := newExportHarness(t)
	saved := h.withCollectionView(accountID)
	h.items.queryPages = []repository.ItemQueryResult{
		{Items: exportedItems(MaxPageSize), Info: repository.PageInfo{HasMore: true, NextCursor: "next"}},
		{Items: exportedItems(7)},
	}

	exported, err := h.export.Execute(t.Context(), feedActor(), saved.ID)
	if err != nil {
		t.Fatalf("exporting failed: %v", err)
	}
	if len(exported.Items) != MaxPageSize+7 || exported.Truncated {
		t.Errorf("the export is %d rows (truncated=%v)", len(exported.Items), exported.Truncated)
	}
}

// A view the caller cannot see is not found, and nothing is read on the way to saying so (T-04).
func TestExportingAViewTheCallerCannotSeeIsNotFound(t *testing.T) {
	h := newExportHarness(t)
	h.permits.allow = false
	saved := h.withCollectionView(otherOwner)
	saved.Sharing = view.SharingScope
	h.views.stored[saved.ID] = saved

	_, err := h.export.Execute(t.Context(), feedActor(), saved.ID)
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "views.not_found" {
		t.Fatalf("refused as %v", err)
	}
	if len(h.items.searched) != 0 {
		t.Error("rows were read for a view the caller cannot see")
	}
	if len(h.audit.entries) != 0 {
		t.Error("a refused export was recorded as one that happened")
	}
}

// The grouping a client draws with is not an export's business, and a stored query that groups
// still exports rows.
func TestAnExportOfAGroupedViewIsStillRows(t *testing.T) {
	h := newExportHarness(t)
	saved := h.withCollectionView(accountID)
	saved.Query["group_by"] = map[string]any{"field": "assignee_id"}
	h.views.stored[saved.ID] = saved
	h.items.result = repository.ItemQueryResult{Items: exportedItems(2)}

	if _, err := h.export.Execute(t.Context(), feedActor(), saved.ID); err != nil {
		t.Fatalf("exporting failed: %v", err)
	}
	if len(h.items.searched) != 1 {
		t.Fatalf("the walk asked %d times", len(h.items.searched))
	}
	if !h.items.searched[0].Spec.GroupBy.IsZero() {
		t.Errorf("the export asked for groups: %+v", h.items.searched[0].Spec.GroupBy)
	}
	if h.items.searched[0].Spec.Size != MaxPageSize {
		t.Errorf("the walk asked for pages of %d", h.items.searched[0].Spec.Size)
	}
}

// The catalogue's door, which is what an agent and an automation rule come through: rows, a count
// and the honest flag - the file itself is the channel's business.
func TestTheExportAnswersRowsThroughTheCatalogue(t *testing.T) {
	h := newExportHarness(t)
	saved := h.withCollectionView(accountID)
	h.items.result = repository.ItemQueryResult{Items: exportedItems(2)}

	out, err := h.export.Descriptor().Handler.Invoke(t.Context(), feedActor(),
		usecase.Input{"view_id": saved.ID.String()})
	if err != nil {
		t.Fatalf("exporting through the catalogue failed: %v", err)
	}
	if out.Int("count") != 2 || out["truncated"] != false {
		t.Errorf("the projection is %+v", out)
	}
	if out.String("view_name") != saved.Name {
		t.Errorf("the projection names the view %q", out.String("view_name"))
	}
	rows, ok := out["rows"].([]usecase.Output)
	if !ok || len(rows) != 2 || rows[0].String("title") != "Pack the kitchen" {
		t.Errorf("the rows came back as %+v", out["rows"])
	}

	// And the refusals, before anything is read.
	if _, err := h.export.Execute(t.Context(), feedActor(), ""); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "views.view_id_required" {
		t.Errorf("exporting nothing answered %v", err)
	}
	stranger := feedActor()
	stranger.Scopes = []string{"items:read"}
	if _, err := h.export.Execute(t.Context(), stranger, saved.ID); err == nil {
		t.Error("a token without the scope exported a view")
	}
}
