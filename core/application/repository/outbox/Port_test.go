// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package outbox_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Before has to say what `(occurred_at, id) > (…, …)` says in the query, including inside one
// microsecond - the two are one order written twice, and a disagreement between them is a poll that
// repeats a row or steps over one.
func TestAPositionSortsByTheMomentThenTheIdentifier(t *testing.T) {
	at := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	low := shared.ID("01936f2a-0000-7000-8000-000000000001")
	high := shared.ID("01936f2a-0000-7000-8000-000000000002")

	cases := []struct {
		name   string
		left   outbox.Position
		right  outbox.Position
		before bool
	}{
		{
			name:  "an earlier moment",
			left:  outbox.Position{OccurredAt: at, ID: high},
			right: outbox.Position{OccurredAt: at.Add(time.Microsecond), ID: low}, before: true,
		},
		{
			name:  "a later moment",
			left:  outbox.Position{OccurredAt: at.Add(time.Microsecond), ID: low},
			right: outbox.Position{OccurredAt: at, ID: high}, before: false,
		},
		{
			name:  "the same moment, a lower identifier",
			left:  outbox.Position{OccurredAt: at, ID: low},
			right: outbox.Position{OccurredAt: at, ID: high}, before: true,
		},
		{
			name:  "the same moment, a higher identifier",
			left:  outbox.Position{OccurredAt: at, ID: high},
			right: outbox.Position{OccurredAt: at, ID: low}, before: false,
		},
		{
			name:  "the same position",
			left:  outbox.Position{OccurredAt: at, ID: low},
			right: outbox.Position{OccurredAt: at, ID: low}, before: false,
		},
		{
			// The start of the window: no identifier at all, which sorts before every row of that
			// microsecond. It is the position a caller with no cursor begins at.
			name:  "the start of a moment",
			left:  outbox.Position{OccurredAt: at},
			right: outbox.Position{OccurredAt: at, ID: low}, before: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.left.Before(tc.right); got != tc.before {
				t.Errorf("Before = %v, want %v", got, tc.before)
			}
		})
	}
}
