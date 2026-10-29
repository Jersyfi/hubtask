// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package condition_test

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

type linkRows []work.EntryLink

func (l linkRows) Of(context.Context, shared.ID, int) ([]work.EntryLink, error) { return l, nil }

// UC-PLN-01 check 8: a rule's condition reads the entry's links as `item.links` - type, side and
// the far end's identifier, never a title.
func TestTheItemCarriesItsLinks(t *testing.T) {
	entries, containers := workspace()
	other := shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")
	values := condition.Values{
		Envelope: itemEnvelope(), Now: readAt, Entries: entries, Containers: containers,
		Links: linkRows{
			{Type: work.LinkTypeBlocks, SourceID: valuesItem, TargetID: other},
			{Type: work.LinkTypeDependsOn, SourceID: other, TargetID: valuesItem},
		},
	}
	item, _ := resolved(t, values, "item").(map[string]any)
	links, _ := item["links"].([]any)
	if len(links) != 2 {
		t.Fatalf("item.links = %v", item["links"])
	}
	first, _ := links[0].(map[string]any)
	second, _ := links[1].(map[string]any)
	if first["type"] != "blocks" || first["outward"] != true || first["other_id"] != other.String() {
		t.Errorf("the outward link reads %v", first)
	}
	if second["type"] != "depends_on" || second["outward"] != false || len(second) != 3 {
		t.Errorf("the inward link reads %v", second)
	}
}
