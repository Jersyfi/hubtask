// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// reach is what one change touches beyond the configuration (configuration.md §2): the
// entries a workflow's new shape moves, the people a role's change gains or loses a permission
// for, the people a module switched off leaves without it. Counts and identifiers only - never an entry's content (rule 10).
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
	// HiddenEntries and HiddenValues are a hub's structure preview: the entries its levels would
	// stop showing, and the entries whose field values would be hidden. Hidden, never deleted.
	HiddenEntries, HiddenValues int
	Hides                       bool
	// People is how many people a module switched off leaves without it, Rules how many
	// switched-on rules it quiets; Switches says the row is one.
	People, Rules int
	Switches      bool
	// Intake is a form's change: per unattended door, the fields it now requires that the door
	// does not send - every creation of the type through it is refused until it does.
	Intake []work.IntakeGap
	// Withheld says the reach was not counted: the preview it comes from refused this reader (a
	// token without the preview's scope, a module off). The plan holds; what it touches is
	// unknown to this reader, and the plan says so rather than nothing.
	Withheld bool
}

const (
	previewWorkflowPublish  = "PreviewWorkflowPublish"
	listRoles               = "ListRoles"
	listBusinessCalendars   = "ListBusinessCalendars"
	previewCustomFieldReach = "PreviewCustomFieldReach"
	// The structure previews: a hub's, and the workspace's over every hub that follows it.
	previewContainerStructure = "PreviewContainerStructure"
	previewWorkspaceStructure = "PreviewWorkspaceStructure"
	previewModuleReach        = "PreviewModuleReach"
)

// reachOf asks the use cases that know what a change touches, in the plan's read: a workflow's
// publish preview for the shape the plan sets, a hub scheme's preview, a structure preview, the
// people a module switched off leaves, the role grants for a changed role. A shape that removes a status entries are in is refused here,
// as its publish would refuse the apply.
func (d Desk) reachOf(ctx context.Context, actor appshared.ActorContext, p *plan) error {
	var grants []any
	grantsRead, grantsWithheld := false, false
	structure := map[string]any{}
	reset := []any{}
	var shadows map[string]bool
	shadowsRead := false
	for _, change := range p.Changes {
		switch {
		case change.Section == sectionModules && slices.Contains([]string{"OFF", "INHERIT"}, text(change.After[switchAttribute])):
			in := usecase.Input{"module": strings.ToUpper(change.Key), "switch": text(change.After[switchAttribute])}
			if !p.Hub.IsZero() {
				in["hub_id"] = p.Hub.String()
			}
			answer, err := d.preview(ctx, previewModuleReach, actor, in)
			if err != nil {
				if withheld(err) {
					p.Reach = append(p.Reach, reach{Path: change.Path(), Withheld: true})
					continue
				}
				p.Refusals = append(p.Refusals, fieldsOf(err, change.Path())...)
				continue
			}
			normal, err := normalise(map[string]any(answer))
			if err != nil {
				return err
			}
			counted, _ := normal.(map[string]any)
			p.Reach = append(p.Reach, reach{
				Path: change.Path(), Switches: true, People: whole(counted["people"]), Rules: whole(counted["rules"]),
			})
		case change.Section == sectionStructure:
			if change.Op == domain.OpRemove {
				reset = append(reset, change.Key)
			} else {
				structure[change.Key] = change.After[settingValueField]
			}
		case change.Section == sectionWorkflows && change.Op == domain.OpChange && slices.Contains(change.Fields, "shape"):
			answer, err := d.preview(ctx, previewWorkflowPublish, actor, usecase.Input{
				"key": change.Key, "shape": change.After["shape"],
			})
			if err != nil {
				if withheld(err) {
					p.Reach = append(p.Reach, reach{Path: change.Path(), Withheld: true})
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
		case change.Section == sectionScheme && !p.Hub.IsZero():
			// The scheme's own preview: the entries that take another status where the hub runs
			// another workflow for the type.
			entryType := change.Key
			if entryType == schemeEveryType {
				entryType = ""
			}
			answer, err := d.preview(ctx, "PreviewWorkflowScheme", actor, usecase.Input{
				"container_id": p.Hub.String(), "entry_type": entryType, "workflow": text(change.After[workflowAttribute]),
			})
			if err != nil {
				if withheld(err) {
					p.Reach = append(p.Reach, reach{Path: change.Path(), Withheld: true})
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
			if r.Moves == nil {
				r.Moves = []any{}
			}
			p.Reach = append(p.Reach, r)
		case change.Section == sectionTypes && change.Op == domain.OpChange && slices.Contains(change.Fields, "form"):
			// A form the plan cannot read is the apply's to refuse, with its path.
			before, errBefore := workflow.FormFromInput(change.Before["form"])
			after, errAfter := workflow.FormFromInput(change.After["form"])
			if errBefore != nil || errAfter != nil {
				continue
			}
			if gaps := work.IntakeGaps(before, after); len(gaps) > 0 {
				p.Reach = append(p.Reach, reach{Path: change.Path(), Intake: gaps})
			}
		case change.Section == sectionCalendars && change.Op == domain.OpRemove && p.Hub.IsZero():
			// DeleteBusinessCalendar refuses a calendar a policy names, unless the installation offers
			// its key. Counted in the document's own policies, which the apply defines and removes
			// before it removes a calendar; a hub's own version and a rule's trigger are found at the apply.
			if domain.PoliciesNamingCalendar(p.Target[sectionPolicies], change.Key) == 0 {
				continue
			}
			if !shadowsRead {
				shadowsRead = true
				var err error
				if shadows, err = d.shadowedCalendars(ctx, actor); err != nil {
					return err
				}
			}
			// nil: the reader may not list the calendars, and the apply refuses what the plan cannot see.
			if shadows != nil && !shadows[change.Key] {
				p.Refusals = append(p.Refusals, domain.Refusal("calendars.in_use", change.Path(),
					map[string]string{"key": change.Key}))
			}
		case change.Section == sectionRoles && change.Op != domain.OpAdd:
			if !grantsRead {
				grantsRead = true
				answer, err := d.preview(ctx, listRoles, actor, usecase.Input{})
				if err != nil && !withheld(err) {
					return err
				}
				grantsWithheld = err != nil
				if err == nil {
					normal, err := normalise(map[string]any(answer))
					if err != nil {
						return err
					}
					grants, _ = normal.(map[string]any)["grants"].([]any)
				}
			}
			r := reach{Path: change.Path(), Withheld: grantsWithheld}
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
			held := 0
			for _, raw := range grants {
				grant, _ := raw.(map[string]any)
				if text(grant["role"]) != change.Key {
					continue
				}
				held++
				if id := text(grant["account_id"]); id != "" {
					accounts[id] = true
				}
				if id := text(grant["group_id"]); id != "" {
					groups[id] = true
				}
			}
			r.Accounts, r.Groups = len(accounts), len(groups)
			// DeleteRole refuses a role somebody holds or a switched-on rule addresses; the plan says
			// so rather than the apply. Rules are counted in the document's own target, since the
			// apply changes and removes them before it removes a role; another scope's rules are
			// found at the apply.
			if change.Op == domain.OpRemove && held > 0 {
				p.Refusals = append(p.Refusals, domain.Refusal("roles.held", change.Path(),
					map[string]string{"key": change.Key, "grants": strconv.Itoa(held)}))
			}
			if rules := domain.RulesAddressingRole(p.Target[sectionRules], change.Key); change.Op == domain.OpRemove && rules > 0 {
				p.Refusals = append(p.Refusals, domain.Refusal("roles.named_by_rules", change.Path(),
					map[string]string{"key": change.Key, "rules": strconv.Itoa(rules)}))
			}
			if len(r.Gained)+len(r.Lost) > 0 {
				p.Reach = append(p.Reach, r)
			}
		}
	}
	if len(structure)+len(reset) > 0 {
		if err := d.structureReach(ctx, actor, p, structure, reset); err != nil {
			return err
		}
	}
	for _, field := range p.hiddenFields {
		if err := d.fieldReach(ctx, actor, p, field); err != nil {
			return err
		}
	}
	if len(p.Refusals) > 0 {
		p.Hash = ""
	}
	return nil
}

// shadowedCalendars are the workspace's calendar keys the installation also offers; nil where the
// reader may not list the calendars.
func (d Desk) shadowedCalendars(ctx context.Context, actor appshared.ActorContext) (map[string]bool, error) {
	out := map[string]bool{}
	answer, err := d.preview(ctx, listBusinessCalendars, actor, usecase.Input{"include_offered": true})
	if err != nil {
		if withheld(err) {
			return nil, nil
		}
		return nil, err
	}
	normal, err := normalise(map[string]any(answer))
	if err != nil {
		return nil, err
	}
	rows, _ := normal.(map[string]any)["data"].([]any)
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if shadows, _ := row["shadows_offered"].(bool); shadows {
			out[text(row["key"])] = true
		}
	}
	return out, nil
}

// structureReach asks the structure preview of the plan's scope - a hub's, or the workspace's
// over every hub and collection that follows it - once for every structure change the plan makes
// there, what it would stop showing.
func (d Desk) structureReach(ctx context.Context, actor appshared.ActorContext, p *plan, settings map[string]any, reset []any) error {
	asked, in := previewWorkspaceStructure, usecase.Input{}
	if !p.Hub.IsZero() {
		asked, in = previewContainerStructure, usecase.Input{"container_id": p.Hub.String()}
	}
	if len(settings) > 0 {
		in["settings"] = settings
	}
	if len(reset) > 0 {
		in["reset"] = reset
	}
	answer, err := d.preview(ctx, asked, actor, in)
	if err != nil {
		if withheld(err) {
			p.Reach = append(p.Reach, reach{Path: sectionStructure, Withheld: true})
			return nil
		}
		p.Refusals = append(p.Refusals, fieldsOf(err, sectionStructure)...)
		return nil
	}
	normal, err := normalise(map[string]any(answer))
	if err != nil {
		return err
	}
	preview, _ := normal.(map[string]any)
	r := reach{Path: sectionStructure}
	r.Hides, _ = preview["narrows"].(bool)
	levels, _ := preview["levels"].([]any)
	for _, raw := range levels {
		level, _ := raw.(map[string]any)
		r.HiddenEntries += whole(level["entries"])
	}
	fields, _ := preview["fields"].([]any)
	for _, raw := range fields {
		field, _ := raw.(map[string]any)
		r.HiddenValues += whole(field["entries"])
	}
	if r.Hides {
		p.Reach = append(p.Reach, r)
	}
	return nil
}

// fieldReach asks how many entries hold a value under a field the plan removes or hides from
// readers: what would stop being shown, never deleted.
func (d Desk) fieldReach(ctx context.Context, actor appshared.ActorContext, p *plan, field hiddenField) error {
	if field.ID == "" {
		return nil
	}
	answer, err := d.preview(ctx, previewCustomFieldReach, actor, usecase.Input{"field_id": field.ID})
	if err != nil {
		if withheld(err) {
			p.Reach = append(p.Reach, reach{Path: field.Path, Withheld: true})
			return nil
		}
		p.Refusals = append(p.Refusals, fieldsOf(err, field.Path)...)
		return nil
	}
	normal, err := normalise(map[string]any(answer))
	if err != nil {
		return err
	}
	counted, _ := normal.(map[string]any)
	p.Reach = append(p.Reach, reach{Path: field.Path, Hides: true, HiddenValues: whole(counted["entries"])})
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
		if r.Hides {
			row["hidden_entries"] = r.HiddenEntries
			row["hidden_values"] = r.HiddenValues
		}
		if r.Switches {
			row["people"] = r.People
			row["rules"] = r.Rules
		}
		if len(r.Intake) > 0 {
			doors := make([]usecase.Output, 0, len(r.Intake))
			for _, gap := range r.Intake {
				doors = append(doors, usecase.Output{"channel": string(gap.Channel), "fields": gap.Fields})
			}
			row["intake"] = doors
		}
		if r.Withheld {
			row["withheld"] = true
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
