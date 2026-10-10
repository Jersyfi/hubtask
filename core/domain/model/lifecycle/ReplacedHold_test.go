// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

func replacedInput() lifecycle.ReplacedHoldInput {
	return lifecycle.ReplacedHoldInput{
		ID: holdID, Scope: lifecycle.HoldContainer, ScopeID: hubID,
		Reason: "Dispute with the supplier", PlacedBy: accountID, PlacedAt: placedAt,
		Text: text.Composing{},
	}
}

// A hold placed again keeps what it was and is in force.
func TestAReplacedHoldIsTheSameHoldInForce(t *testing.T) {
	hold, err := lifecycle.ReplacedLegalHold(replacedInput())
	if err != nil {
		t.Fatalf("a whole record was refused: %v", err)
	}
	if hold.ID != holdID || hold.Scope != lifecycle.HoldContainer || hold.ScopeID != hubID ||
		hold.PlacedBy != accountID || !hold.PlacedAt.Equal(placedAt) ||
		hold.Reason != "Dispute with the supplier" || hold.Released() {
		t.Errorf("placed again as %+v", hold)
	}
}

// A record is refused where a new hold would be, and where it lacks its moment.
func TestAReplacedHoldIsRefusedWhereANewOneWouldBe(t *testing.T) {
	cases := []struct {
		name   string
		change func(*lifecycle.ReplacedHoldInput)
		code   string
	}{
		{"no moment", func(in *lifecycle.ReplacedHoldInput) { in.PlacedAt = time.Time{} }, lifecycle.CodeHoldIncomplete},
		{"no identifier", func(in *lifecycle.ReplacedHoldInput) { in.ID = "" }, lifecycle.CodeHoldIncomplete},
		{"no placer", func(in *lifecycle.ReplacedHoldInput) { in.PlacedBy = "" }, lifecycle.CodeHoldIncomplete},
		{"no reason", func(in *lifecycle.ReplacedHoldInput) { in.Reason = "  " }, lifecycle.CodeHoldReasonRequired},
		{"an unknown scope", func(in *lifecycle.ReplacedHoldInput) { in.Scope = "PLANET" }, lifecycle.CodeHoldScopeInvalid},
		{"a workspace hold naming something", func(in *lifecycle.ReplacedHoldInput) {
			in.Scope = lifecycle.HoldTenant
		}, lifecycle.CodeHoldScopeIDMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := replacedInput()
			c.change(&in)
			_, err := lifecycle.ReplacedLegalHold(in)
			if got := shared.AsError(err).DetailCode; got != c.code {
				t.Errorf("refused as %q, want %q", got, c.code)
			}
		})
	}
}
