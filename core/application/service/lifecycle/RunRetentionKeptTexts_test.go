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
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// keptStore is KeptCommentTexts as an in-memory list, paged by identifier as the query is.
type keptStore struct {
	texts   []repository.KeptCommentText
	pages   int
	cleared []shared.ID
}

func (s *keptStore) Kept(_ context.Context, after shared.ID, batch int) ([]repository.KeptCommentText, error) {
	s.pages++
	var page []repository.KeptCommentText
	for _, text := range s.texts {
		if string(text.ID) > string(after) && len(page) < batch {
			page = append(page, text)
		}
	}
	return page, nil
}

func (s *keptStore) Clear(_ context.Context, ids []shared.ID) (int, error) {
	s.cleared = append(s.cleared, ids...)
	s.texts = slices.DeleteFunc(s.texts, func(text repository.KeptCommentText) bool {
		return slices.Contains(ids, text.ID)
	})
	return len(ids), nil
}

func keptOn(id string, item shared.ID) repository.KeptCommentText {
	return repository.KeptCommentText{
		ID: shared.MustParseID(id), ItemID: item, Path: work.RootPath(item),
		CollectionID: collectionID, HubID: hubID,
	}
}

var (
	keptA = "0192f000-0000-7000-8000-0000000000c1"
	keptB = "0192f000-0000-7000-8000-0000000000c2"
	keptC = "0192f000-0000-7000-8000-0000000000c3"
)

// UC-LIF-06 check 9, the pass's half: the text of a comment no hold covers any more is cleared, the
// text a hold still covers stays - by the entry, the hub, or an account that contributed to the
// entry - and a held text is blocked, never matched, so the job does not come straight back.
func TestThePassClearsTheTextNoHoldCoversAnyMore(t *testing.T) {
	someone := shared.MustParseID("0192f000-0000-7000-8000-0000000000e9")

	cases := []struct {
		name         string
		holds        domain.Holds
		contributors map[shared.ID][]shared.ID
		cleared      []string
	}{
		{"no hold any more", nil, nil, []string{keptA, keptB, keptC}},
		{"a hold on one entry", domain.Holds{{ID: holdID, Scope: domain.HoldItem, ScopeID: taskID}},
			nil, []string{keptC}},
		{"a hold on the hub", domain.Holds{{ID: holdID, Scope: domain.HoldContainer, ScopeID: hubID}},
			nil, nil},
		{"a hold on a contributor to one entry",
			domain.Holds{{ID: holdID, Scope: domain.HoldAccount, ScopeID: someone}},
			map[shared.ID][]shared.ID{otherTaskID: {someone}}, []string{keptA, keptB}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newRunHarness()
			h.holds.holds = c.holds
			h.holds.contributors = c.contributors
			store := &keptStore{texts: []repository.KeptCommentText{
				keptOn(keptA, taskID), keptOn(keptB, taskID), keptOn(keptC, otherTaskID),
			}}
			h.run.KeptTexts = store

			outcome, err := h.run.Execute(t.Context(), actor())
			if err != nil {
				t.Fatalf("the run failed: %v", err)
			}

			want := make([]shared.ID, 0, len(c.cleared))
			for _, id := range c.cleared {
				want = append(want, shared.MustParseID(id))
			}
			if !slices.Equal(store.cleared, want) {
				t.Errorf("cleared %v, want %v", store.cleared, want)
			}
			kept := 3 - len(c.cleared)
			if outcome.Blocked[domain.BlockedByLegalHold] != kept {
				t.Errorf("blocked %v, want %d held texts", outcome.Blocked, kept)
			}
			if outcome.Matched != len(c.cleared) || outcome.Removed != len(c.cleared) {
				t.Errorf("matched %d, removed %d, want %d cleared", outcome.Matched, outcome.Removed, len(c.cleared))
			}
		})
	}
}

// Every page in one pass: a full page of texts still held must not hide a released one behind it.
func TestThePassLooksPastAPageOfHeldTexts(t *testing.T) {
	h := newRunHarness()
	h.purger.BatchSize = 2
	h.run.Purger = h.purger
	h.holds.holds = domain.Holds{{ID: holdID, Scope: domain.HoldItem, ScopeID: taskID}}
	store := &keptStore{texts: []repository.KeptCommentText{
		keptOn(keptA, taskID), keptOn(keptB, taskID), keptOn(keptC, otherTaskID),
	}}
	h.run.KeptTexts = store

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if !slices.Equal(store.cleared, []shared.ID{shared.MustParseID(keptC)}) {
		t.Errorf("cleared %v, want the released text on the second page", store.cleared)
	}
}

// A workspace-wide hold covers every text; the pass does not even read them.
func TestATenantWideHoldKeepsEveryText(t *testing.T) {
	h := newRunHarness()
	h.holds.holds = domain.Holds{{ID: holdID, Scope: domain.HoldTenant}}
	store := &keptStore{texts: []repository.KeptCommentText{keptOn(keptA, taskID)}}
	h.run.KeptTexts = store

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if store.pages != 0 || len(store.cleared) != 0 {
		t.Errorf("read %d pages and cleared %v under a workspace hold", store.pages, store.cleared)
	}
}
