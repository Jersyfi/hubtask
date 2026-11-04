// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
)

const otherGroup = "0190a8f0-0000-7000-8000-0000000000de"

// UC-CFG-01 check 11: a role of the document names the groups holding it at the workspace; an
// apply grants it to each group added and revokes it from each dropped, the plan flagged as a
// change of what somebody may do, and the export reads the groups back.
func TestARolesGroupsAreGrantedAndRevokedByTheDocument(t *testing.T) {
	f := newFixture()
	apply := func(groups ...any) {
		t.Helper()
		document := map[string]any{
			"roles":       map[string]any{"cab_member": map[string]any{"name": "CAB member", "permissions": []any{"READ"}}},
			"role_groups": map[string]any{"cab_member": map[string]any{"groups": groups}},
		}
		plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})
		if plan["flags"].(usecase.Output)["changes_permissions"] != true {
			t.Fatalf("a grant to a group is not flagged: %v", plan["flags"])
		}
		invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
			"document": document, "plan_hash": plan["plan_hash"], "acknowledge": true,
		})
	}
	held := func() []string {
		var groups []string
		for _, raw := range f.world.grants {
			grant := raw.(map[string]any)
			if grant["role"] == "cab_member" && grant["scope_type"] == "TENANT" {
				groups = append(groups, grant["group_id"].(string))
			}
		}
		return groups
	}

	apply(knownGroup)
	if got := held(); len(got) != 1 || got[0] != knownGroup {
		t.Fatalf("the role is held by %v", got)
	}
	exported := invoke(t, ExportConfig{f.desk}.Descriptor(), person, nil)["document"].(map[string]any)
	block, _ := exported["role_groups"].(map[string]any)["cab_member"].(map[string]any)
	if groups, _ := block["groups"].([]any); len(groups) != 1 || groups[0] != knownGroup {
		t.Errorf("the export reads %v", exported["role_groups"])
	}

	apply(otherGroup)
	if got := held(); len(got) != 1 || got[0] != otherGroup {
		t.Errorf("after the change the role is held by %v", got)
	}
}

// A role a template adds is held by nobody yet, so the plan needs no acknowledgement for it
// (user-experience.md §3.2, "an add-only plan needs no acknowledgement"); changing a role's
// permissions, or giving a role to a group, still changes what people may do.
func TestANewRoleChangesNobodysPermissions(t *testing.T) {
	desk := Desk{Catalogue: &scoped{}}
	empty := current{Document: domain.Document{}, Rows: map[string]map[string]map[string]any{}}
	added := desk.planOf(empty, domain.Document{sectionRoles: {
		"cab_member": {"name": "CAB member", "permissions": []any{"READ"}},
	}}, domain.ModeMerge)
	if len(added.Refusals) > 0 || len(added.Ops) != 1 || added.Flags.ChangesPermissions {
		t.Fatalf("adding a role is planned as %+v", added)
	}

	held := current{
		Document: domain.Document{sectionRoles: {"cab_member": {"name": "CAB member", "permissions": []any{"READ"}}}},
		Rows:     map[string]map[string]map[string]any{sectionRoles: {"cab_member": {"key": "cab_member"}}},
	}
	changed := desk.planOf(held, domain.Document{sectionRoles: {
		"cab_member": {"name": "CAB member", "permissions": []any{"READ", "WRITE"}},
	}}, domain.ModeMerge)
	if !changed.Flags.ChangesPermissions {
		t.Fatalf("changing a role's permissions is planned as %+v", changed)
	}
}
