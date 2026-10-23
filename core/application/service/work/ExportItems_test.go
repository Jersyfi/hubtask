// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

func newItemsExport(h *exportHarness) ExportItems {
	return ExportItems{
		Query: h.export.Query, ItemLabels: &itemLabels{}, Audit: h.audit,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
}

// itemsReader holds the scope an export of entries asks for.
func itemsReader() appshared.ActorContext {
	acting := actor()
	acting.Scopes = []string{itemsRead}
	return acting
}

func collectionQuery() usecase.Input {
	return usecase.Input{
		"scope_container_id": collectionID.String(),
		"filter":             map[string]any{"field": "is_completed", "op": "EQ", "value": false},
		"group_by":           map[string]any{"field": "assignee_id"},
	}
}

// A collection as it is filtered, exported without saving a view: rows, the count recorded
// against the collection, and nothing of their content in the record (rule 10).
func TestExportingAQueryAnswersItsRowsAndRecordsTheRead(t *testing.T) {
	h := newExportHarness(t)
	h.items.result = repository.ItemQueryResult{Items: exportedItems(3)}

	out, err := newItemsExport(h).Descriptor().Handler.Invoke(t.Context(), itemsReader(), collectionQuery())
	if err != nil {
		t.Fatalf("exporting failed: %v", err)
	}
	if out.Int("count") != 3 || out["truncated"] != false {
		t.Errorf("the projection is %+v", out)
	}
	if rows, ok := out["rows"].([]usecase.Output); !ok || len(rows) != 3 {
		t.Errorf("the rows came back as %+v", out["rows"])
	}

	// The query's own filter reached the walk, and its grouping did not: a file is rows.
	if len(h.items.searched) != 1 || h.items.searched[0].Spec.Filter == nil {
		t.Fatalf("the walk asked %+v", h.items.searched)
	}
	if !h.items.searched[0].Spec.GroupBy.IsZero() || h.items.searched[0].Spec.Size != MaxPageSize {
		t.Errorf("the walk asked for %+v", h.items.searched[0].Spec)
	}

	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ItemsExportedAction {
		t.Fatalf("the audit trail is %+v", h.audit.entries)
	}
	entry := h.audit.entries[0]
	if entry.TargetType != containerTarget || entry.TargetID != collectionID {
		t.Errorf("the entry names %s %s", entry.TargetType, entry.TargetID)
	}
	if recorded, _ := entry.Changes["rows"].(map[string]any); recorded["to"] != "3" {
		t.Errorf("the entry records %+v", entry.Changes)
	}
	if _, carried := entry.Changes["title"]; carried {
		t.Error("the audit entry carries a title")
	}
}

// The walk and the cap are the view export's: a query that reaches the cap says so.
func TestAQueryExportStopsAtTheCap(t *testing.T) {
	h := newExportHarness(t)
	pages := make([]repository.ItemQueryResult, 0)
	for written := 0; written < view.MaxExportRows+MaxPageSize; written += MaxPageSize {
		pages = append(pages, repository.ItemQueryResult{
			Items: exportedItems(MaxPageSize),
			Info:  repository.PageInfo{HasMore: true, NextCursor: "next"},
		})
	}
	h.items.queryPages = pages

	spec, err := specOf(collectionQuery())
	if err != nil {
		t.Fatal(err)
	}
	exported, err := newItemsExport(h).Execute(t.Context(), itemsReader(), spec)
	if err != nil {
		t.Fatalf("exporting failed: %v", err)
	}
	if len(exported.Items) != view.MaxExportRows || !exported.Truncated {
		t.Errorf("the export is %d rows (truncated=%v)", len(exported.Items), exported.Truncated)
	}
}

// The scope's own permission decides, once: a refusal is a refusal, nothing is read and nothing
// recorded (T-04).
func TestAQueryExportTheCallerMayNotReadIsRefused(t *testing.T) {
	h := newExportHarness(t)
	h.authorizer.err = shared.ErrForbidden
	h.items.result = repository.ItemQueryResult{Items: exportedItems(3)}

	_, err := newItemsExport(h).Descriptor().Handler.Invoke(t.Context(), itemsReader(), collectionQuery())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("refused as %v", err)
	}
	if len(h.items.searched) != 0 || len(h.audit.entries) != 0 {
		t.Errorf("a refused export read %d times and recorded %d entries",
			len(h.items.searched), len(h.audit.entries))
	}

	// A token without the scope exports nothing.
	h.authorizer.err = nil
	if _, err := newItemsExport(h).Descriptor().Handler.Invoke(t.Context(), feedActor(),
		collectionQuery()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a token without items:read answered %v", err)
	}

	// An unanchored query is refused by the grammar before anything is asked.
	if _, err := newItemsExport(h).Descriptor().Handler.Invoke(t.Context(), itemsReader(),
		usecase.Input{}); shared.AsError(err) == nil {
		t.Errorf("an export without a scope answered %v", err)
	}
}
