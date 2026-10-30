// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"strconv"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ImportPlanName = "ImportPlan"
	// PlanImportedAction is a project file brought into a collection; the file's words are content
	// and stay out of the trail (rule 10), its counts go in.
	PlanImportedAction audit.Action = "plan.imported"
	createWorkItemName              = "CreateWorkItem"
	linkEntriesName                 = "LinkEntries"
)

// ImportPlanCommand is the input, typed.
type ImportPlanCommand struct {
	CollectionID shared.ID
	Format       domain.FileFormat
	Content      []byte
	// Zone is the IANA zone the file's days are local to; empty is UTC.
	Zone   string
	DryRun bool
}

// Imported is what a project file brings, and - applied - the entry each of its tasks became.
type Imported struct {
	DryRun                                     bool
	Tasks, WorkPackages, Activities, Flattened int
	Dependencies, Milestones                   int
	Created                                    []usecase.Output
}

// placed is one file task as the collection's tree takes it: its level and its parent's key.
type placed struct {
	task   domain.FileTask
	level  work.ItemType
	parent string
}

// ImportPlan brings a plan in from a project file (PLN-09): each task an entry of the collection
// at the depth its outline gives - a task, a work package under it, an activity below, deeper
// rows flattened onto the last level that may hold them -, its dates as start and all-day due
// date, its milestone mark, and each predecessor a "blocks" dependency with its kind and lag.
// Every write runs through its own use case - CreateWorkItem, LinkEntries, SetDependencyTiming,
// SetEntryScheduling - in one transaction, so the file comes in whole or not at all and each entry
// is checked, recorded and announced as one made by hand is. A dry run answers the counts only.
type ImportPlan struct{ Planner Planner }

// Execute imports the file.
func (h ImportPlan) Execute(ctx context.Context, actor appshared.ActorContext, cmd ImportPlanCommand) (Imported, error) {
	if cmd.CollectionID.IsZero() {
		return Imported{}, invalid("/collection_id", "planning.collection_required")
	}
	plan, err := h.Planner.plan(ctx, actor, cmd.CollectionID, service.PermissionWriteItems, PlanImportedAction, itemsWrite)
	if err != nil {
		return Imported{}, err
	}
	if plan.Type != work.ContainerCollection {
		return Imported{}, invalid("/collection_id", "planning.import_needs_collection")
	}
	zone := time.UTC
	if cmd.Zone != "" {
		if zone, err = time.LoadLocation(cmd.Zone); err != nil {
			return Imported{}, invalid("/zone", "planning.zone_unknown")
		}
	}
	tasks, err := domain.ParseProjectFile(cmd.Format, cmd.Content)
	if err != nil {
		return Imported{}, err
	}
	rows, out := placeTasks(tasks)
	out.DryRun = cmd.DryRun
	if cmd.DryRun {
		return out, nil
	}
	created := map[string]string{}
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		for _, row := range rows {
			in := usecase.Input{"type": string(row.level), "title": row.task.Name}
			if row.parent == "" {
				in["collection_id"] = cmd.CollectionID.String()
			} else {
				in["parent_id"] = created[row.parent]
			}
			if row.task.Start != nil && !row.task.Milestone {
				in["start_at"] = row.task.Start.In(zone).Format(time.RFC3339)
			}
			if finish := row.task.Finish; finish != nil {
				in["due_at"], in["due_date_only"], in["due_time_zone"] = finish.In(zone).Format(time.RFC3339), true, zone.String()
			}
			entry, err := h.Planner.Catalogue.Invoke(ctx, createWorkItemName, actor, in)
			if err != nil {
				return err
			}
			id, _ := entry["id"].(string)
			created[row.task.Key] = id
			out.Created = append(out.Created, usecase.Output{"key": row.task.Key, "item_id": id})
			if row.task.Milestone {
				if _, err := h.Planner.Catalogue.Invoke(ctx, SetEntrySchedulingName, actor, usecase.Input{"item_id": id, "milestone": true}); err != nil {
					return err
				}
			}
		}
		for _, row := range rows {
			for _, p := range row.task.Predecessors {
				link, err := h.Planner.Catalogue.Invoke(ctx, linkEntriesName, actor, usecase.Input{
					"item_id": created[p.Key], "target_id": created[row.task.Key], "type": work.LinkTypeBlocks,
				})
				if err != nil {
					return err
				}
				if p.Timing == (domain.Timing{Kind: domain.FinishToStart}) {
					continue
				}
				id, _ := link["id"].(string)
				if _, err := h.Planner.Catalogue.Invoke(ctx, SetDependencyTimingName, actor, usecase.Input{
					"link_id": id, "dependency_kind": string(p.Timing.Kind), "lag_days": p.Timing.LagDays,
				}); err != nil {
					return err
				}
			}
		}
		return h.Planner.record(ctx, actor, PlanImportedAction, planTarget, cmd.CollectionID, h.Planner.Clock.Now(),
			audit.Change{Field: "format", Classification: audit.Open, To: string(cmd.Format)},
			audit.Change{Field: "entries", Classification: audit.Open, To: strconv.Itoa(len(rows))},
			audit.Change{Field: "dependencies", Classification: audit.Open, To: strconv.Itoa(out.Dependencies)})
	})
	if err != nil {
		return Imported{}, err
	}
	return out, nil
}

// placeTasks gives each task its level under the collection: a task at the top, a work package
// under a task, an activity under either; a row below an activity hangs on that activity's parent.
func placeTasks(tasks []domain.FileTask) ([]placed, Imported) {
	type open struct {
		depth int
		key   string
		level work.ItemType
	}
	var (
		stack []open
		rows  = make([]placed, 0, len(tasks))
		out   Imported
	)
	for _, t := range tasks {
		for len(stack) > 0 && stack[len(stack)-1].depth >= t.Level {
			stack = stack[:len(stack)-1]
		}
		row := placed{task: t, level: work.ItemTask}
		if len(stack) > 0 {
			parent := stack[len(stack)-1]
			if parent.level == work.ItemActivity {
				out.Flattened++
				for len(stack) > 0 && stack[len(stack)-1].level == work.ItemActivity {
					stack = stack[:len(stack)-1]
				}
				parent = stack[len(stack)-1]
			}
			row.parent = parent.key
			row.level = work.ItemActivity
			if parent.level == work.ItemTask {
				row.level = work.ItemWorkPackage
			}
		}
		switch row.level {
		case work.ItemTask:
			out.Tasks++
		case work.ItemWorkPackage:
			out.WorkPackages++
		default:
			out.Activities++
		}
		if t.Milestone {
			out.Milestones++
		}
		out.Dependencies += len(t.Predecessors)
		rows = append(rows, row)
		stack = append(stack, open{depth: t.Level, key: t.Key, level: row.level})
	}
	return rows, out
}

// Output is the shape every door returns (schema PlanImport).
func (i Imported) Output() usecase.Output {
	created := i.Created
	if created == nil {
		created = []usecase.Output{}
	}
	return usecase.Output{
		"dry_run": i.DryRun, "tasks": i.Tasks, "work_packages": i.WorkPackages, "activities": i.Activities,
		"flattened": i.Flattened, "dependencies": i.Dependencies, "milestones": i.Milestones, "created": created,
	}
}

// Descriptor is the catalogue entry.
func (h ImportPlan) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ImportPlanName, Permission: service.PermissionWriteItems,
		Summary: "Brings a plan into a collection from a project file - a project tool's XML or a CSV with a " +
			"predecessors column -: each task an entry at the depth its outline gives (task, work package, " +
			"activity; deeper rows flattened), its dates as start and all-day due date, milestones marked, and " +
			"each predecessor a dependency with its kind and lag. All or nothing; a dry run answers the counts.",
		SideEffects: "Creates entries through CreateWorkItem, dependencies through LinkEntries and " +
			"SetDependencyTiming and milestones through SetEntryScheduling, each with its own event, history and " +
			"audit entry, and writes an audit entry for the import. A dry run writes nothing.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection the plan comes into."},
			{Name: "format", Kind: usecase.KindString, Required: true, Enum: []string{string(domain.FormatProjectXML), string(domain.FormatCSV)}, Description: "The file's kind."},
			{Name: "content", Kind: usecase.KindString, Required: true, Description: "The file's text, UTF-8, at most 512 KiB."},
			{Name: "zone", Kind: usecase.KindString, Description: "The IANA time zone the file's days are local to; UTC without."},
			{Name: "dry_run", Kind: usecase.KindBool, Description: "Answer what the file would bring without writing it."},
		},
		Audit:    usecase.AuditDeclaration{Action: PlanImportedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "each entry and dependency is recorded in its entry's history by the use case that writes it."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			content, _ := in["content"].(string)
			imported, err := h.Execute(ctx, actor, ImportPlanCommand{
				CollectionID: id, Format: domain.FileFormat(in.String("format")), Content: []byte(content),
				Zone: in.String("zone"), DryRun: in.Bool("dry_run"),
			})
			if err != nil {
				return nil, err
			}
			return imported.Output(), nil
		}),
	}, planTarget)
}
