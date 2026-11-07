// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
)

// A board's words that hold a zero byte are refused as malformed: the database cannot store one,
// and the write would fail as unavailable - an error a client retries for ever.
func TestWordsHoldingAZeroByteAreRefused(t *testing.T) {
	code := func(err error) string {
		var e *shared.Error
		if errors.As(err, &e) {
			return e.DetailCode
		}
		return ""
	}
	if _, err := whiteboard.CheckTitle("Retro\x00", nil); code(err) != "whiteboards.title_malformed" {
		t.Errorf("a title: %v", err)
	}
	if _, err := whiteboard.CheckVersionName("Before\x00", nil); code(err) != "whiteboards.version_name_malformed" {
		t.Errorf("a version's name: %v", err)
	}
	_, err := whiteboard.NewComment(whiteboard.NewCommentInput{
		ID: "0190a000-0000-7000-8000-0000000000e1", TenantID: entryID, BoardID: boardID, AuthorID: commenter,
		Body: "Why\x00 here?", Now: time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC),
	})
	if code(err) != "whiteboards.comment_malformed" {
		t.Errorf("a comment: %v", err)
	}
}

// UC-CNV-10 check 3: a board stands under a page of its own hub, never under another board.
func TestCheckParent(t *testing.T) {
	hub, other := shared.ID("0190a5f0-0000-7000-8000-0000000000a1"), shared.ID("0190a5f0-0000-7000-8000-0000000000a2")
	cases := []struct {
		name   string
		parent whiteboard.ParentCandidate
		code   string
	}{
		{"a page of the same hub", whiteboard.ParentCandidate{HubID: hub}, ""},
		{"a page of another hub", whiteboard.ParentCandidate{HubID: other}, "whiteboards.parent_other_space"},
		{"a board", whiteboard.ParentCandidate{IsBoard: true, HubID: hub}, "whiteboards.no_children"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := whiteboard.CheckParent(hub, c.parent)
			if c.code == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || shared.AsError(err).DetailCode != c.code {
				t.Fatalf("error %v, want %s", err, c.code)
			}
		})
	}
}
