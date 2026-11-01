// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
)

// RulesNaming counts the switched-on rules that name a block by its key, for the deletion that
// would leave them without it: a business calendar a RELATIVE_DATE trigger counts on, a custom
// role a NOTIFY_ROLE step addresses. A deletion refused with the count is the rule's owner hearing
// of it before the rule stops firing, rather than after (configuration.md §2).
//
// It runs inside the deleting transaction and judges nothing; the refusal is the caller's.
type RulesNaming struct {
	Rules repository.Rules
}

// Calendar is how many switched-on rules' triggers count working days on the calendar.
func (n RulesNaming) Calendar(ctx context.Context, key string) (int, error) {
	return n.count(ctx, func(rule domain.Rule) bool { return rule.Trigger.Calendar == key })
}

// Role is how many switched-on rules have a NOTIFY_ROLE step, in a branch or not, addressing the
// role.
func (n RulesNaming) Role(ctx context.Context, key string) (int, error) {
	return n.count(ctx, func(rule domain.Rule) bool { return namesRole(rule.Actions, key, 0) })
}

func (n RulesNaming) count(ctx context.Context, names func(domain.Rule) bool) (int, error) {
	if n.Rules == nil {
		return 0, nil
	}
	enabled := true
	count, cursor := 0, ""
	for {
		page, err := n.Rules.List(ctx, repository.Query{Enabled: &enabled, Cursor: cursor, Size: maxRulePage})
		if err != nil {
			return 0, err
		}
		for _, rule := range page.Rules {
			if names(rule) {
				count++
			}
		}
		if !page.HasMore {
			return count, nil
		}
		cursor = page.NextCursor
	}
}

func namesRole(actions []domain.Action, key string, depth int) bool {
	for _, action := range actions {
		if action.Kind == domain.ActionBranch {
			branch, err := domain.ReadBranch(action.Params, "", depth)
			if err == nil && (namesRole(branch.Then, key, depth+1) || namesRole(branch.Else, key, depth+1)) {
				return true
			}
			continue
		}
		if role, _ := action.Params["role"].(string); action.Kind == "NOTIFY_ROLE" && role == key {
			return true
		}
	}
	return false
}
