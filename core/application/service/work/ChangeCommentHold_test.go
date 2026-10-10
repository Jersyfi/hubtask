// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// legalHolds is the hold reading as the deletion sees it. It counts the reads, because an edit
// must not ask at all.
type legalHolds struct {
	active       lifecycle.Holds
	contributors map[shared.ID][]shared.ID
	asked        int
}

func (l *legalHolds) Active(context.Context) (lifecycle.Holds, error) {
	l.asked++
	return l.active, nil
}

func (l *legalHolds) Contributors(context.Context, []shared.ID) (map[shared.ID][]shared.ID, error) {
	return l.contributors, nil
}

func hold(scope lifecycle.HoldScope, id shared.ID) lifecycle.LegalHold {
	return lifecycle.LegalHold{
		ID: "0192f000-0000-7000-8000-0000000000e1", Scope: scope, ScopeID: id,
		Reason: "dispute", PlacedBy: accountID, PlacedAt: now,
	}
}

// UC-LIF-06 check 9: deleted under a hold, a comment keeps its text in the store and reads as
// deleted all the same; a hold elsewhere keeps nothing. The comment is judged by its entry, so an
// account hold on anybody who contributed to the entry keeps it too - another author's comment
// included, the reach a purge of the entry has.
func TestADeletionUnderAHoldKeepsTheText(t *testing.T) {
	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000000ee")
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000e9")

	cases := []struct {
		name         string
		holds        lifecycle.Holds
		contributors []shared.ID
		kept         bool
	}{
		{"no hold", nil, nil, false},
		{"the workspace", lifecycle.Holds{hold(lifecycle.HoldTenant, "")}, nil, true},
		{"the hub", lifecycle.Holds{hold(lifecycle.HoldContainer, hubID)}, nil, true},
		{"the collection", lifecycle.Holds{hold(lifecycle.HoldContainer, collectionID)}, nil, true},
		{"the entry", lifecycle.Holds{hold(lifecycle.HoldItem, assignedItem)}, nil, true},
		{"the author", lifecycle.Holds{hold(lifecycle.HoldAccount, accountID)}, []shared.ID{accountID}, true},
		{"another contributor to the entry", lifecycle.Holds{hold(lifecycle.HoldAccount, other)},
			[]shared.ID{accountID, other}, true},
		{"another hub", lifecycle.Holds{hold(lifecycle.HoldContainer, elsewhere)}, nil, false},
		{"another entry", lifecycle.Holds{hold(lifecycle.HoldItem, elsewhere)}, nil, false},
		{"a person who did not contribute", lifecycle.Holds{hold(lifecycle.HoldAccount, other)},
			[]shared.ID{accountID}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newCommentHarness(t)
			h.withItem(domain.ItemTask)
			h.withModeration("", false)
			h.writer.Holds = &legalHolds{
				active: c.holds, contributors: map[shared.ID][]shared.ID{assignedItem: c.contributors},
			}
			comment := h.withComment("0192f000-0000-7000-8000-0000000000b1", accountID, "We pay anyway")

			deleted, err := DeleteComment{Writer: h.writer}.Execute(
				t.Context(), actor(), editCmd(comment.ID, ""))
			if err != nil {
				t.Fatalf("deleting failed: %v", err)
			}
			if len(h.comments.tombstones) != 1 || h.comments.tombstones[0].keptText != c.kept {
				t.Fatalf("tombstones %+v, want the text kept = %v", h.comments.tombstones, c.kept)
			}
			// Deleted for everybody either way: no text in the answer or the event.
			if deleted.Body != "" || deleted.DeletedAt == nil {
				t.Errorf("answered %+v, want the tombstone", deleted)
			}
			if body := h.events.appended[0].Payload["body"]; body != nil {
				t.Errorf("the deletion event carries the text: %v", body)
			}
		})
	}
}

// A hold does not freeze editing (check 6), and an edit does not even ask.
func TestAnEditDoesNotAskForHolds(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	h.withModeration("", false)
	holds := &legalHolds{active: lifecycle.Holds{hold(lifecycle.HoldTenant, "")}}
	h.writer.Holds = holds
	comment := h.withComment("0192f000-0000-7000-8000-0000000000b1", accountID, "Frist")

	if _, err := (EditComment{Writer: h.writer}).Execute(
		t.Context(), actor(), editCmd(comment.ID, "First")); err != nil {
		t.Fatalf("an edit under a hold was refused: %v", err)
	}
	if holds.asked != 0 {
		t.Errorf("the edit read the holds %d times", holds.asked)
	}
}
