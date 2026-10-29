// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// walk answers the entries it holds after the cursor, in order.
type walk struct{ ids []shared.ID }

func (w walk) IDsAfter(_ context.Context, _, after shared.ID, limit int) ([]shared.ID, error) {
	var page []shared.ID
	for _, id := range w.ids {
		if id.String() > after.String() && len(page) < limit {
			page = append(page, id)
		}
	}
	return page, nil
}

// UC-WRK-12, domain-model.md §3.5: a CALCULATED field defined after its entries fills them - each
// entry recalculated as a write of an input would, as the system's act - and the fill hands its
// cursor on to the next page's job until the walk reaches the end.
func TestANewCalculatedFieldFillsTheEntriesDefinedBeforeIt(t *testing.T) {
	ctx := context.Background()
	h := newValueHarness(t)
	item := h.withItem(domain.ItemTask)
	item.CustomFields = map[string]any{"hours": 3.0, "rate": 12.5}
	h.items.stored[filledItem] = item
	h.withDefinition(t, collectionID, "hours", domain.CustomFieldNumber)
	h.withDefinition(t, collectionID, "rate", domain.CustomFieldNumber)
	cost := h.withDefinition(t, collectionID, "cost", domain.CustomFieldNumber)
	cost.Kind, cost.Formula = domain.CustomFieldCalculated, "entry.custom_fields.hours * entry.custom_fields.rate"
	h.fields.stored[cost.ID] = cost
	h.set.Compiler = product{inputs: []string{"hours", "rate"}}

	queued := &jobs{}
	fill := FillCalculatedFields{Walk: walk{ids: []shared.ID{filledItem}}, Setter: h.set, Jobs: queued, BatchSize: 1}
	system := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantID}
	outcome, err := fill.Execute(ctx, system, FillCalculatedFieldsCommand{DefinitionID: cost.ID, CollectionID: collectionID})
	if err != nil {
		t.Fatal(err)
	}
	if h.items.stored[filledItem].CustomFields["cost"] != 37.5 || outcome.Filled != 1 || outcome.Next != filledItem {
		t.Fatalf("filled %+v, outcome %+v", h.items.stored[filledItem].CustomFields, outcome)
	}
	if len(queued.enqueued) != 1 || queued.enqueued[0].Kind != queue.KindFieldFill ||
		queued.enqueued[0].Payload["after"] != filledItem.String() {
		t.Fatalf("the next page's job: %+v", queued.enqueued)
	}

	outcome, err = fill.Execute(ctx, system, FillCalculatedFieldsCommand{DefinitionID: cost.ID, After: filledItem})
	if err != nil || !outcome.Done || outcome.Filled != 0 || len(queued.enqueued) != 1 {
		t.Fatalf("the walk's end: %+v, %v, %d jobs", outcome, err, len(queued.enqueued))
	}
}

// Defining a CALCULATED field seeds its fill in the definition's own transaction; any other kind
// seeds nothing.
func TestDefiningACalculatedFieldSeedsItsFill(t *testing.T) {
	h := newCustomFieldHarness(t)
	queued := &jobs{}
	h.define.Jobs = queued
	h.define.Compiler = product{}
	if _, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "hours", Kind: domain.CustomFieldNumber,
	}); err != nil {
		t.Fatal(err)
	}
	if len(queued.enqueued) != 0 {
		t.Fatalf("a number field seeded %+v", queued.enqueued)
	}
	defined, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "double", Kind: domain.CustomFieldCalculated,
		Formula: "entry.custom_fields.hours * 2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(queued.enqueued) != 1 || queued.enqueued[0].Payload["definition_id"] != defined.ID.String() ||
		queued.enqueued[0].Payload["collection_id"] != collectionID.String() {
		t.Fatalf("the fill seeded %+v", queued.enqueued)
	}
}
