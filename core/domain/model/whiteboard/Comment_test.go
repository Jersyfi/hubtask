// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
)

var (
	commenter = shared.ID("0190a000-0000-7000-8000-0000000000c1")
	mentioned = shared.ID("0190a000-0000-7000-8000-0000000000d1")
)

// UC-CNV-09 check 1: a thread is pinned to an object or a place; a reply stands in its thread.
func TestACommentIsPinnedToAnObjectOrAPlace(t *testing.T) {
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	base := whiteboard.NewCommentInput{
		ID: "0190a000-0000-7000-8000-0000000000e1", TenantID: entryID, BoardID: boardID, AuthorID: commenter,
		Body: "Why here?", Now: now,
	}
	thread, err := whiteboard.NewComment(withPin(base, "note-1", 120, -40))
	if err != nil || thread.Element != "note-1" || thread.X != 120 || thread.Y != -40 {
		t.Fatalf("a pinned thread: %+v, %v", thread, err)
	}
	reply := base
	reply.ID, reply.Parent = "0190a000-0000-7000-8000-0000000000e2", &thread
	if got, err := whiteboard.NewComment(reply); err != nil || got.ParentID != thread.ID || got.Element != "" {
		t.Fatalf("a reply: %+v, %v", got, err)
	}
	nested := reply
	child, _ := whiteboard.NewComment(reply)
	nested.Parent = &child
	other := thread
	other.BoardID = entryID
	gone := thread
	gone.DeletedAt = &now
	cases := []struct {
		name string
		in   whiteboard.NewCommentInput
		code string
	}{
		{"no words", withBody(base, "  "), "whiteboards.comment_empty"},
		{"too many words", withBody(base, strings.Repeat("x", whiteboard.MaxCommentBytes+1)), "whiteboards.comment_too_long"},
		{"a malformed object", withPin(base, "not an id", 0, 0), "whiteboards.comment_pin_malformed"},
		{"off the board", withPin(base, "", whiteboard.MaxCoordinate+1, 0), "whiteboards.comment_pin_malformed"},
		{"a reply of a reply", nested, "whiteboards.comment_reply_nested"},
		{"a reply with a pin", withPin(reply, "note-1", 0, 0), "whiteboards.comment_reply_pinned"},
		{"a reply on another board", withParent(base, &other), "whiteboards.comment_not_found"},
		{"a reply to a comment taken back", withParent(base, &gone), "whiteboards.comment_not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := whiteboard.NewComment(tc.in)
			if shared.AsError(err).DetailCode != tc.code {
				t.Fatalf("want %s, got %v", tc.code, err)
			}
		})
	}
	if _, err := child.Resolved(true, commenter, now); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a reply resolved: %v", err)
	}
	resolved, err := thread.Resolved(true, commenter, now)
	if err != nil || resolved.ResolvedAt == nil || resolved.ResolvedBy != commenter {
		t.Fatalf("a thread resolved: %+v, %v", resolved, err)
	}
	if reopened, _ := resolved.Resolved(false, commenter, now); reopened.ResolvedAt != nil || !reopened.ResolvedBy.IsZero() {
		t.Fatalf("a thread reopened: %+v", reopened)
	}
}

// UC-CNV-09 check 2: the people a comment names are read from its words once each, and a rewrite
// tells only the ones it adds.
func TestTheMentionsOfAComment(t *testing.T) {
	link := func(id shared.ID) string { return "[@P](hubtask:person/" + id.String() + ")" }
	cases := []struct {
		name string
		body string
		want []shared.ID
	}{
		{"none", "Just words", nil},
		{"one, twice", link(mentioned) + " and " + link(mentioned), []shared.ID{mentioned}},
		{"two in order", link(commenter) + " " + link(mentioned), []shared.ID{commenter, mentioned}},
		{"an upper-case address", "[@P](hubtask:person/" + strings.ToUpper(mentioned.String()) + ")", []shared.ID{mentioned}},
		{"a malformed one is text", "[@P](hubtask:person/nobody)", nil},
		{"another kind is no person", "[x](hubtask:entry/" + mentioned.String() + ")", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := whiteboard.Mentions(tc.body)
			if len(got) != len(tc.want) {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("want %v, got %v", tc.want, got)
				}
			}
		})
	}
	many := ""
	for i := 0; i < whiteboard.MaxMentions+5; i++ {
		many += link(shared.ID(fmt.Sprintf("0190a000-0000-7000-8000-%012d", i)))
	}
	if got := whiteboard.Mentions(many); len(got) != whiteboard.MaxMentions {
		t.Fatalf("at most %d people: %d", whiteboard.MaxMentions, len(got))
	}
	if got := whiteboard.NewlyMentioned(link(commenter), link(commenter)+link(mentioned)); len(got) != 1 || got[0] != mentioned {
		t.Fatalf("newly mentioned: %v", got)
	}
}

func withPin(in whiteboard.NewCommentInput, element string, x, y float64) whiteboard.NewCommentInput {
	in.Element, in.X, in.Y = element, x, y
	return in
}

func withBody(in whiteboard.NewCommentInput, body string) whiteboard.NewCommentInput {
	in.Body = body
	return in
}

func withParent(in whiteboard.NewCommentInput, parent *whiteboard.Comment) whiteboard.NewCommentInput {
	in.Parent = parent
	return in
}
