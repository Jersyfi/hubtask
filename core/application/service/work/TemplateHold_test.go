// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// templateHolds is the hold reading as a template's deletion sees it.
type templateHolds struct {
	active lifecycle.Holds
}

func (h *templateHolds) Active(context.Context) (lifecycle.Holds, error) { return h.active, nil }

func (h *templateHolds) Contributors(context.Context, []shared.ID) (map[shared.ID][]shared.ID, error) {
	return nil, nil
}

// removalLog records the journal entries and tombstones a removal leaves.
type removalLog struct {
	removals   []lifecycle.Removal
	purgeAfter time.Duration
}

func (r *removalLog) Record(
	_ context.Context, removals []lifecycle.Removal, deletedAt, purgeAfter time.Time,
) error {
	r.removals = append(r.removals, removals...)
	r.purgeAfter = purgeAfter.Sub(deletedAt)
	return nil
}

func templateHold(scope lifecycle.HoldScope, id shared.ID) lifecycle.LegalHold {
	return lifecycle.LegalHold{
		ID: "0192f000-0000-7000-8000-0000000000e1", Scope: scope, ScopeID: id,
		Reason: "dispute", PlacedBy: accountID, PlacedAt: now,
	}
}

// A deleted template is removed at once, with its journal entry and tombstone, unless a hold
// covers it (data-retention.md §4): the workspace, its collection or the hub above it. A hold on an
// entry or on a person never does - a template sits on no entry and names no author.
func TestADeletedTemplateIsRemovedUnlessAHoldCoversIt(t *testing.T) {
	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000000ee")

	cases := []struct {
		name  string
		holds lifecycle.Holds
		kept  bool
	}{
		{"no hold", nil, false},
		{"the workspace", lifecycle.Holds{templateHold(lifecycle.HoldTenant, "")}, true},
		{"its collection", lifecycle.Holds{templateHold(lifecycle.HoldContainer, collectionID)}, true},
		{"the hub above it", lifecycle.Holds{templateHold(lifecycle.HoldContainer, hubID)}, true},
		{"another hub", lifecycle.Holds{templateHold(lifecycle.HoldContainer, elsewhere)}, false},
		{"an entry", lifecycle.Holds{templateHold(lifecycle.HoldItem, elsewhere)}, false},
		{"a person", lifecycle.Holds{templateHold(lifecycle.HoldAccount, accountID)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newTemplateHarness(t)
			template, err := h.create.Execute(t.Context(), actor(), CreateTemplateCommand{Spec: moveSpec(t)})
			if err != nil {
				t.Fatalf("defining the template failed: %v", err)
			}
			h.holds.active = c.holds

			if err := h.remove.Execute(t.Context(), actor(), ChangeTemplateCommand{
				TemplateID: template.ID,
			}); err != nil {
				t.Fatalf("deleting failed: %v", err)
			}

			stored, found := h.templates.stored[template.ID]
			if c.kept {
				if !found || stored.DeletedAt == nil || len(h.templates.removed) != 0 {
					t.Errorf("under a hold the template is %+v (found %v)", stored, found)
				}
				if len(h.removals.removals) != 0 {
					t.Errorf("a kept template was journalled: %+v", h.removals.removals)
				}
			} else {
				if found {
					t.Errorf("the template is still stored: %+v", stored)
				}
				want := lifecycle.Removal{
					Entity: "template", EntityID: template.ID, Reason: lifecycle.DeletedByUser,
				}
				if len(h.removals.removals) != 1 || h.removals.removals[0] != want {
					t.Errorf("journalled %+v, want %+v", h.removals.removals, want)
				}
				if h.removals.purgeAfter != 90*24*time.Hour {
					t.Errorf("the tombstone lasts %v, want the offline window", h.removals.purgeAfter)
				}
			}
			// Either way it is gone for whoever asks, and the deletion is announced and audited.
			if _, err := h.get.Execute(t.Context(), actor(), template.ID); err == nil {
				t.Error("a deleted template was read back")
			}
			if len(h.audit.entries) == 0 || h.audit.entries[len(h.audit.entries)-1].Action != TemplateDeletedAction {
				t.Errorf("unexpected audit entries: %+v", h.audit.entries)
			}
		})
	}
}
