// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// NewItemCreated announces a task, a work package or an activity that now exists.
//
// One event for all three levels rather than one per level. A subscriber that wanted only
// activities filters on the payload's `type`, and the day a fifth level is configured it reaches
// that subscriber without anybody adding a subscription for an event name nobody had heard of
// (ADR-0006, domain-model.md §4).
//
// The payload is a snapshot, in the API's field names, for the reasons given at
// NewContainerCreated: a consumer that had to fetch the item would produce a request per event
// and read a state that has already moved on, and a webhook payload should describe the object in
// the same words a REST response does.
//
// `parent_id` is sent even when it is null - it is the `parentRef` domain-model.md §4 asks for,
// and a consumer reading it to place the item in a tree needs the field present rather than
// inferred from the type. `collection_id` travels beside it because the parent alone does not say
// which collection an item belongs to until the whole chain has been walked.
func NewItemCreated(id shared.ID, item work.WorkItem, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	payload := map[string]any{
		"id":            item.ID.String(),
		"type":          string(item.Type),
		"collection_id": item.CollectionID.String(),
		"parent_id":     nil,
		"path":          item.Path,
		"depth":         item.Depth,
		"title":         item.Title,
		"completion":    completionPayload(item.Completion),
		"order_key":     item.OrderKey,
		"created_at":    item.CreatedAt.UTC(),
		"created_by":    item.CreatedBy.String(),
		"version":       item.Version,
	}
	if !item.ParentID.IsZero() {
		payload["parent_id"] = item.ParentID.String()
	}
	if item.Notes != "" {
		payload["notes"] = item.Notes
	}

	return NewEnvelope(id, ItemCreated, item.TenantID,
		ItemSubject(item.ID), actor, occurredAt, cause, payload)
}

// completionPayload is the done/open state as the API spells it (schema Completion).
//
// Always sent, even on a create where it is always open. An automation rule that reacts to items
// and reads `completion.is_completed` should not need a special case for the one event where the
// field happens to be absent.
func completionPayload(completion work.Completion) map[string]any {
	payload := map[string]any{
		"is_completed": completion.IsCompleted,
		"completed_at": nil,
		"completed_by": nil,
	}
	if completion.CompletedAt != nil {
		payload["completed_at"] = completion.CompletedAt.UTC()
	}
	if !completion.CompletedBy.IsZero() {
		payload["completed_by"] = completion.CompletedBy.String()
	}
	return payload
}

// ItemSubject is what an item event is about. Kept next to the event so that the two cannot
// drift: a consumer filtering on the subject and a producer writing it read the same line.
func ItemSubject(id shared.ID) string { return "item/" + id.String() }
