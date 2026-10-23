// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-WRK-31 check 2: where a collection applies REQUIRED_BEYOND_FIRST_COLUMN, an entry enters a
// column past the first only with its required fields. The structures are literal, never what the
// resolver answers for itself.

var (
	firstColumnID  = shared.MustParseID("0192f000-0000-7000-8000-000000000f01")
	secondColumnID = shared.MustParseID("0192f000-0000-7000-8000-000000000f02")
	customerField  = shared.MustParseID("0192f000-0000-7000-8000-000000000f11")
)

// flowHarness is the item harness with a two-column board, a required "customer" field and the
// collection's own layer applying the gate where flow is set.
func flowHarness(t *testing.T, flow bool, fields ...domain.Capability) *itemHarness {
	t.Helper()
	h := newItemHarness()
	h.profiles.rows[0].Capabilities = append(h.profiles.rows[0].Capabilities, domain.CapabilityCustomFields)
	h.handler.Structure = StructureResolver{
		Containers: h.containers, Structures: newStructureStore(h.containers), Profiles: h.profiles,
	}
	h.handler.Gate = EntryGate{Structure: h.handler.Structure, Types: h.items}
	board := &buckets{stored: map[shared.ID]domain.Bucket{
		secondColumnID: {ID: secondColumnID, CollectionID: collectionID, Name: "Doing", OrderKey: "b0", Version: 1},
		firstColumnID:  {ID: firstColumnID, CollectionID: collectionID, Name: "New", OrderKey: "a0", Version: 1},
	}}
	h.handler.Buckets = board
	h.handler.Flow = FlowGate{Buckets: board, Fields: h.fields}
	h.fields.stored[customerField] = domain.CustomFieldDefinition{
		ID: customerField, TenantID: tenantID, CollectionID: collectionID, Key: "customer",
		Kind: domain.CustomFieldText, IsRequired: true, AppliesTo: []domain.ItemType{domain.ItemTask}, Version: 1,
	}
	values := domain.StructureValues{}
	if flow {
		values.Automation = []domain.AutomationBehaviour{domain.AutomationRequiredBeyondFirstColumn}
	}
	if len(fields) > 0 {
		values.Fields = map[domain.ItemType]domain.LevelFields{domain.ItemTask: {Capabilities: fields}}
	}
	collection := h.containers.stored[collectionID]
	collection.Structure = domain.StructureLayer{Values: values}
	h.containers.stored[collectionID] = collection
	return h
}

func TestAnEntryLeavesTheFirstColumnOnlyWithItsRequiredFields(t *testing.T) {
	cases := []struct {
		name   string
		flow   bool
		fields []domain.Capability
		bucket shared.ID
		values map[string]any
		refuse bool
	}{
		{"the first column", true, nil, firstColumnID, nil, false},
		{"no column", true, nil, "", nil, false},
		{"past the first without the field", true, nil, secondColumnID, nil, true},
		{"past the first with the field", true, nil, secondColumnID, map[string]any{"customer": "ACME"}, false},
		{"a collection without the gate", false, nil, secondColumnID, nil, false},
		{"a collection whose entries carry no custom fields", true,
			[]domain.Capability{domain.CapabilityCompletion, domain.CapabilityBucket}, secondColumnID, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := flowHarness(t, tc.flow, tc.fields...)
			item := domain.WorkItem{Type: domain.ItemTask, CollectionID: collectionID, CustomFields: tc.values}
			err := h.handler.Flow.ensureMayEnter(
				context.Background(), h.handler.Structure, h.containers.stored[collectionID], item, tc.bucket,
			)
			if !tc.refuse {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("answered %v, want a validation failure", err)
			}
			refusal := shared.AsError(err)
			if refusal.DetailCode != "items.required_fields_missing" || refusal.Params["keys"] != "customer" ||
				len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/custom_fields/customer" {
				t.Errorf("refusal %+v", refusal)
			}
		})
	}
}

// A collection that asks for the gate where it cannot be asked is refused, never let through.
func TestTheGateFailsClosedWhereItIsNotWired(t *testing.T) {
	h := flowHarness(t, true)
	gate := FlowGate{}
	err := gate.ensureMayEnter(context.Background(), h.handler.Structure, h.containers.stored[collectionID],
		domain.WorkItem{Type: domain.ItemTask}, secondColumnID)
	if !errors.Is(err, shared.ErrInternal) {
		t.Errorf("answered %v", err)
	}
}

// A creation into a column past the first is refused whole without the field and passes with it.
func TestACreationIntoALaterColumnCarriesItsRequiredFields(t *testing.T) {
	h := flowHarness(t, true)
	registry, err := usecase.NewRegistry(nil, h.handler.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Invoke(context.Background(), CreateWorkItemName, itemActor(), usecase.Input{
		"type": "TASK", "collection_id": collectionID.String(), "title": "Printer jams",
		"bucket_id": secondColumnID.String(),
	})
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "items.required_fields_missing" {
		t.Fatalf("answered %v", err)
	}
	if !h.uow.rolledBack {
		t.Error("a refused creation was not rolled back")
	}

	out, err := registry.Invoke(context.Background(), CreateWorkItemName, itemActor(), usecase.Input{
		"type": "TASK", "collection_id": collectionID.String(), "title": "Printer jams",
		"bucket_id": secondColumnID.String(), "custom_fields": map[string]any{"customer": "ACME"},
	})
	if err != nil {
		t.Fatalf("with the field: %v", err)
	}
	if out["bucket_id"] != secondColumnID.String() {
		t.Errorf("created in %v", out["bucket_id"])
	}
}
