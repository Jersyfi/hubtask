// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const listActivityUseCase = "ListActivity"

// ListActivity answers GET /items/{itemId}/activity.
//
// No ETag, for the reason no list has one: a page is not an entity with a version, and there is
// nothing here for a client to write back against - the history is append-only and nothing writes
// to it through this endpoint.
func (c *RestController) ListActivity(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, params openapi.ListActivityParams,
) {
	out, ok := c.read(w, r, listActivityUseCase, usecase.Input{
		"item_id": itemID.String(),
		"cursor":  optionalStringField(params.Cursor),
		"size":    optionalIntField(params.Size),
	})
	if !ok {
		return
	}

	page := openapi.ActivityPage{Data: []openapi.ActivityEntry{}, Page: pageResponse(out)}
	for _, row := range rowsOf(out) {
		page.Data = append(page.Data, activityEntryResponse(row))
	}
	writeJSON(w, r, http.StatusOK, page)
}

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
