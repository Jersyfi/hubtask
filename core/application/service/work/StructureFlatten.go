// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// Narrowing is what a structure change does with the entries it hides by their level.
type Narrowing string

const (
	// NarrowingHide keeps them where they are, stored and marked hidden: the default, undone by
	// switching back (domain-model.md §2, "Narrowing hides, it never deletes").
	NarrowingHide Narrowing = "HIDE"
	// NarrowingFlatten moves them to the top level (UC-WRK-24 check 6); switching back does not
	// undo it.
	NarrowingFlatten Narrowing = "FLATTEN"
)

// flattenEntryCeiling bounds the entries one flatten reads, as the preview bounds what it counts:
// past it the change is refused rather than half applied.
const flattenEntryCeiling = previewEntryCeiling

// StructureFlattener moves the entries a structure hides by their level to the top level of their
// collection: each as an entry of the top level's type, labelled with its former parent's title,
// its history recording the move. Deepest first, so that each one is a leaf when it moves and only
// a trashed entry's path has to follow it.
type StructureFlattener struct {
	Placement PlacementWriter
	Labels    ItemLabelWriter
	NewLabels CreateLabel
	Shapes    repository.EntryShapes
	Flat      repository.ItemFlattener
}

// Wired reports whether the flattener has what it needs.
func (f StructureFlattener) Wired() bool {
	return f.Shapes != nil && f.Flat != nil && f.Placement.Items != nil && f.Labels.ItemLabels != nil &&
		f.NewLabels.Labels != nil
}

// flatteningOrder answers the entries of one collection its structure hides by their level,
// deepest first; entries of equal depth by path, so that a repeat moves them in the same order.
func flatteningOrder(offered domain.Offered, shapes []repository.EntryShape) []repository.EntryShape {
	types := make(map[shared.ID]domain.ItemType, len(shapes))
	for _, shape := range shapes {
		types[shape.ID] = shape.Type
	}
	hidden := []repository.EntryShape{}
	for _, shape := range shapes {
		ids := domain.PathIDs(shape.Path)
		ancestors := make([]domain.ItemType, 0, len(ids))
		for i := len(ids) - 2; i >= 0; i-- {
			if itemType, held := types[ids[i]]; held {
				ancestors = append(ancestors, itemType)
			}
		}
		if offered.Hidden(shape.Type, ancestors).Level {
			hidden = append(hidden, shape)
		}
	}
	slices.SortFunc(hidden, func(a, b repository.EntryShape) int {
		if depth := cmp.Compare(len(domain.PathIDs(b.Path)), len(domain.PathIDs(a.Path))); depth != 0 {
			return depth
		}
		return strings.Compare(a.Path, b.Path)
	})
	return hidden
}

// flattenCount answers how many entries a flatten would move in these collections, each under its
// structure after the change.
func flattenCount(
	bound domain.StructureBound, after map[shared.ID]domain.EffectiveStructure, shapes []repository.EntryShape,
) int {
	byCollection := map[shared.ID][]repository.EntryShape{}
	for _, shape := range shapes {
		byCollection[shape.CollectionID] = append(byCollection[shape.CollectionID], shape)
	}
	moved := 0
	for id, effective := range after {
		moved += len(flatteningOrder(domain.NewOffered(bound, effective), byCollection[id]))
	}
	return moved
}

// flatten moves what the structures after the change hide, inside the caller's transaction, and
// answers how many entries moved.
func (f StructureFlattener) flatten(
	ctx context.Context, actor appshared.ActorContext, bound domain.StructureBound,
	collections map[shared.ID]domain.Container, after map[shared.ID]domain.EffectiveStructure, now time.Time,
) (int, error) {
	if !f.Wired() {
		return 0, shared.ErrInternal.WithDetail("containers.structure_unwired")
	}
	ids := make([]shared.ID, 0, len(after))
	for id := range after {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b shared.ID) int { return strings.Compare(a.String(), b.String()) })
	shapes, err := f.Shapes.ShapesIn(ctx, ids, flattenEntryCeiling+1)
	if err != nil {
		return 0, err
	}
	if len(shapes) > flattenEntryCeiling {
		return 0, shared.ErrValidation.WithDetail("structure.flatten_too_large").
			WithParams(map[string]string{"maximum": "50000"})
	}
	byCollection := map[shared.ID][]repository.EntryShape{}
	for _, shape := range shapes {
		byCollection[shape.CollectionID] = append(byCollection[shape.CollectionID], shape)
	}
	moved := 0
	for _, id := range ids {
		effective := after[id]
		order := flatteningOrder(domain.NewOffered(bound, effective), byCollection[id])
		if len(order) == 0 || len(effective.Values.Levels) == 0 {
			continue
		}
		top := effective.Values.Levels[0].Type
		profile, _ := bound.Profile(top)
		labels, err := f.labelsOf(ctx, id)
		if err != nil {
			return 0, err
		}
		for _, shape := range order {
			if err := f.move(ctx, actor, collections[id], shape.ID, top, profile, labels, now); err != nil {
				return 0, err
			}
			moved++
		}
	}
	return moved, nil
}

// labelsOf is the collection's labels by name, folded as a person compares them.
func (f StructureFlattener) labelsOf(ctx context.Context, collectionID shared.ID) (map[string]shared.ID, error) {
	held, err := f.NewLabels.Labels.List(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	labels := make(map[string]shared.ID, len(held))
	for _, label := range held {
		labels[strings.ToLower(label.Name)] = label.ID
	}
	return labels, nil
}

// move makes one entry a top-level entry of the top level's type and labels it with its former
// parent's title.
func (f StructureFlattener) move(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, id shared.ID,
	top domain.ItemType, profile domain.CapabilityProfile, labels map[string]shared.ID, now time.Time,
) error {
	p := f.Placement
	before, err := p.Items.Find(ctx, id)
	if err != nil {
		return err
	}
	parent, err := p.Items.Find(ctx, before.ParentID)
	if err != nil {
		return err
	}
	previous, _, err := p.Items.Neighbours(ctx, repository.Level{CollectionID: before.CollectionID}, "", id)
	if err != nil {
		return err
	}
	orderKey, err := service.OrderKeyBetween(previous, "")
	if err != nil {
		return err
	}
	after := before
	after.Type = top
	after.ParentID = ""
	after.Path = domain.RootPath(id)
	after.Depth = 1
	after.OrderKey = orderKey
	after.UpdatedAt = now
	if err := f.Flat.Flatten(ctx, repository.Flattening{
		Item: after, OldPath: before.Path, DepthDelta: after.Depth - before.Depth, ExpectedVersion: before.Version,
	}); err != nil {
		return err
	}
	after.Version = before.Version + 1

	announcement, err := event.NewItemMoved(p.IDs.NewID(), after, event.Movement{
		FromParentID: before.ParentID, FromPath: before.Path, FromCollectionID: before.CollectionID,
		FromBucketID: before.BucketID,
	}, event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return err
	}
	if err := p.Events.Append(ctx, announcement); err != nil {
		return err
	}
	if err := p.recordChanges(ctx, before, after, actor); err != nil {
		return err
	}
	if err := p.Changes.Record(ctx, changelog.Change{
		TenantID: after.TenantID, Entity: itemTarget, EntityID: after.ID, Op: changelog.Upsert,
		ContainerID: after.CollectionID, ActorID: actor.AccountID, HLC: p.HLC.Next(),
		Field: fieldType, Payload: map[string]any{fieldType: string(after.Type)},
	}); err != nil {
		return err
	}
	if err := f.recordAudit(ctx, before, after, actor, now); err != nil {
		return err
	}
	moved := []domain.FieldChange{
		{Field: domain.FieldParentID, From: before.ParentID.String(), To: ""},
		{Field: fieldType, From: string(before.Type), To: string(after.Type)},
		{Field: domain.FieldOrderKey, From: before.OrderKey, To: after.OrderKey},
	}
	if err := p.Activity.record(ctx, actor, after, activity.ItemMoved,
		activity.ChangeSet(historyForm(profile), historyFields(moved)...), now); err != nil {
		return err
	}
	return f.label(ctx, actor, collection, after, domain.LabelNameFrom(parent.Title), labels, now)
}

// fieldType is the entry's type as the change log, the history and the trail name it.
const fieldType = "type"

// label puts the label of this name on the entry, creating it in the collection where none has
// the name. The label is stored even where the collection does not offer labels: a value hidden,
// never refused here, comes back when the structure offers it.
func (f StructureFlattener) label(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container, item domain.WorkItem,
	name string, labels map[string]shared.ID, now time.Time,
) error {
	if name == "" {
		return nil
	}
	labelID, held := labels[strings.ToLower(name)]
	if !held {
		created, err := f.NewLabels.insertWithin(ctx, actor, collection, CreateLabelCommand{
			// The quiet colour: a label nobody chose should not look like one somebody did.
			CollectionID: collection.ID, Name: name, ColorToken: string(shared.LabelTokenSlate),
		})
		if err != nil {
			return err
		}
		labelID = created.ID
		labels[strings.ToLower(created.Name)] = created.ID
	}
	tag := setTag(ctx, domain.SetLabels, f.Labels.HLC)
	changed, err := f.Labels.apply(ctx, LabelCommand{ItemID: item.ID, LabelID: labelID}, adding, tag)
	if err != nil || !changed {
		return err
	}
	return f.Labels.announce(ctx, actor, item, collection, labelID, adding, tag, now)
}

// recordAudit writes the evidence of one flattened entry: structure only, no title (rule 10).
func (f StructureFlattener) recordAudit(
	ctx context.Context, before, after domain.WorkItem, actor appshared.ActorContext, now time.Time,
) error {
	return f.Placement.Audit.Append(ctx, audit.Entry{
		TenantID: after.TenantID, OccurredAt: now, Action: ItemMovedAction,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: itemTarget, TargetID: after.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "parent_id", Classification: audit.Open, From: idOrNil(before.ParentID), To: nil},
			audit.Change{Field: fieldType, Classification: audit.Open, From: string(before.Type), To: string(after.Type)},
			audit.Change{Field: "order_key", Classification: audit.Open, From: before.OrderKey, To: after.OrderKey},
		),
	})
}
