// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// deletedTemplateStore is DeletedTemplates as an in-memory list, paged by identifier as the query is.
type deletedTemplateStore struct {
	templates []repository.DeletedTemplate
	pages     int
	removed   []shared.ID
}

func (s *deletedTemplateStore) Deleted(
	_ context.Context, after shared.ID, batch int,
) ([]repository.DeletedTemplate, error) {
	s.pages++
	var page []repository.DeletedTemplate
	for _, template := range s.templates {
		if string(template.ID) > string(after) && len(page) < batch {
			page = append(page, template)
		}
	}
	return page, nil
}

func (s *deletedTemplateStore) Remove(_ context.Context, ids []shared.ID) ([]shared.ID, error) {
	s.removed = append(s.removed, ids...)
	s.templates = slices.DeleteFunc(s.templates, func(template repository.DeletedTemplate) bool {
		return slices.Contains(ids, template.ID)
	})
	return ids, nil
}

var (
	templateA = shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	templateB = shared.MustParseID("0192f000-0000-7000-8000-0000000000d2")
	templateC = shared.MustParseID("0192f000-0000-7000-8000-0000000000d3")
	otherHub  = shared.MustParseID("0192f000-0000-7000-8000-00000000001b")
)

func deletedTemplateFixture() []repository.DeletedTemplate {
	return []repository.DeletedTemplate{
		{ID: templateA, ScopeID: collectionID, HubID: hubID},
		{ID: templateB, ScopeID: hubID},
		{ID: templateC, ScopeID: otherHub},
	}
}

// The pass removes the deleted templates no hold covers any more, journals each with RETENTION,
// and counts a held one as blocked, never as matched (data-retention.md §4).
func TestThePassRemovesTheTemplatesNoHoldCoversAnyMore(t *testing.T) {
	cases := []struct {
		name    string
		holds   domain.Holds
		removed []shared.ID
	}{
		{"no hold any more", nil, []shared.ID{templateA, templateB, templateC}},
		{"a hold on the hub", domain.Holds{{ID: holdID, Scope: domain.HoldContainer, ScopeID: hubID}},
			[]shared.ID{templateC}},
		{"a hold on the collection", domain.Holds{{ID: holdID, Scope: domain.HoldContainer, ScopeID: collectionID}},
			[]shared.ID{templateB, templateC}},
		{"an account hold", domain.Holds{{ID: holdID, Scope: domain.HoldAccount, ScopeID: accountID}},
			[]shared.ID{templateA, templateB, templateC}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newRunHarness()
			h.holds.holds = c.holds
			store := &deletedTemplateStore{templates: deletedTemplateFixture()}
			h.run.DeletedTemplates = store

			outcome, err := h.run.Execute(t.Context(), actor())
			if err != nil {
				t.Fatalf("the run failed: %v", err)
			}
			if !slices.Equal(store.removed, c.removed) {
				t.Errorf("removed %v, want %v", store.removed, c.removed)
			}
			var journalled []shared.ID
			for _, removal := range h.removals.recorded {
				if removal.Entity == "template" {
					if removal.Reason != domain.DeletedByRetention {
						t.Errorf("journalled %+v, want the reason RETENTION", removal)
					}
					journalled = append(journalled, removal.EntityID)
				}
			}
			if !slices.Equal(journalled, c.removed) {
				t.Errorf("journalled %v, want %v", journalled, c.removed)
			}
			held := 3 - len(c.removed)
			if outcome.Blocked[domain.BlockedByLegalHold] != held {
				t.Errorf("blocked %v, want %d held templates", outcome.Blocked, held)
			}
			if outcome.Matched != len(c.removed) || outcome.Removed != len(c.removed) {
				t.Errorf("matched %d, removed %d, want %d", outcome.Matched, outcome.Removed, len(c.removed))
			}
		})
	}
}

// Every page in one pass: a full page of held templates must not hide a released one behind it.
func TestThePassLooksPastAPageOfHeldTemplates(t *testing.T) {
	h := newRunHarness()
	h.purger.BatchSize = 2
	h.run.Purger = h.purger
	h.holds.holds = domain.Holds{{ID: holdID, Scope: domain.HoldContainer, ScopeID: hubID}}
	store := &deletedTemplateStore{templates: deletedTemplateFixture()}
	h.run.DeletedTemplates = store

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if !slices.Equal(store.removed, []shared.ID{templateC}) {
		t.Errorf("removed %v, want the released template on the second page", store.removed)
	}
}

// A workspace-wide hold covers every template; the pass does not even read them.
func TestATenantWideHoldKeepsEveryDeletedTemplate(t *testing.T) {
	h := newRunHarness()
	h.holds.holds = domain.Holds{{ID: holdID, Scope: domain.HoldTenant}}
	store := &deletedTemplateStore{templates: deletedTemplateFixture()}
	h.run.DeletedTemplates = store

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if store.pages != 0 || len(store.removed) != 0 {
		t.Errorf("read %d pages and removed %v under a workspace hold", store.pages, store.removed)
	}
}
