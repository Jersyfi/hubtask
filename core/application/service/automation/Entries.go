// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"

	"github.com/Jersyfi/hubtask/core/application/condition"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CodeEntriesUnwired fails a run whose rule names *for which entries* in a build that cannot ask:
// acting without the line would widen the rule.
const CodeEntriesUnwired = "automation.entries_unwired"

// listed answers *for which entries* (UC-AUT-01/13): whether the run's entry is among those the rule's
// line lists, asked of POST /items:query's use case as the run_as account reads - so the line
// never reaches an entry that account may not read, and a name resolved at save keeps meaning the
// same identifier. A rule without a line lists every entry; a run about no entry is about none of
// them. failure is a code where the line itself cannot be read any more (a field gone since).
func (h RunRule) listed(
	ctx context.Context, actor appshared.ActorContext, rule domain.Rule, values condition.Values,
) (ok bool, failure string, err error) {
	if rule.Entries == "" {
		return true, "", nil
	}
	entry := values.EntryID()
	if entry.IsZero() {
		return false, "", nil
	}
	if h.EntryScope == nil {
		return false, CodeEntriesUnwired, nil
	}
	as := appshared.ActorContext{
		Kind:        appshared.ActorServiceAccount,
		TenantID:    actor.TenantID,
		AccountID:   rule.RunAs,
		RunsForRule: rule.ID,
		Locale:      actor.Locale,
		TimeZone:    actor.TimeZone,
		Scopes:      []string{itemsReadScope},
	}
	out, err := h.EntryScope.Invoke(ctx, as, usecase.Input{
		"scope_item_id": entry.String(), "include_descendants": false, "q": rule.Entries, "size": 1,
	})
	switch {
	case errors.Is(err, shared.ErrNotFound), errors.Is(err, shared.ErrForbidden):
		// Gone, or out of run_as's reach: not one of the entries it lists.
		return false, "", nil
	case errors.Is(err, shared.ErrValidation):
		return false, codeOf(err), nil
	case err != nil:
		return false, "", err
	}
	data, _ := out["data"].([]usecase.Output)
	return len(data) > 0, "", nil
}

// itemsReadScope is the one scope the line is read under: listing entries, nothing else.
const itemsReadScope = "items:read"
