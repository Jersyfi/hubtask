// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

func (s *snapshotStore) CustomFields(_ context.Context, after shared.ID, batch int) ([]work.CustomFieldDefinition, error) {
	return pageAfter(s.fields, func(r work.CustomFieldDefinition) string { return r.ID.String() }, after.String(), batch), nil
}

// A custom field definition travels where its form is drawn: a collection's to a reader of the
// collection, a workspace-wide one to every member - including one who may not read at the
// workspace itself - and never a collection's to a device that may not read it.
func TestTheWalkCarriesCustomFieldDefinitionsWhereTheirFormIsDrawn(t *testing.T) {
	pull, f, store := walking(t)
	wide, own, hidden := idAt('a', 1), idAt('a', 2), idAt('a', 3)
	store.fields = []work.CustomFieldDefinition{
		{ID: wide, Key: "cost_centre", Kind: work.CustomFieldText, AppliesTo: []work.ItemType{work.ItemTask}},
		{ID: own, CollectionID: collectionA, Key: "priority", Kind: work.CustomFieldSelect, Options: []string{"high"}},
		{ID: hidden, CollectionID: collectionB, Key: "secret", Kind: work.CustomFieldText},
	}
	f.auth.allowed[collectionB] = false
	f.auth.allowed[shared.ID("")] = false

	records, _ := wholeWalk(t, pull, 100)

	got := map[shared.ID]Record{}
	for _, record := range records {
		if record.Entity == "custom_field" {
			got[record.EntityID] = record
		}
	}
	if len(got) != 2 {
		t.Fatalf("definitions delivered %v, want the workspace's and the readable collection's", got)
	}
	if got[wide].Payload["collection_id"] != nil || got[wide].Payload["key"] != "cost_centre" {
		t.Errorf("the workspace-wide definition = %v", got[wide].Payload)
	}
	if got[own].ContainerID != collectionA || got[own].Payload["key"] != "priority" {
		t.Errorf("the collection's definition = %+v", got[own])
	}
}
