// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// AssigneeRestrictedCode is the answer to a rule that names a person whose processing is
// restricted: no rule puts them on an entry (UC-PRV-04 check 4, data-protection.md §4). The engine
// records the action as SKIPPED with it, never as failed.
const AssigneeRestrictedCode = "items.assignee_restricted"

// RestrictedAccounts answers which of the accounts named may not be processed automatically - the
// question automatic assignment already asks of its pool.
type RestrictedAccounts interface {
	Restricted(ctx context.Context, accountIDs []shared.ID) (map[shared.ID]bool, error)
}

// restrictedUnderRule reports whether a rule is about to put a restricted person on an entry. By
// hand nothing is restricted: Art. 18 stops automatic processing, not people deciding.
func restrictedUnderRule(
	ctx context.Context, accounts RestrictedAccounts, actor appshared.ActorContext, accountID shared.ID,
) (bool, error) {
	if accounts == nil || actor.RunsForRule.IsZero() || accountID.IsZero() {
		return false, nil
	}
	restricted, err := accounts.Restricted(ctx, []shared.ID{accountID})
	return restricted[accountID], err
}

func assigneeRestricted(accountID shared.ID) error {
	return shared.ErrValidation.WithDetail(AssigneeRestrictedCode).
		WithParams(map[string]string{"account_id": accountID.String()})
}
