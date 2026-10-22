// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-WRK-24 check 3, stamping: a stamp writes new values, so what the collection hides is left
// out and reported - by the new entry's identifier, never the value - and a node of a hidden level
// refuses the stamp.

func TestAStampLeavesOutAndReportsWhatTheCollectionHides(t *testing.T) {
	h := newTemplateHarness(t)
	spec := domain.TemplateSpec{
		Scope: string(domain.TemplateScopeCollection), ScopeID: collectionID,
		Name: "Weekly shop", RootType: string(domain.ItemTask),
		Root: domain.TemplateNode{
			Type: domain.ItemTask, Title: "Weekly shop", Notes: "Bring the bags",
			AssigneeID: colleagueAccountID,
		},
	}
	offset, err := domain.ParseTemplateOffset("P1D")
	if err != nil {
		t.Fatal(err)
	}
	spec.Root.DueOffset = &offset
	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{Spec: spec})
	if err != nil {
		t.Fatalf("defining: %v", err)
	}
	h.instantiate.Writer.Gate = gateOver(h.items, h.containers)
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	result, err := h.instantiate.Execute(t.Context(), actor(), InstantiateTemplateCommand{
		TemplateID: template.ID, CollectionID: collectionID,
		Anchor: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("stamping into List failed: %v", err)
	}
	root := h.items.stored[result.Root.ID]
	if root.Notes != "" || root.Due != nil || !root.AssigneeID.IsZero() {
		t.Errorf("the stamp wrote values List hides: %+v", root)
	}
	kinds := map[domain.ReferenceKind]domain.DroppedReference{}
	for _, reference := range result.DroppedReferences {
		kinds[reference.Kind] = reference
	}
	for _, kind := range []domain.ReferenceKind{domain.ReferenceNotes, domain.ReferenceDueDate, domain.ReferenceAssignee} {
		reference, reported := kinds[kind]
		if !reported {
			t.Errorf("%s was left out without a report: %+v", kind, result.DroppedReferences)
			continue
		}
		if reference.Code != "items.capability_hidden" {
			t.Errorf("%s reported as %s", kind, reference.Code)
		}
	}
	if kinds[domain.ReferenceNotes].ID != result.Root.ID.String() {
		t.Errorf("the notes are reported by %q, want the entry's identifier", kinds[domain.ReferenceNotes].ID)
	}
}

func TestAStampWithANodeOfAHiddenLevelIsRefused(t *testing.T) {
	h := newTemplateHarness(t)
	template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{Spec: moveSpec(t)})
	if err != nil {
		t.Fatalf("defining: %v", err)
	}
	h.instantiate.Writer.Gate = gateOver(h.items, h.containers)
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	_, err = h.instantiate.Execute(t.Context(), actor(), InstantiateTemplateCommand{
		TemplateID: template.ID, CollectionID: collectionID,
		Anchor: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
	})
	refusedFor(t, err, "items.level_hidden")
}
