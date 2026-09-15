// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The initial synchronisation (N-02, offline-sync.md §3.1): a device with no cursor is told
// everything it may read, as UPSERT records carrying whole objects, one kind at a time in an
// order a client can apply without a forward reference. The log's position is read before the
// first page and becomes the cursor the last page hands back, so that a change landing mid-walk
// is the first delta's business rather than a gap.

// walkKinds is the order of the walk. Each name is the entity the change log records under, so
// that a record from the walk and a record from the delta about the same object say the same
// thing in the same place. `set` is the exception: set elements travel as records of the entry
// they belong to, exactly as a delta's set change does.
var walkKinds = []string{
	entityContainer, entityBucket, entityLabel, entityItem, kindSet,
	entityComment, entityReminder, entityRecurrence, entityTemplate,
}

const (
	entityContainer  = "container"
	entityBucket     = "bucket"
	entityLabel      = "label"
	entityItem       = "item"
	entityComment    = "comment"
	entityReminder   = "reminder"
	entityRecurrence = "recurrence_rule"
	entityTemplate   = "template"
	// kindSet is the walk's own name for the set element pass. Not an entity: its records are the
	// entry's.
	kindSet = "set"
)

// setKeySeparator joins the three parts of a set element's key inside the cursor. A bar, because
// neither an identifier nor a set name contains one, and the cursor's own separator is a full stop.
const setKeySeparator = "|"

// walkRow is one object as the walk hands it out: the record's facts, and the key the next page
// resumes after.
type walkRow struct {
	entity      string
	entityID    shared.ID
	containerID shared.ID
	hlc         shared.HLC
	payload     map[string]any
	key         string
}

// walk serves one page of the initial synchronisation from the position, filling it across kinds
// until it is full or the walk is over.
func (p PullChanges) walk(
	ctx context.Context, actor appshared.ActorContext, from Position, limit int,
	keep func(work.Container) bool,
) (Batch, error) {
	kind := indexOfKind(from.Kind)
	if kind < 0 {
		// A kind this build does not walk: a cursor from a build that walked more, or a forged
		// one that got past the signature. Either way the walk cannot be resumed.
		return Batch{}, shared.ErrValidation.WithDetail("sync.cursor_invalid")
	}

	resolved := map[shared.ID]visibility{}
	records := make([]Record, 0, limit)
	position := from
	for kind < len(walkKinds) {
		position.Kind = walkKinds[kind]
		rows, err := p.readKind(ctx, actor, position.Kind, position.After, limit-len(records))
		if err != nil {
			return Batch{}, err
		}
		for _, row := range rows {
			position.After = row.key
			seen, err := p.Stream.mayRead(ctx, actor, row.containerID, resolved)
			if err != nil {
				return Batch{}, err
			}
			if !seen.allowed || (keep != nil && !keep(seen.container)) {
				continue
			}
			records = append(records, Record{
				Recorded: repository.Recorded{
					Change: repository.Change{
						TenantID: actor.TenantID, Entity: row.entity, EntityID: row.entityID,
						Op: repository.Upsert, ContainerID: row.containerID,
						HLC: row.hlc, Payload: row.payload,
					},
					Seq: from.Seq, OccurredAt: from.IssuedAt,
				},
				Cursor: position,
			})
		}
		if len(records) >= limit {
			return Batch{Records: records, Cursor: position, More: true}, nil
		}
		// The kind is exhausted - fewer rows than asked for - and the walk moves on to the next
		// one from its start.
		kind++
		position.After = ""
	}

	// The walk is over. The cursor is the delta position taken before the first page: everything
	// that happened since is in the log past it, and the first delta delivers it.
	return Batch{
		Records: records,
		Cursor:  Position{Seq: from.Seq, IssuedAt: from.IssuedAt},
		More:    false,
	}, nil
}

// readKind pages one kind after the key, inside one read-only transaction.
func (p PullChanges) readKind(
	ctx context.Context, actor appshared.ActorContext, kind, after string, batch int,
) ([]walkRow, error) {
	var rows []walkRow
	err := p.Stream.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		rows, err = p.pageOf(ctx, kind, after, batch)
		return err
	})
	return rows, err
}

// pageOf is the table of kinds: what each one reads, and how its rows become records.
func (p PullChanges) pageOf(ctx context.Context, kind, after string, batch int) ([]walkRow, error) {
	afterID := shared.ID(after)
	switch kind {
	case entityContainer:
		containers, err := p.Snapshot.Containers(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(containers))
		for _, container := range containers {
			rows = append(rows, walkRow{
				entity: entityContainer, entityID: container.ID, containerID: container.ID,
				payload: workservice.ContainerSnapshot(container), key: container.ID.String(),
			})
		}
		return rows, nil
	case entityBucket:
		buckets, err := p.Snapshot.Buckets(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(buckets))
		for _, bucket := range buckets {
			rows = append(rows, walkRow{
				entity: entityBucket, entityID: bucket.ID, containerID: bucket.CollectionID,
				payload: workservice.BucketSnapshot(bucket), key: bucket.ID.String(),
			})
		}
		return rows, nil
	case entityLabel:
		labels, err := p.Snapshot.Labels(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(labels))
		for _, label := range labels {
			rows = append(rows, walkRow{
				entity: entityLabel, entityID: label.ID, containerID: label.CollectionID,
				payload: workservice.LabelSnapshot(label), key: label.ID.String(),
			})
		}
		return rows, nil
	case entityItem:
		items, err := p.Snapshot.Items(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(items))
		for _, item := range items {
			rows = append(rows, walkRow{
				entity: entityItem, entityID: item.ID, containerID: item.CollectionID,
				payload: workservice.ItemSnapshot(item), key: item.ID.String(),
			})
		}
		return rows, nil
	case kindSet:
		key, err := parseSetKey(after)
		if err != nil {
			return nil, err
		}
		elements, err := p.Snapshot.SetElements(ctx, key, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(elements))
		for _, element := range elements {
			rows = append(rows, setRow(element))
		}
		return rows, nil
	case entityComment:
		comments, err := p.Snapshot.Comments(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(comments))
		for _, comment := range comments {
			rows = append(rows, walkRow{
				entity: entityComment, entityID: comment.Value.ID, containerID: comment.CollectionID,
				payload: workservice.CommentSnapshot(comment.Value), key: comment.Value.ID.String(),
			})
		}
		return rows, nil
	case entityReminder:
		reminders, err := p.Snapshot.Reminders(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(reminders))
		for _, reminder := range reminders {
			rows = append(rows, walkRow{
				entity: entityReminder, entityID: reminder.Value.ID, containerID: reminder.CollectionID,
				payload: workservice.ReminderSnapshot(reminder.Value), key: reminder.Value.ID.String(),
			})
		}
		return rows, nil
	case entityRecurrence:
		rules, err := p.Snapshot.Recurrences(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(rules))
		for _, rule := range rules {
			rows = append(rows, walkRow{
				entity: entityRecurrence, entityID: rule.Value.ID, containerID: rule.CollectionID,
				payload: workservice.RecurrenceSnapshot(rule.Value), key: rule.Value.ID.String(),
			})
		}
		return rows, nil
	case entityTemplate:
		templates, err := p.Snapshot.Templates(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(templates))
		for _, template := range templates {
			rows = append(rows, walkRow{
				entity: entityTemplate, entityID: template.ID, containerID: template.ScopeID,
				payload: workservice.TemplateSnapshot(template), key: template.ID.String(),
			})
		}
		return rows, nil
	}
	return nil, shared.ErrValidation.WithDetail("sync.cursor_invalid")
}

// setRow is one tag row as a record of its entry: the same shape a delta's set change carries
// (AddLabel.recordChange), with the tag that decides the element as the record's clock. A present
// element is an addition under its add tag; an absent one a removal under its remove tag, so that
// a device merging a later re-add has the removal to compare it against.
func setRow(element repository.ItemSetElement) walkRow {
	operation, tag := "add", element.Element.AddedAt
	if !element.Element.IsPresent() {
		operation, tag = "remove", element.Element.RemovedAt
	}
	return walkRow{
		entity: entityItem, entityID: element.ItemID, containerID: element.CollectionID, hlc: tag,
		payload: map[string]any{
			"set":        string(element.Set),
			"element_id": element.Element.ElementID.String(),
			"op":         operation,
		},
		key: strings.Join([]string{
			element.ItemID.String(), string(element.Set), element.Element.ElementID.String(),
		}, setKeySeparator),
	}
}

// parseSetKey reads a set element's key back out of the cursor. Empty is the start.
func parseSetKey(after string) (repository.SetElementKey, error) {
	if after == "" {
		return repository.SetElementKey{}, nil
	}
	parts := strings.Split(after, setKeySeparator)
	if len(parts) != 3 {
		return repository.SetElementKey{}, shared.ErrValidation.WithDetail("sync.cursor_invalid")
	}
	return repository.SetElementKey{
		ItemID: shared.ID(parts[0]), Set: work.SetName(parts[1]), ElementID: shared.ID(parts[2]),
	}, nil
}

func indexOfKind(kind string) int {
	for i, known := range walkKinds {
		if known == kind {
			return i
		}
	}
	return -1
}
