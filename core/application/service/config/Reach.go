// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"slices"
	"strconv"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
)

// reach is what one change touches beyond the configuration (config-document.md §2): the
// entries a workflow's new shape moves, the people a role's change gains or loses a permission
// for. Counts and identifiers only - never an entry's content (rule 10).
type reach struct {
	Path string
	// Entries, Moves and Removed are a workflow's publish preview for the shape the plan sets.
	Entries int
	Moves   []any
	Removed []any
	// Accounts and Groups hold the role directly or as a group; Gained and Lost are the
	// permissions the change gives and takes.
	Accounts, Groups int
	Gained, Lost     []string
}

const (
	previewWorkflowPublish = "PreviewWorkflowPublish"
	listRoles              = "ListRoles"
)

// reachOf asks the use cases that know what a change touches, in the plan's read: a workflow's
// publish preview for the shape the plan sets, the role grants for a changed role. A shape that
// removes a status entries are in is refused here, as its publish would refuse the apply.
func (d Desk) reachOf(ctx context.Context, actor appshared.ActorContext, p *plan) error {
	var grants []any
	grantsRead := false
	for _, change := range p.Changes {
		switch {
		case change.Section == sectionWorkflows && change.Op == domain.OpChange && slices.Contains(change.Fields, "shape"):
			answer, err := d.Catalogue.Invoke(ctx, previewWorkflowPublish, actor, usecase.Input{
				"key": change.Key, "shape": change.After["shape"],
			})
			if err != nil {
				if withheld(err) {
					continue
				}
				p.Refusals = append(p.Refusals, fieldsOf(err, change.Path())...)
				continue
			}
			normal, err := normalise(map[string]any(answer))
			if err != nil {
				return err
			}
			preview, _ := normal.(map[string]any)
			r := reach{Path: change.Path(), Entries: whole(preview["entries"])}
			r.Moves, _ = preview["moves"].([]any)
			r.Removed, _ = preview["removed"].([]any)
			for _, raw := range r.Removed {
				removed, _ := raw.(map[string]any)
				if whole(removed["count"]) > 0 {
					p.Refusals = append(p.Refusals, domain.Refusal("workflows.mapping_required", change.Path()+".shape",
						map[string]string{"status": text(removed["status"]), "count": strconv.Itoa(whole(removed["count"]))}))
				}
			}
			p.Reach = append(p.Reach, r)
		case change.Section == sectionRoles && change.Op != domain.OpAdd:
			if !grantsRead {
				grantsRead = true
				answer, err := d.Catalogue.Invoke(ctx, listRoles, actor, usecase.Input{})
				if err != nil && !withheld(err) {
					return err
				}
				if err == nil {
					normal, err := normalise(map[string]any(answer))
					if err != nil {
						return err
					}
					grants, _ = normal.(map[string]any)["grants"].([]any)
				}
			}
			r := reach{Path: change.Path()}
			before, after := names(change.Before["permissions"]), names(change.After["permissions"])
			for _, permission := range after {
				if !slices.Contains(before, permission) {
					r.Gained = append(r.Gained, permission)
				}
			}
			for _, permission := range before {
				if !slices.Contains(after, permission) {
					r.Lost = append(r.Lost, permission)
				}
			}
			accounts, groups := map[string]bool{}, map[string]bool{}
			for _, raw := range grants {
				grant, _ := raw.(map[string]any)
				if text(grant["role"]) != change.Key {
					continue
				}
				if id := text(grant["account_id"]); id != "" {
					accounts[id] = true
				}
				if id := text(grant["group_id"]); id != "" {
					groups[id] = true
				}
			}
			r.Accounts, r.Groups = len(accounts), len(groups)
			if len(r.Gained)+len(r.Lost) > 0 {
				p.Reach = append(p.Reach, r)
			}
		}
	}
	if len(p.Refusals) > 0 {
		p.Hash = ""
	}
	return nil
}

func whole(v any) int {
	n, _ := v.(float64)
	return int(n)
}

func names(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ReachOutput is a plan's reach as every door answers it.
func ReachOutput(rows []reach) []usecase.Output {
	out := make([]usecase.Output, 0, len(rows))
	for _, r := range rows {
		row := usecase.Output{"path": r.Path}
		if r.Moves != nil || r.Removed != nil {
			row["entries"] = r.Entries
			row["moves"] = orEmpty(r.Moves)
			row["removed"] = orEmpty(r.Removed)
		}
		if r.Gained != nil || r.Lost != nil {
			row["accounts"] = r.Accounts
			row["groups"] = r.Groups
			row["gained"] = nonNil(r.Gained)
			row["lost"] = nonNil(r.Lost)
		}
		out = append(out, row)
	}
	return out
}

func orEmpty(v []any) []any {
	if v == nil {
		return []any{}
	}
	return v
}
