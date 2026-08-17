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
	payload := map[string]any{
		"id":         container.ID.String(),
		"type":       string(container.Type),
		"parent_id":  nil,
		"name":       container.Name,
		"order_key":  container.OrderKey,
		"created_at": container.CreatedAt.UTC(),
		"created_by": container.CreatedBy.String(),
		"version":    container.Version,
	}
	if !container.ParentID.IsZero() {
		payload["parent_id"] = container.ParentID.String()
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

	return NewEnvelope(id, ContainerCreated, container.TenantID,
		ContainerSubject(container.ID), actor, occurredAt, cause, payload)
}

// ContainerSubject is what a container event is about. Kept next to the event so that the two
// cannot drift: a consumer filtering on the subject and a producer writing it read the same line.
func ContainerSubject(id shared.ID) string { return "container/" + id.String() }
