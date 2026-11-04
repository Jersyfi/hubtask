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
