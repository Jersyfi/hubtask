// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
)

// UC-CNV-06 check 1: the saves of one drawing session fold into one version, whoever saved; a
// pause, or a name on the last version, starts the next.
func TestSavesOfOneSessionFoldIntoOneVersion(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	anna, bert := shared.ID("0190a000-0000-7000-8000-0000000000a1"), shared.ID("0190a000-0000-7000-8000-0000000000b1")
	board := whiteboard.Board{ID: boardID, Title: "Retro", Scene: whiteboard.Scene{Elements: []whiteboard.Element{sticky("a")}}}
	first, fold := whiteboard.Snapshot(nil, board, anna, now)
	if fold || first.Seq != 1 || first.AuthorID != anna {
		t.Fatalf("the first version: %+v, fold %v", first, fold)
	}
	named := first
	named.Name = "Before"
	cases := []struct {
		name    string
		last    whiteboard.Version
		author  shared.ID
		at      time.Time
		fold    bool
		wantSeq int
	}{
		{"another person in the same session", first, bert, now.Add(5 * time.Minute), true, 1},
		{"after a rest", first, anna, now.Add(whiteboard.SessionGap), false, 2},
		{"after a named version", named, anna, now.Add(time.Minute), false, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			last := tc.last
			got, fold := whiteboard.Snapshot(&last, board, tc.author, tc.at)
			if fold != tc.fold || got.Seq != tc.wantSeq || got.AuthorID != tc.author {
				t.Fatalf("got %+v, fold %v", got, fold)
			}
		})
	}
	if _, err := whiteboard.CheckVersionName(strings.Repeat("x", 121), nil); detail(err) != "whiteboards.version_name_malformed" {
		t.Fatalf("a long name: %v", err)
	}
	if name, err := whiteboard.CheckVersionName("  Before the workshop ", nil); err != nil || name != "Before the workshop" {
		t.Fatalf("name %q, err %v", name, err)
	}
}
