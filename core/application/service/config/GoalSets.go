// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"strconv"
	"strings"
	"time"

	configrepo "github.com/Jersyfi/hubtask/core/application/repository/config"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListGoalSetsName = "ListGoalSets"
	AddGoalSetName   = "AddGoalSet"

	GoalSetAddedAction audit.Action = "config.goal_set_added"

	// sectionGoalSets is the goal periods a template brings to the workspace, each with its goals
	// and their key results (configuration.md §6).
	sectionGoalSets = "goal_sets"
	// spanAttribute is a template's goal set's period, resolved when the template is planned.
	spanAttribute = "span"
)

// goalSetAttributes is what a block names; the document reads back the period's name and days
// alone - its goals are the workspace's to change from the moment they are written.
var goalSetAttributes = []string{"name", "start_on", "end_on", "goals"}

// ListGoalSets answers the goal periods a template brought to the workspace.
type ListGoalSets struct{ Desk Desk }

func (h ListGoalSets) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListGoalSetsName,
		Permission: service.PermissionStructure,
		Summary: "Lists the goal periods a template brought to the workspace - a quarter's objectives with " +
			"their key results, or a period alone - marked under their key: the workspace document's " +
			"goal_sets section.",
		SideEffects: "None.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, ReadAction, scopeRead, true); err != nil {
				return nil, err
			}
			var sets []configrepo.GoalSet
			err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				sets, err = h.Desk.GoalSets.List(ctx)
				return err
			})
			if err != nil {
				return nil, err
			}
			rows := make([]any, 0, len(sets))
			for _, s := range sets {
				rows = append(rows, goalSetRow(s.Key, s.PeriodID.String(), s.Name,
					s.StartOn.Format(time.DateOnly), s.EndOn.Format(time.DateOnly)))
			}
			return usecase.Output{"data": rows}, nil
		}),
	}
}

func goalSetRow(key, period, name, start, end string) map[string]any {
	return map[string]any{"key": key, "period_id": period, "name": name, "start_on": start, "end_on": end}
}

// AddGoalSet writes the workspace's goals into a goal period, marked as a template's.
type AddGoalSet struct{ Desk Desk }

func (h AddGoalSet) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       AddGoalSetName,
		Permission: service.PermissionStructure,
		Summary: "Writes the workspace's goals and their key results into the open goal period of exactly " +
			"the set's days - opening it where there is none - in one write, and marks the set under its " +
			"key, as a template's goal sets are taken. A key the workspace marks already is refused. The " +
			"period and its goals are ordinary ones: writing them asks what opening a period and writing a goal ask.",
		SideEffects: "Opens the period where needed, writes its goals with everything writing a goal writes, marks the " +
			"period, and writes an audit entry.",
		TokenScope: scopeItemsWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The set's key in the workspace, a lower-case word."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "The period's name, e.g. 2026 Q4."},
			{Name: "start_on", Kind: usecase.KindString, Required: true, Description: "The period's first day, YYYY-MM-DD."},
			{Name: "end_on", Kind: usecase.KindString, Required: true, Description: "The period's last day, YYYY-MM-DD."},
			{Name: "goals", Kind: usecase.KindList, Description: "At most 20 of the workspace's goals, each {title, description, key_results} as creating a goal takes them."},
		},
		Audit:    usecase.AuditDeclaration{Action: GoalSetAddedAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "the period's and the goals' writing is their own activity."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, GoalSetAddedAction, scopeItemsWrite, false); err != nil {
				return nil, err
			}
			return h.Desk.addGoalSet(ctx, actor, in)
		}),
	}
}

// maxGoalsInSet bounds one write, as a goal bounds its key results.
const maxGoalsInSet = 20

func (d Desk) addGoalSet(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	key := in.String("key")
	if !sampleKeyPattern.MatchString(key) {
		return nil, refused("config.key_invalid", "/key", nil)
	}
	goals, _ := in["goals"].([]any)
	if len(goals) > maxGoalsInSet {
		return nil, refused("config.goal_set_too_many_goals", "/goals", map[string]string{"maximum": strconv.Itoa(maxGoalsInSet)})
	}
	var out usecase.Output
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		period, err := d.goalSetPeriod(ctx, actor, in)
		if err != nil {
			return err
		}
		id, err := shared.ParseID(text(period["id"]))
		if err != nil {
			return err
		}
		for _, raw := range goals {
			goal, _ := raw.(map[string]any)
			create := usecase.Input{"period_id": id.String(), "title": text(goal["title"])}
			if description := text(goal["description"]); description != "" {
				create["description"] = description
			}
			if results, ok := goal["key_results"].([]any); ok && len(results) > 0 {
				create["key_results"] = results
			}
			if _, err := d.Catalogue.Invoke(ctx, "CreateGoal", actor, create); err != nil {
				return err
			}
		}
		now := d.Clock.Now()
		marked, err := d.GoalSets.Mark(ctx, id, key, now)
		if err != nil {
			return err
		}
		if !marked {
			// The whole write rolls back: no unmarked period is left behind a refusal.
			return refused("config.goal_set_taken", "/key", map[string]string{"key": key})
		}
		out = goalSetRow(key, id.String(), text(period["name"]), text(period["start_on"]), text(period["end_on"]))
		return d.trail(ctx, actor, GoalSetAddedAction, now,
			audit.Change{Field: "key", Classification: audit.Open, To: key},
			audit.Change{Field: "goals", Classification: audit.Open, To: strconv.Itoa(len(goals))})
	})
	return out, err
}

// goalSetPeriod is the open period of exactly the set's days where the workspace has one - a
// second set of the same quarter, or a quarter somebody started by hand, takes its goals there
// rather than a twin of it - else a period opened for the set.
func (d Desk) goalSetPeriod(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	listed, err := d.Catalogue.Invoke(ctx, "ListGoalPeriods", actor, nil)
	if err != nil {
		return nil, err
	}
	for _, p := range rowsOfAny(listed["data"]) {
		if text(p["state"]) == "OPEN" && text(p["start_on"]) == in.String("start_on") && text(p["end_on"]) == in.String("end_on") {
			return usecase.Output(p), nil
		}
	}
	return d.Catalogue.Invoke(ctx, "DefineGoalPeriod", actor, usecase.Input{
		"name": in.String("name"), "start_on": in.String("start_on"), "end_on": in.String("end_on"),
	})
}

// goalSetsSection reads the goal periods a template brought and writes new ones. Every attribute
// is fixed and a block is never removed: once written, a period and its goals are the
// workspace's, closed as a period is, never deleted by a document.
func goalSetsSection() section {
	return section{
		name: sectionGoalSets, module: module.Goals, list: ListGoalSetsName, rows: "data", key: "key",
		create: AddGoalSetName, attributes: goalSetAttributes, fixed: goalSetAttributes,
		block: func(row map[string]any) (string, domain.Block) {
			return text(row["key"]), domain.Block{"name": row["name"], "start_on": row["start_on"], "end_on": row["end_on"]}
		},
		ops: func(change domain.BlockChange, _ map[string]any) ([]operation, error) {
			path := change.Path()
			switch change.Op {
			case domain.OpAdd:
				in := usecase.Input{"key": change.Key}
				for _, name := range goalSetAttributes {
					if value, held := change.After[name]; held && value != nil {
						in[name] = value
					}
				}
				return []operation{{UseCase: AddGoalSetName, Path: path, Input: in}}, nil
			case domain.OpChange:
				return nil, refused("config.attribute_fixed", path+"."+change.Fields[0], nil)
			default:
				return nil, refused("config.not_removable", path, nil)
			}
		},
	}
}

// resolvedGoalSets is a template's workspace part with each goal set's span made the period it
// names around today: its first and last day, and its name with {{year}}, {{quarter}} and
// {{half}} filled in. A block naming its own days keeps them.
func resolvedGoalSets(d domain.Document, today time.Time) domain.Document {
	sets, held := d[sectionGoalSets]
	if !held {
		return d
	}
	out := make(domain.Document, len(d))
	for name, s := range d {
		out[name] = s
	}
	resolved := make(domain.Section, len(sets))
	for key, block := range sets {
		span := text(block[spanAttribute])
		if block == nil || span == "" {
			resolved[key] = block
			continue
		}
		start, end, ok := periodAround(span, today)
		copied := make(domain.Block, len(block))
		for attribute, value := range block {
			copied[attribute] = value
		}
		if ok {
			delete(copied, spanAttribute)
			copied["start_on"], copied["end_on"] = start.Format(time.DateOnly), end.Format(time.DateOnly)
			copied["name"] = strings.NewReplacer(
				"{{year}}", strconv.Itoa(start.Year()),
				"{{quarter}}", strconv.Itoa((int(start.Month())-1)/3+1),
				"{{half}}", strconv.Itoa((int(start.Month())-1)/6+1),
			).Replace(text(copied["name"]))
		}
		resolved[key] = copied
	}
	out[sectionGoalSets] = resolved
	return out
}

// periodAround answers the first and last day of the quarter, half-year or year holding today.
func periodAround(span string, today time.Time) (time.Time, time.Time, bool) {
	months := map[string]int{"QUARTER": 3, "HALF_YEAR": 6, "YEAR": 12}[span]
	if months == 0 {
		return time.Time{}, time.Time{}, false
	}
	first := (int(today.Month())-1)/months*months + 1
	start := time.Date(today.Year(), time.Month(first), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, months, -1), true
}

// withoutGoalSets is a document without the goal sets it brings, the original untouched: an
// upgrade compares none of them, so no newer version writes a second quarter beside the first.
func withoutGoalSets(d domain.Document) domain.Document {
	if _, held := d[sectionGoalSets]; !held {
		return d
	}
	out := make(domain.Document, len(d))
	for name, s := range d {
		if name != sectionGoalSets {
			out[name] = s
		}
	}
	return out
}
