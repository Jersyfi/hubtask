// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// ReplacedHoldInput is one hold as it stood before a point-in-time recovery rewound it away
// (backup-restore.md §8.5 step 5).
type ReplacedHoldInput struct {
	ID       shared.ID
	Scope    HoldScope
	ScopeID  shared.ID
	Reason   string
	PlacedBy shared.ID
	PlacedAt time.Time
	Text     text.Normalizer
}

// ReplacedLegalHold builds a hold placed again after a point-in-time recovery: the same hold, with
// its own identifier, placer and moment, rather than a new one placed by the operator - an auditor
// reading the holds afterwards reads the owner's decision, not the recovery's.
//
// It is in force whatever happened to it in the rewound period: a hold released there stays in
// force until its owner releases it again (data-protection.md §5), so a release
// is never carried over. The checks are a new hold's - a record a trail or an archive gave back is
// no more trusted than a request - and a hold without its moment is refused rather than given the
// recovery's: the moment is part of what an auditor reads.
func ReplacedLegalHold(in ReplacedHoldInput) (LegalHold, error) {
	if in.PlacedAt.IsZero() {
		return LegalHold{}, invalidHold(CodeHoldIncomplete, "/placed_at")
	}
	return NewLegalHold(NewHoldInput{
		ID: in.ID, Scope: in.Scope, ScopeID: in.ScopeID, Reason: in.Reason,
		PlacedBy: in.PlacedBy, Now: in.PlacedAt, Text: in.Text,
	})
}
