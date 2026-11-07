// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"slices"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
)

// sectionRoleGroups holds which groups hold a custom role at the whole workspace: what a
// template's roles part maps to the groups a GROUP question names on applying (configuration.md
// §6), so that somebody holds a role the moment the template is applied. A block is the role's
// key with its groups; a grant to a person, or at a hub or a collection, is not the document's.
const sectionRoleGroups = "role_groups"

const groupsAttribute = "groups"

// roleGroupsSection reads the workspace grants of ListRoles and writes them through GrantRole and
// RevokeRoleGrant; it comes after the roles section, so a role the same apply creates is there.
func roleGroupsSection() section {
	return section{
		name: sectionRoleGroups, list: "ListRoles", key: "role", attributes: []string{groupsAttribute},
		rowsOf: roleGroupRows, ops: roleGroupOps, permissions: true,
	}
}

// roleGroupRows is one row per custom role a group holds at the workspace: the groups in order,
// and the grant behind each, which a revocation names.
func roleGroupRows(answer map[string]any) []any {
	grants, _ := answer["grants"].([]any)
	byRole := map[string]map[string]any{}
	// The grant behind each group, by role; the row carries the same map.
	grantIDs := map[string]map[string]any{}
	var order []string
	for _, raw := range grants {
		grant, _ := raw.(map[string]any)
		role, group := text(grant["role"]), text(grant["group_id"])
		if role == "" || group == "" || text(grant["scope_type"]) != "TENANT" {
			continue
		}
		if _, seen := byRole[role]; !seen {
			grantIDs[role] = map[string]any{}
			byRole[role] = map[string]any{"role": role, "grant_ids": grantIDs[role]}
			order = append(order, role)
		}
		grantIDs[role][group] = text(grant["id"])
	}
	slices.Sort(order)
	rows := make([]any, 0, len(order))
	for _, role := range order {
		row := byRole[role]
		held := make([]string, 0, len(grantIDs[role]))
		for group := range grantIDs[role] {
			held = append(held, group)
		}
		slices.Sort(held)
		groups := make([]any, 0, len(held))
		for _, group := range held {
			groups = append(groups, group)
		}
		row[groupsAttribute] = groups
		rows = append(rows, row)
	}
	return rows
}

// roleGroupOps grants the role to each group the block adds and revokes it from each it drops; a
// removed block revokes every one. The order of the list is not a change.
func roleGroupOps(change domain.BlockChange, row map[string]any) ([]operation, error) {
	path := change.Path()
	before := names(change.Before[groupsAttribute])
	var after []string
	if change.Op != domain.OpRemove {
		after = names(change.After[groupsAttribute])
	}
	grantIDs, _ := row["grant_ids"].(map[string]any)
	var ops []operation
	for _, group := range after {
		if !slices.Contains(before, group) {
			ops = append(ops, operation{UseCase: "GrantRole", Path: path, Input: usecase.Input{
				"role": change.Key, "scope_type": "TENANT", "group_id": group,
			}})
		}
	}
	for _, group := range before {
		if slices.Contains(after, group) {
			continue
		}
		if id := text(grantIDs[group]); id != "" {
			ops = append(ops, operation{UseCase: "RevokeRoleGrant", Path: path, Input: usecase.Input{"grant_id": id}})
		}
	}
	return ops, nil
}
