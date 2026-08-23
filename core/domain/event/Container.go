// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// NewContainerCreated announces a hub or a collection that now exists.
//
// The payload is a snapshot rather than a reference (domain-model.md §4). A consumer that would
// have to fetch the container in order to react is a consumer that produces a request per event
// and sees a state that has already moved on - and for a deletion event there would be nothing
// left to fetch at all.
//
// The field names are the API's, `snake_case` (api-guidelines.md §1), so that a webhook payload
// and a REST response describe the same object in the same words. Optional fields that are unset
// are left out rather than sent as null: a client tolerates unknown fields, and an absent one
// says "not set" just as clearly with fewer bytes. `parent_id` is the exception - a hub has no
// parent, and saying so explicitly is what tells a consumer which level it is looking at without
// interpreting the type.
func NewContainerCreated(id shared.ID, container work.Container, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return NewEnvelope(id, ContainerCreated, container.TenantID,
		ContainerSubject(container.ID), actor, occurredAt, cause, containerPayload(container))
}

// NewContainerRenamed announces that a container's own descriptive fields changed.
//
// A snapshot, as domain-model.md §4 gives every container event, and the change set beside it. Both,
// because they answer different questions: the snapshot is what the container now is, and the change
// set is what a field change trigger is written against - a rule fires on "the name became X" or on
// "it stopped being Y", and only the second needs the value that went.
//
// A container carries no field of the size that made ItemUpdated drop its snapshot; the largest
// thing here is a name of at most 200 characters, which is already in the snapshot every other
// container event carries.
func NewContainerRenamed(id shared.ID, container work.Container, changes []work.FieldChange,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newContainerChange(id, ContainerRenamed, container, changes, actor, occurredAt, cause)
}

// NewContainerPoliciesUpdated announces that a collection works differently now.
func NewContainerPoliciesUpdated(id shared.ID, container work.Container, changes []work.FieldChange,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newContainerChange(id, ContainerPoliciesUpdated, container, changes, actor, occurredAt, cause)
}

func newContainerChange(id shared.ID, eventType Type, container work.Container,
	changes []work.FieldChange, actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if len(changes) == 0 {
		// An event announcing that nothing changed. The writer does not write when nothing moved, so
		// reaching this means the two disagree - a defect rather than something a client sent
		// (security.md §9).
		return Envelope{}, shared.ErrInternal.WithDetail("events.change_set_empty")
	}

	changeSet := make(map[string]any, len(changes))
	for _, change := range changes {
		changeSet[change.Field] = map[string]any{"from": change.From, "to": change.To}
	}

	payload := containerPayload(container)
	payload["change_set"] = changeSet
	return NewEnvelope(id, eventType, container.TenantID,
		ContainerSubject(container.ID), actor, occurredAt, cause, payload)
}

// NewContainerMoved announces that a collection sits in a different hub, or at a different rank in
// the same one.
//
// `from_parent_id` travels beside the snapshot for the reason ItemMoved's does: a consumer that
// cares only about reparenting compares it with `parent_id`, and one that cares about the order
// reads `order_key`. Equal identifiers mean a reorder.
func NewContainerMoved(id shared.ID, container work.Container, fromParentID shared.ID,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	payload := containerPayload(container)
	payload["from_parent_id"] = nil
	if !fromParentID.IsZero() {
		payload["from_parent_id"] = fromParentID.String()
	}

	return NewEnvelope(id, ContainerMoved, container.TenantID,
		ContainerSubject(container.ID), actor, occurredAt, cause, payload)
}

// NewContainerArchived announces that a container is read-only, and everything under it with it.
func NewContainerArchived(id shared.ID, container work.Container, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return NewEnvelope(id, ContainerArchived, container.TenantID,
		ContainerSubject(container.ID), actor, occurredAt, cause, containerPayload(container))
}

// NewContainerUnarchived announces that a container is writable again.
func NewContainerUnarchived(id shared.ID, container work.Container, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return NewEnvelope(id, ContainerUnarchived, container.TenantID,
		ContainerSubject(container.ID), actor, occurredAt, cause, containerPayload(container))
}

// containerPayload is the snapshot every container event carries, in the API's field names, so that
// a webhook payload and a REST response describe the same object in the same words.
//
// One builder rather than one per event. Copies of this map would drift in the way that matters
// least to whoever changes one and most to a subscriber reading them all - which is why the created
// event, which used to build its own, reads this one too.
//
// `effective_archived` is here because a subscriber cannot derive it: a collection is read-only when
// its hub is archived, and nothing in its own row says so. `archived_at` says which of the two it
// is, so a rule can tell "this was archived" from "the hub above it was".
func containerPayload(container work.Container) map[string]any {
	payload := map[string]any{
		"id":                 container.ID.String(),
		"type":               string(container.Type),
		"parent_id":          nil,
		"name":               container.Name,
		"order_key":          container.OrderKey,
		"completion_policy":  string(container.CompletionPolicy.OrDefault()),
		"auto_assign":        autoAssignPayload(container.AutoAssign),
		"archived_at":        nil,
		"effective_archived": container.IsEffectivelyArchived(),
		"created_at":         container.CreatedAt.UTC(),
		"created_by":         container.CreatedBy.String(),
		"updated_at":         container.UpdatedAt.UTC(),
		"version":            container.Version,
	}
	if !container.ParentID.IsZero() {
		payload["parent_id"] = container.ParentID.String()
	}
	if container.ArchivedAt != nil {
		payload["archived_at"] = container.ArchivedAt.UTC()
	}
	for field, value := range map[string]string{
		"description": container.Description,
		"icon":        container.Icon,
		"color_token": container.ColorToken,
	} {
		if value != "" {
			payload[field] = value
		}
	}
	return payload
}

// autoAssignPayload is the auto_assign key as the snapshot carries it: null for no policy, and
// the document without the rotation state - the state is the server's bookkeeping, and an event
// that carried it would announce every assignment as a configuration change.
func autoAssignPayload(policy *work.AutoAssignDefinition) any {
	if policy == nil {
		return nil
	}
	candidates := make([]any, 0, len(policy.Candidates))
	for _, candidate := range policy.Candidates {
		candidates = append(candidates, map[string]any{
			"kind": string(candidate.Kind), "id": candidate.ID.String(),
		})
	}
	return map[string]any{
		"strategy":   string(policy.Strategy),
		"candidates": candidates,
		"enabled":    policy.Enabled,
	}
}

// ContainerSubject is what a container event is about. Kept next to the event so that the two
// cannot drift: a consumer filtering on the subject and a producer writing it read the same line.
func ContainerSubject(id shared.ID) string { return "container/" + id.String() }

// NewContainerDeleted announces that a container and everything under it are in the trash.
//
// The batch is in the payload because it is what makes the deletion one thing: a consumer that
// holds the subtree removes it whole, and a restore of that batch is what will bring it back. The
// counts say how much went - a client cannot derive them, because nothing below the container gets
// an event of its own.
func NewContainerDeleted(id shared.ID, container work.Container, cascade Cascade, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if container.DeletedAt == nil {
		// The event would say the opposite of its own name. A defect in whatever built it, not
		// something a client did (security.md §9).
		return Envelope{}, shared.ErrInternal.WithDetail("events.lifecycle_inconsistent")
	}
	return NewEnvelope(id, ContainerDeleted, container.TenantID,
		ContainerSubject(container.ID), actor, occurredAt, cause,
		trashPayload(container, cascade))
}

// NewContainerRestored announces that a container's deletion has been reversed, whole.
func NewContainerRestored(id shared.ID, container work.Container, cascade Cascade, actor Actor,
	occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if container.DeletedAt != nil {
		return Envelope{}, shared.ErrInternal.WithDetail("events.lifecycle_inconsistent")
	}
	return NewEnvelope(id, ContainerRestored, container.TenantID,
		ContainerSubject(container.ID), actor, occurredAt, cause,
		trashPayload(container, cascade))
}

// Cascade is how much a container's deletion or restore took with it.
//
// Counts rather than identifiers. A consumer reacting to the deletion of a hub does not need a list
// of two hundred entries in order to drop what it holds below it - it has the container - and an
// event whose size grew with the subtree would eventually be an event nobody can deliver.
type Cascade struct {
	Collections int
	Items       int
}

// trashPayload is the container snapshot with the deletion's own fields on it.
func trashPayload(container work.Container, cascade Cascade) map[string]any {
	payload := containerPayload(container)
	payload["deleted_at"] = nil
	payload["trash_batch_id"] = nil
	if container.DeletedAt != nil {
		payload["deleted_at"] = container.DeletedAt.UTC()
	}
	if !container.TrashBatchID.IsZero() {
		payload["trash_batch_id"] = container.TrashBatchID.String()
	}
	payload["collections"] = cascade.Collections
	payload["items"] = cascade.Items
	return payload
}
