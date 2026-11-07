// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// activityEntryResponse maps one step of a history.
//
// The change set is passed through as the application layer built it. It is the one place in this
// adapter where an untyped object reaches the wire on purpose: the fields a step names are the
// fields the domain moved, and a shape declared per field here would be a second copy of the domain
// that has to be extended every time a field is added to it (api/openapi.yaml, ActivityEntry).
func activityEntryResponse(out usecase.Output) openapi.ActivityEntry {
	entry := openapi.ActivityEntry{
		Id:         uuidValue(out.String("id")),
		ItemId:     uuidValue(out.String("item_id")),
		Code:       out.String("code"),
		OccurredAt: timeValue(out["occurred_at"]),
		ChangeSet:  map[string]any{},
	}
	if changeSet, ok := out["change_set"].(map[string]any); ok {
		entry.ChangeSet = changeSet
	}

	actor, _ := out["actor"].(map[string]any)
	kind, _ := actor["type"].(string)
	entry.Actor.Type = openapi.ActivityEntryActorType(kind)
	if id, ok := actor["id"].(string); ok {
		entry.Actor.Id = optionalUUIDResponse(id)
	}
	return entry
}
