// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A definition reaches the change log under its scope as each write leaves it - defined, changed,
// deleted - so a device draws an entry's form offline (offline-sync.md §3.1).
func TestADefinitionIsToldToOfflineDevices(t *testing.T) {
	h := newCustomFieldHarness(t)
	log := &changes{}
	tell := FieldChanges{Changes: log, HLC: &hlcSource{}}
	h.define.Changes = tell

	defined := h.defined(t, DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "priority", Kind: domain.CustomFieldSelect, Options: []string{"high", "low"},
	})
	if len(log.recorded) != 1 {
		t.Fatalf("recorded %d changes, want one", len(log.recorded))
	}
	if got := log.recorded[0]; got.Entity != "custom_field" || got.EntityID != defined.ID || got.ContainerID != collectionID ||
		got.Op != changelog.Upsert || got.Payload["key"] != "priority" {
		t.Errorf("definition's change = %+v", got)
	}

	writer := h.writer()
	writer.Changes = tell
	required := true
	if _, err := (UpdateCustomField{Writer: writer}).Execute(t.Context(), actor(), UpdateCustomFieldCommand{
		FieldID: defined.ID, Attributes: domain.CustomFieldAttributes{IsRequired: &required},
	}); err != nil {
		t.Fatal(err)
	}
	if got := log.recorded[len(log.recorded)-1]; got.Payload["is_required"] != true || got.Payload["version"] != defined.Version+1 {
		t.Errorf("a change = %+v", got.Payload)
	}
	if err := (DeleteCustomField{Writer: writer}).Execute(t.Context(), actor(), DeleteCustomFieldCommand{FieldID: defined.ID}); err != nil {
		t.Fatal(err)
	}
	if got := log.recorded[len(log.recorded)-1]; got.Op != changelog.Delete || got.EntityID != defined.ID || got.ContainerID != collectionID {
		t.Errorf("a deletion = %+v, want a removal under the collection", got)
	}
}
