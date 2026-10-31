// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The SLA clocks and the requester's mail read `internal` from the event: a note the requester
// never reads must say so, or it meets the first response and is mailed (UC-SVC-04 check 3).
func TestACommentEventSaysWhetherItIsAnInternalNote(t *testing.T) {
	for _, internal := range []bool{true, false} {
		comment := work.Comment{
			ID: eventHub, ItemID: eventOtherHub, AuthorID: eventHub, Body: "x",
			CreatedAt: occurred, Version: 1, Internal: internal,
		}
		payload := CommentPayload(comment, eventHub)
		if got, ok := payload["internal"].(bool); !ok || got != internal {
			t.Errorf("internal %v: the payload carries %#v", internal, payload["internal"])
		}
	}
}
