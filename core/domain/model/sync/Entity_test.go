// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import "testing"

func TestEveryEntityDeclaresOneMergeRule(t *testing.T) {
	cases := map[Entity]MergeRule{
		EntityEntryType: MergeVersioned, EntityLinkType: MergeVersioned, EntityWorkflow: MergeVersioned,
		EntityCustomRole: MergeVersioned, EntityEntryLink: MergePresence,
	}
	if len(cases) != len(Entities()) {
		t.Fatalf("%d entities, %d cases", len(Entities()), len(cases))
	}
	for entity, want := range cases {
		if !entity.Valid() || entity.Rule() != want {
			t.Errorf("%s: valid=%v rule=%s, want %s", entity, entity.Valid(), entity.Rule(), want)
		}
	}
	if Entity("custom_field").Valid() {
		t.Error("an entity the contract does not name is valid")
	}
}

func TestAPresenceIsNeverPatched(t *testing.T) {
	cases := []struct {
		rule MergeRule
		kind MutationKind
		want bool
	}{
		{MergePresence, EntityUpsert, true},
		{MergePresence, EntityPatch, false},
		{MergePresence, EntityDelete, true},
		{MergeVersioned, EntityPatch, true},
	}
	for _, c := range cases {
		if got := c.rule.Admits(c.kind); got != c.want {
			t.Errorf("%s admits %s = %v, want %v", c.rule, c.kind, got, c.want)
		}
	}
	if !EntityPatch.OnEntity() || ItemPatch.OnEntity() {
		t.Error("OnEntity tells entries and entities apart wrongly")
	}
}
