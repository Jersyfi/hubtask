// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-WRK-12 check 4: a set of people is held to the entry's readers element by element - one
// stranger refuses the whole value, with the answer an assignment gives.
func TestAPeopleValueNamingSomebodyOutOfReachIsRefused(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	h.withDefinition(t, collectionID, "reviewers", domain.CustomFieldUserSet)
	delete(h.visibility.reachable, strangerID)

	_, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "reviewers", Value: []any{accountID.String(), strangerID.String()},
	})
	if detail := shared.AsError(err).DetailCode; detail != "items.account_without_access" {
		t.Fatalf("detail %q, want items.account_without_access", detail)
	}
}

func TestAPeopleValueEveryoneCanReachIsStored(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	h.withDefinition(t, collectionID, "reviewers", domain.CustomFieldUserSet)

	written, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "reviewers", Value: []any{accountID.String(), strangerID.String()},
	})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if got, _ := written.CustomFields["reviewers"].([]any); len(got) != 2 {
		t.Errorf("the stored value is %#v", written.CustomFields["reviewers"])
	}
}

// UC-WRK-12 check 4: a reference to an entry that does not exist, or that is in the trash, is
// refused with one code; a reference to one the writer sees is stored.
func TestAReferenceNamesAnEntryTheWriterCanSee(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(h *valueHarness) shared.ID
		detail  string
	}{
		{"an entry that does not exist", func(*valueHarness) shared.ID {
			return shared.MustParseID("0192f000-0000-7000-8000-0000000000e9")
		}, "fields.value_not_an_entry"},
		{"an entry in the trash", func(h *valueHarness) shared.ID {
			id := shared.MustParseID("0192f000-0000-7000-8000-0000000000e8")
			h.items.stored[id] = domain.WorkItem{
				ID: id, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemTask,
				Title: "Gone", OrderKey: "a1", CreatedAt: now, UpdatedAt: now, DeletedAt: &now,
				Version: 1,
			}
			return id
		}, "fields.value_not_an_entry"},
		{"an entry out of the writer's reach", func(h *valueHarness) shared.ID {
			id := shared.MustParseID("0192f000-0000-7000-8000-0000000000e7")
			h.items.stored[id] = domain.WorkItem{
				ID: id, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemTask,
				Title: "Hidden", OrderKey: "a1", CreatedAt: now, UpdatedAt: now, Version: 1,
			}
			delete(h.visibility.reachable, accountID)
			return id
		}, "fields.value_not_an_entry"},
		{"an entry the writer sees", func(h *valueHarness) shared.ID {
			id := shared.MustParseID("0192f000-0000-7000-8000-0000000000e6")
			h.items.stored[id] = domain.WorkItem{
				ID: id, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemTask,
				Title: "Visible", OrderKey: "a1", CreatedAt: now, UpdatedAt: now, Version: 1,
			}
			return id
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newValueHarness(t)
			h.withItem(domain.ItemTask)
			h.withDefinition(t, collectionID, "blocked_by", domain.CustomFieldReference)
			target := c.prepare(h)

			written, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
				ItemID: filledItem, Key: "blocked_by", Value: target.String(),
			})
			if c.detail != "" {
				if detail := shared.AsError(err).DetailCode; detail != c.detail {
					t.Fatalf("detail %q, want %s", detail, c.detail)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if written.CustomFields["blocked_by"] != target.String() {
				t.Errorf("the stored value is %#v", written.CustomFields["blocked_by"])
			}
		})
	}
}
