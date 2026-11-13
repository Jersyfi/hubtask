// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// UC-SVC-04: an internal note leaves the API marked as one, and a public reply as none, so a
// client never draws a note as a reply.
func TestACommentCarriesItsInternalFlag(t *testing.T) {
	for _, internal := range []bool{true, false} {
		comment := commentResponse(usecase.Output{
			"id": "0192f000-0000-7000-8000-000000000c01", "item_id": "0192f000-0000-7000-8000-000000000c02",
			"author_id": "0192f000-0000-7000-8000-000000000c03", "body": "text", "kind": "USER", "internal": internal,
		})
		if comment.Internal == nil || *comment.Internal != internal {
			t.Errorf("internal %v answers %v", internal, comment.Internal)
		}
	}
}
