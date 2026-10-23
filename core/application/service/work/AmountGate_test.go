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

// UC-WRK-25 check 2 and the gate of D11: an amount is written only where the collection's structure
// in force carries AMOUNT for the type. The structures here are literal, never what the resolver
// answers for itself (known-traps: a capability test can ask itself).

// amountProfiles is the installation's matrix with AMOUNT offered to a task, as migration 0188 seeds
// it.
func amountProfiles() []domain.CapabilityProfile {
	rows := dueDateProfiles()
	rows[0].Capabilities = append(rows[0].Capabilities, domain.CapabilityAmount)
	return rows
}

// listLayer is a collection's own layer naming AMOUNT for its tasks, as *List* does.
func listLayer() domain.StructureLayer {
	return domain.StructureLayer{Values: domain.StructureValues{Fields: map[domain.ItemType]domain.LevelFields{
		domain.ItemTask: {Capabilities: []domain.Capability{domain.CapabilityCompletion, domain.CapabilityAmount}},
	}}}
}

func withList(store *containers, on bool) {
	collection := store.stored[collectionID]
	collection.Structure = domain.StructureLayer{}
	if on {
		collection.Structure = listLayer()
	}
	store.stored[collectionID] = collection
}

func createRegistry(t *testing.T, list bool) (*itemHarness, *usecase.Registry) {
	t.Helper()
	h := newItemHarness()
	h.profiles.rows = amountProfiles()
	h.handler.Structure = StructureResolver{
		Containers: h.containers, Structures: newStructureStore(h.containers),
		Profiles: &profiles{rows: amountProfiles()},
	}
	withList(h.containers, list)
	registry, err := usecase.NewRegistry(nil, h.handler.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	return h, registry
}

func TestAnAmountIsCreatedWhereTheCollectionOffersIt(t *testing.T) {
	h, registry := createRegistry(t, true)
	out, err := registry.Invoke(context.Background(), CreateWorkItemName, itemActor(), usecase.Input{
		"type": "TASK", "collection_id": collectionID.String(), "title": "milk", "amount": " 2 ",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out["amount"] != "2" || out["title"] != "milk" {
		t.Errorf("answered amount %#v, title %#v", out["amount"], out["title"])
	}
	if len(h.items.inserted) != 1 || h.items.inserted[0].Amount != "2" {
		t.Errorf("stored %+v", h.items.inserted)
	}
	if payload := h.events.appended[0].Payload; payload["amount"] != "2" {
		t.Errorf("item.created carries amount %#v", payload["amount"])
	}
}

func TestAnAmountIsRefusedWhereTheCollectionDoesNotOfferIt(t *testing.T) {
	for name, list := range map[string]bool{"Hubtask's default": false} {
		t.Run(name, func(t *testing.T) {
			h, registry := createRegistry(t, list)
			_, err := registry.Invoke(context.Background(), CreateWorkItemName, itemActor(), usecase.Input{
				"type": "TASK", "collection_id": collectionID.String(), "title": "milk", "amount": "2",
			})
			if !errors.Is(err, shared.ErrCapabilityNotSupported) {
				t.Fatalf("error %v, want capability_not_supported", err)
			}
			refusal := shared.AsError(err)
			if refusal.Params["capability"] != "AMOUNT" || refusal.Params["item_type"] != "TASK" ||
				len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/amount" {
				t.Errorf("refusal %+v", refusal)
			}
			if len(h.items.inserted) != 0 {
				t.Error("an entry was written all the same")
			}
		})
	}

	// No amount needs nothing, wherever the entry lands.
	_, registry := createRegistry(t, false)
	if _, err := registry.Invoke(context.Background(), CreateWorkItemName, itemActor(), usecase.Input{
		"type": "TASK", "collection_id": collectionID.String(), "title": "milk", "amount": "  ",
	}); err != nil {
		t.Errorf("no amount was refused: %v", err)
	}
}

func TestAnAmountOverItsBoundIsRefusedByName(t *testing.T) {
	_, registry := createRegistry(t, true)
	_, err := registry.Invoke(context.Background(), CreateWorkItemName, itemActor(), usecase.Input{
		"type": "TASK", "collection_id": collectionID.String(), "title": "milk",
		"amount": "12345678901234567890123456789012345678901",
	})
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "items.amount_invalid" {
		t.Fatalf("error %v, want items.amount_invalid", err)
	}
}

// A resolver that is not wired offers no amount: the gate fails closed.
func TestAnAmountWithoutAResolverIsRefused(t *testing.T) {
	h, _ := createRegistry(t, true)
	h.handler.Structure = StructureResolver{}
	registry, err := usecase.NewRegistry(nil, h.handler.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Invoke(context.Background(), CreateWorkItemName, itemActor(), usecase.Input{
		"type": "TASK", "collection_id": collectionID.String(), "title": "milk", "amount": "2",
	}); !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("error %v, want capability_not_supported", err)
	}
}

func updateRegistry(t *testing.T, list bool, amount string) (*updateHarness, *usecase.Registry) {
	t.Helper()
	h := newUpdateHarness()
	rows := &profiles{rows: amountProfiles()}
	h.handler.Profiles = rows
	h.handler.Structure = StructureResolver{
		Containers: h.containers, Structures: newStructureStore(h.containers), Profiles: rows,
	}
	withList(h.containers, list)
	task := h.items.stored[taskID]
	task.Amount = amount
	h.items.stored[taskID] = task
	registry, err := usecase.NewRegistry(nil, h.handler.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	return h, registry
}

func TestAnAmountIsChangedAndClearedWhereTheCollectionOffersIt(t *testing.T) {
	h, registry := updateRegistry(t, true, "2")
	out, err := registry.Invoke(context.Background(), UpdateWorkItemName, itemActor(), usecase.Input{
		"item_id": taskID.String(), "amount": "3",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out["amount"] != "3" {
		t.Errorf("answered %#v", out["amount"])
	}
	write := h.written(t)
	if write.item.Amount != "3" {
		t.Errorf("stored %q", write.item.Amount)
	}
	var recorded bool
	for _, change := range h.changes.recorded {
		if change.Field == domain.FieldAmount && change.Payload[domain.FieldAmount] == "3" {
			recorded = true
		}
	}
	if !recorded {
		t.Errorf("no change-log entry for the amount: %+v", h.changes.recorded)
	}
	for _, entry := range h.audit.entries {
		recorded, _ := entry.Changes[domain.FieldAmount].(map[string]any)
		if recorded == nil || recorded["changed"] != true {
			t.Fatalf("the amount is not in the trail: %+v", entry.Changes)
		}
		if _, readable := recorded["to"]; readable {
			t.Errorf("the amount is in the trail in clear text: %+v", recorded)
		}
	}

	if _, err := registry.Invoke(context.Background(), UpdateWorkItemName, itemActor(), usecase.Input{
		"item_id": taskID.String(), "amount": "",
	}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
}

func TestAnAmountIsNotWrittenWhereTheCollectionDoesNotOfferIt(t *testing.T) {
	for name, amount := range map[string]string{"setting one": "", "clearing a hidden one": "2"} {
		t.Run(name, func(t *testing.T) {
			h, registry := updateRegistry(t, false, amount)
			value := "3"
			if amount != "" {
				value = ""
			}
			_, err := registry.Invoke(context.Background(), UpdateWorkItemName, itemActor(), usecase.Input{
				"item_id": taskID.String(), "amount": value,
			})
			if !errors.Is(err, shared.ErrCapabilityNotSupported) {
				t.Fatalf("error %v, want capability_not_supported", err)
			}
			if len(h.items.attributes) != 0 {
				t.Error("the entry was written all the same")
			}
		})
	}

	// Clearing an amount the entry never had asks for the state it is in.
	_, registry := updateRegistry(t, false, "")
	if _, err := registry.Invoke(context.Background(), UpdateWorkItemName, itemActor(), usecase.Input{
		"item_id": taskID.String(), "amount": "", "title": "Order the cable now",
	}); err != nil {
		t.Errorf("an empty amount on an entry without one: %v", err)
	}
}
