// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"time"

	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Kept is what one legal hold kept from one erasure (data-protection.md §4.1): a hold wins over an
// erasure as far as it reaches, and the case that kept something closes as partly completed, naming
// how much, under which hold and why.
//
// Counts rather than identifiers, and never a name: the case is read by an administrator and an
// auditor, and what they need is how much stayed and why, not which comment.
type Kept struct {
	HoldID      shared.ID
	HoldScope   lifecycle.HoldScope
	HoldScopeID shared.ID
	// Account is the person's account itself, kept rather than anonymised or deleted - only under a
	// hold on that account or on the whole workspace.
	Account     bool
	Entries     int
	Comments    int
	Assignments int
	Intake      int
	RecordedAt  time.Time
	// ErasedAt is when the rest went, once the hold no longer kept anything; zero while it does.
	ErasedAt time.Time
	// BlockedCode and BlockedParams are why the rest could not be erased yet, empty otherwise.
	BlockedCode   string
	BlockedParams map[string]string
}

// Pending reports a part that is still kept.
func (k Kept) Pending() bool { return k.ErasedAt.IsZero() }

// KeptLegalBasis is why what a hold keeps is not erased: the establishment, exercise or defence of
// legal claims (Art. 17(3)(e)). A value rather than prose; the clients say it in words.
const KeptLegalBasis = "ART_17_3_E"
