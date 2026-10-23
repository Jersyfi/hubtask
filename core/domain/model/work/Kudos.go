// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Message codes of a refused kudos (rule 8).
const (
	CodeKudosNotCompleted  = "kudos.not_completed"
	CodeKudosOwnCompletion = "kudos.own_completion"
)

// Kudos is one person's acknowledgement of another's completion of an entry (UC-WRK-29 check 5,
// design-system.md §7).
//
// It belongs to the completion, not to the entry: GivenTo is whoever completed it when the kudos
// were given, so an entry reopened and finished by somebody else does not hand them kudos meant for
// the first. Whom it was given to is the only person who learns who gave it; no figure counts kudos
// per person (check 6).
type Kudos struct {
	ItemID  shared.ID
	GivenBy shared.ID
	GivenTo shared.ID
	GivenAt time.Time
}

// KudosTo answers whom kudos on the entry go to: the person who completed it.
//
// Refused on an open entry, and on one the giver completed themselves - acknowledging oneself is
// the score-keeping §7 excludes. An entry completed before completions named their author has
// nobody to acknowledge and is refused as open is.
func KudosTo(item WorkItem, giver shared.ID) (shared.ID, error) {
	if !item.Completion.IsCompleted || item.Completion.CompletedBy.IsZero() {
		return "", shared.ErrConflict.WithDetail(CodeKudosNotCompleted)
	}
	if item.Completion.CompletedBy == giver {
		return "", shared.ErrValidation.WithDetail(CodeKudosOwnCompletion)
	}
	return item.Completion.CompletedBy, nil
}

// KudosView is what one reader may know of an entry's kudos: whether they gave kudos to its
// current completion, and the kudos they received on it. Never another person's.
type KudosView struct {
	ItemID shared.ID
	Given  bool
	From   []Kudos
}
