// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var (
	supportHub = shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	salesHub   = shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	copyID     = shared.MustParseID("0192f000-0000-7000-8000-0000000000e2")
)

// UC-FLW-01 check 10: a hub's own type is offered only inside that hub; a workspace's type
// everywhere.
func TestAHubsTypeIsOfferedOnlyInsideTheHub(t *testing.T) {
	common := newEntryType(t, bugType)
	own := bugType
	own.ID, own.Key, own.HubID = copyID, "incident", supportHub
	hubType := newEntryType(t, own)
	registry := work.NewEntryTypes([]work.EntryType{common, hubType})

	for _, tc := range []struct {
		name string
		hub  shared.ID
		want []string
	}{
		{"inside the hub", supportHub, []string{"bug", "incident"}},
		{"another hub", salesHub, []string{"bug"}},
		{"no hub", "", []string{"bug"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var keys []string
			for _, entryType := range registry.Within(tc.hub).Offered(work.ItemTask) {
				if !entryType.System {
					keys = append(keys, entryType.Key)
				}
			}
			if len(keys) != len(tc.want) {
				t.Fatalf("offered %v, want %v", keys, tc.want)
			}
			for i := range keys {
				if keys[i] != tc.want[i] {
					t.Errorf("offered %v, want %v", keys, tc.want)
				}
			}
			if _, err := registry.Within(tc.hub).Choose(work.ItemTask, "incident", nil); (err == nil) != (tc.hub == supportHub) {
				t.Errorf("choosing the hub's type: %v", err)
			}
		})
	}
}

// A copy takes everything of its source but its identity and its key, belongs to the hub, and
// names the key it came from; a system type is copied as well as a workspace's own.
func TestACopyHereBelongsToTheHubAndNamesItsSource(t *testing.T) {
	source := newEntryType(t, bugType)
	source.AllowedChildren = []string{"step"}
	copied, err := source.CopyAt(work.EntryTypeCopy{
		ID: copyID, HubID: supportHub, Key: "support_bug", Now: entryTypeNow, Text: text.Composing{},
	})
	if err != nil {
		t.Fatalf("the copy was refused: %v", err)
	}
	if copied.ID != copyID || copied.Key != "support_bug" || copied.HubID != supportHub || copied.CopiedFrom != "bug" {
		t.Errorf("the copy is %+v", copied)
	}
	if copied.Name != source.Name || copied.Icon != source.Icon || copied.Behaviours != source.Behaviours ||
		len(copied.AllowedChildren) != 1 || copied.Version != 1 {
		t.Errorf("the copy lost its source's shape: %+v", copied)
	}

	system := work.SystemEntryType(work.ItemTask)
	renamed := "Support task"
	fromSystem, err := system.CopyAt(work.EntryTypeCopy{
		ID: copyID, TenantID: tenant, HubID: supportHub, Key: "support_task", Name: &renamed,
		Now: entryTypeNow, Text: text.Composing{},
	})
	if err != nil || fromSystem.System || fromSystem.Name != renamed || fromSystem.CopiedFrom != "task" {
		t.Errorf("a system type's copy: %+v, %v", fromSystem, err)
	}

	for name, in := range map[string]work.EntryTypeCopy{
		"no hub":       {ID: copyID, Key: "x_copy", Now: entryTypeNow, Text: text.Composing{}},
		"reserved key": {ID: copyID, HubID: supportHub, Key: "task", Now: entryTypeNow, Text: text.Composing{}},
	} {
		if _, err := source.CopyAt(in); err == nil {
			t.Errorf("%s: the copy was made", name)
		}
	}
}
