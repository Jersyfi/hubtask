// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"strconv"

	planrepo "github.com/Jersyfi/hubtask/core/application/repository/planning"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	SetDependencyTimingName = "SetDependencyTiming"
	SetEntrySchedulingName  = "SetEntryScheduling"
)

// entryWritable reads an entry and asks WRITE_ITEMS on it, as a change of its own fields asks.
func (p Planner) entryWritable(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, action audit.Action,
) (work.WorkItem, error) {
	var (
		item       work.WorkItem
		collection work.Container
	)
	if err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := p.Items.Find(ctx, id)
		if err != nil {
			return err
		}
		item = found
		collection, err = p.Containers.Find(ctx, found.CollectionID)
		return err
	}); err != nil {
		return work.WorkItem{}, err
	}
	if item.DeletedAt != nil {
		return work.WorkItem{}, appshared.ItemNotFound(id)
	}
	if err := p.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       append(service.ContainerScopes(collection), identity.ItemScope(item.ID)),
		Action:     action, TokenScope: itemsWrite, TargetType: itemTarget, TargetID: item.ID,
		On: access.ItemSubject{Does: service.ItemChange, ID: item.ID, Assignee: item.AssigneeID},
	}); err != nil {
		return work.WorkItem{}, err
	}
	return item, nil
}

// SetDependencyTimingCommand is the input, typed.
type SetDependencyTimingCommand struct {
	LinkID shared.ID
	Timing domain.Timing
}

// SetDependencyTiming sets how a dependency's waiting end hangs on the other: finish-to-start,
// start-to-start, finish-to-finish or start-to-finish, with a lag or a lead in working days
// (PLN-02). Whoever may change the waiting entry sets it: it is that entry's dates that move.
type SetDependencyTiming struct{ Planner Planner }

// Execute sets the timing.
func (h SetDependencyTiming) Execute(ctx context.Context, actor appshared.ActorContext, cmd SetDependencyTimingCommand) (work.EntryLink, error) {
	if cmd.LinkID.IsZero() {
		return work.EntryLink{}, invalid("/link_id", "links.id_required")
	}
	var (
		link     work.EntryLink
		linkType work.LinkType
	)
	if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Planner.Links.Find(ctx, cmd.LinkID)
		if err != nil {
			return err
		}
		link = found
		rows, err := h.Planner.LinkTypes.List(ctx)
		if err != nil {
			return err
		}
		t, ok := work.NewLinkTypes(rows).Find(found.Type)
		if !ok {
			return shared.ErrNotFound.WithDetail("links.not_found")
		}
		linkType = t
		return nil
	}); err != nil {
		return work.EntryLink{}, err
	}
	waiter, _, ok := link.Dependency(linkType)
	if !ok {
		return work.EntryLink{}, invalid("/dependency_kind", "links.timing_without_dependency")
	}
	if _, err := h.Planner.entryWritable(ctx, actor, waiter, DependencyTimedAction); err != nil {
		return work.EntryLink{}, err
	}
	timing, err := linkType.TimingOf(cmd.Timing)
	if err != nil {
		return work.EntryLink{}, err
	}
	now := h.Planner.Clock.Now()
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Planner.Plans.SetTiming(ctx, link.ID, timing); err != nil {
			return err
		}
		before := link.Timing
		link.Timing = timing
		if err := h.recordChange(ctx, actor, link); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, DependencyTimedAction, linkTarget, link.ID, now,
			audit.Change{Field: "dependency_kind", Classification: audit.Open, From: string(before.Kind), To: string(timing.Kind)},
			audit.Change{Field: "lag_days", Classification: audit.Open, From: strconv.Itoa(before.LagDays), To: strconv.Itoa(timing.LagDays)})
	})
	if err != nil {
		return work.EntryLink{}, err
	}
	return link, nil
}

// recordChange files the link again for offline devices, as drawing it did.
func (h SetDependencyTiming) recordChange(ctx context.Context, actor appshared.ActorContext, link work.EntryLink) error {
	if h.Planner.Changes == nil {
		return nil
	}
	source, err := h.Planner.Items.Find(ctx, link.SourceID)
	if err != nil {
		return err
	}
	target, err := h.Planner.Items.Find(ctx, link.TargetID)
	if err != nil {
		return err
	}
	return h.Planner.Changes.Record(ctx, changelog.Change{
		TenantID: actor.TenantID, Entity: linkTarget, EntityID: link.ID, Op: changelog.Upsert,
		ContainerID: source.CollectionID, ActorID: actor.AccountID, HLC: h.Planner.HLC.Next(),
		Payload: workservice.LinkRecord(link, source.CollectionID, target.CollectionID),
	})
}

// Descriptor is the catalogue entry.
func (h SetDependencyTiming) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetDependencyTimingName, Permission: service.PermissionWriteItems,
		Summary: "Sets a dependency's kind - FS (finish to start), SS, FF or SF - and its lag in working days, " +
			"negative for a lead, at most 999 either way. Needs write access to the waiting entry; a link " +
			"nobody waits on takes no timing (links.timing_without_dependency).",
		SideEffects: "Writes the link's timing, tells offline devices and writes an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "link_id", Kind: usecase.KindID, Required: true, Description: "The dependency."},
			{Name: "dependency_kind", Kind: usecase.KindString, Required: true, Enum: []string{"FS", "SS", "FF", "SF"}, Description: "How the waiting entry hangs on the other."},
			{Name: "lag_days", Kind: usecase.KindInt, Description: "Working days between the two moments; negative is a lead. 0 without."},
		},
		Audit:    usecase.AuditDeclaration{Action: DependencyTimedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a dependency's timing changes neither entry; the scheduler's moves are recorded on the entries they move."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("link_id")
			if err != nil {
				return nil, err
			}
			link, err := h.Execute(ctx, actor, SetDependencyTimingCommand{
				LinkID: id, Timing: domain.Timing{Kind: domain.DependencyKind(in.String("dependency_kind")), LagDays: in.Int("lag_days")},
			})
			if err != nil {
				return nil, err
			}
			return workservice.LinkOutput(link), nil
		}),
	}, linkTarget)
}

// SetEntrySchedulingCommand is the input, typed. A nil field keeps what stands.
type SetEntrySchedulingCommand struct {
	ItemID    shared.ID
	Mode      *domain.Mode
	Milestone *bool
}

// SetEntryScheduling says whether the scheduler may move an entry, and whether it is a milestone
// (PLN-02, PLN-05). Every entry is manual until a planner says otherwise.
type SetEntryScheduling struct{ Planner Planner }

// Execute writes the entry's mode and mark.
func (h SetEntryScheduling) Execute(ctx context.Context, actor appshared.ActorContext, cmd SetEntrySchedulingCommand) (planrepo.EntrySchedule, error) {
	if cmd.ItemID.IsZero() {
		return planrepo.EntrySchedule{}, invalid("/item_id", "items.item_id_required")
	}
	if cmd.Mode == nil && cmd.Milestone == nil {
		return planrepo.EntrySchedule{}, shared.ErrValidation.WithDetail("planning.update_empty")
	}
	if cmd.Mode != nil && !cmd.Mode.Valid() {
		return planrepo.EntrySchedule{}, invalid("/mode", "planning.mode_unknown")
	}
	item, err := h.Planner.entryWritable(ctx, actor, cmd.ItemID, EntryScheduledAction)
	if err != nil {
		return planrepo.EntrySchedule{}, err
	}
	if err := item.EnsureEditable(); err != nil {
		return planrepo.EntrySchedule{}, err
	}
	now := h.Planner.Clock.Now()
	var out planrepo.EntrySchedule
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := h.Planner.Plans.Schedules(ctx, []shared.ID{item.ID})
		if err != nil {
			return err
		}
		current, ok := stored[item.ID]
		if !ok {
			current = planrepo.EntrySchedule{EntryID: item.ID, Mode: domain.Manual}
		}
		next := current
		if cmd.Mode != nil {
			next.Mode = *cmd.Mode
		}
		if cmd.Milestone != nil {
			next.Milestone = *cmd.Milestone
		}
		next.UpdatedAt = now
		if err := h.Planner.Plans.SetSchedule(ctx, next); err != nil {
			return err
		}
		next.Version = current.Version + 1
		out = next
		if h.Planner.Changes != nil {
			if err := h.Planner.Changes.Record(ctx, changelog.Change{
				TenantID: actor.TenantID, Entity: ScheduleEntity, EntityID: item.ID, Op: changelog.Upsert,
				ContainerID: item.CollectionID, ActorID: actor.AccountID, HLC: h.Planner.HLC.Next(),
				Payload: ScheduleRecord(item.ID, item.CollectionID, next.Mode, next.Milestone, next.Version),
			}); err != nil {
				return err
			}
		}
		return h.Planner.record(ctx, actor, EntryScheduledAction, itemTarget, item.ID, now,
			audit.Change{Field: "mode", Classification: audit.Open, From: string(current.Mode), To: string(next.Mode)},
			audit.Change{Field: "milestone", Classification: audit.Open, From: strconv.FormatBool(current.Milestone), To: strconv.FormatBool(next.Milestone)})
	})
	if err != nil {
		return planrepo.EntrySchedule{}, err
	}
	return out, nil
}

// ScheduleOutput is an entry's mode as every door returns it.
func ScheduleOutput(s planrepo.EntrySchedule) usecase.Output {
	return usecase.Output{"item_id": s.EntryID.String(), "mode": string(s.Mode), "milestone": s.Milestone}
}

// ScheduleEntity is what the change log and the replica file an entry's scheduling under, by the
// entry's identifier and in its collection: an offline device sees an entry's mode where it sees
// the entry.
const ScheduleEntity = "entry_schedule"

// ScheduleRecord is an entry's scheduling as the change log and the replica carry it.
func ScheduleRecord(entryID, collectionID shared.ID, mode domain.Mode, milestone bool, version int) map[string]any {
	if !mode.Valid() {
		mode = domain.Manual
	}
	return map[string]any{
		"item_id": entryID.String(), "collection_id": collectionID.String(), "mode": string(mode),
		"milestone": milestone, "version": version,
	}
}

// Descriptor is the catalogue entry.
func (h SetEntryScheduling) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetEntrySchedulingName, Permission: service.PermissionWriteItems,
		Summary: "Sets whether the scheduler may move an entry - AUTOMATIC places it as early as its " +
			"predecessors allow, MANUAL keeps it where a person put it - and whether it is a milestone, a " +
			"date without a duration. Every entry is MANUAL and no milestone until set.",
		SideEffects: "Writes the entry's scheduling mode and an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
			{Name: "mode", Kind: usecase.KindString, Enum: []string{"MANUAL", "AUTOMATIC"}, Description: "Whether the scheduler may move it; left out keeps it."},
			{Name: "milestone", Kind: usecase.KindBool, Description: "Whether it is a milestone; left out keeps it."},
		},
		Audit:    usecase.AuditDeclaration{Action: EntryScheduledAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a scheduling mode moves no date; the moves it allows are recorded on the entries they move."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			cmd := SetEntrySchedulingCommand{ItemID: id}
			if in.Present("mode") {
				mode := domain.Mode(in.String("mode"))
				cmd.Mode = &mode
			}
			if in.Present("milestone") {
				milestone := in.Bool("milestone")
				cmd.Milestone = &milestone
			}
			schedule, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return ScheduleOutput(schedule), nil
		}),
	}, itemTarget)
}
