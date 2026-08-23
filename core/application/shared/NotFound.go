// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import (
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ItemNotFound is the one answer for an entry the actor cannot reach, whatever the reason.
//
// It lives here rather than in the package that reads entries because two layers have to give
// exactly the same answer, down to the message code and the parameters. A repository says it when
// the row is not in the tenant; the authorisation service says it when nothing on the entry's path
// grants the actor anything (T-04). If the two differed by a single character, the difference would
// be an oracle for which identifiers exist - which is precisely the disclosure the not-found answer
// is there to prevent (multi-tenancy.md §2).
func ItemNotFound(id shared.ID) error {
	return shared.ErrNotFound.
		WithDetail("items.not_found").
		WithParams(map[string]string{"item_id": id.String()})
}
