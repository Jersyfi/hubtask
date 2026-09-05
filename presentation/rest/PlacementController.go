// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/google/uuid"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Moving and reordering (B-08). Neither holds a rule: the cycle check, the re-checked placement, the rank and
// the subtree rewrite all happen in the application and domain layers, once, whichever channel the call came
// through (ADR-0005, arc42 §4).

const (
	moveWorkItemUseCase    = "MoveWorkItem"
	reorderWorkItemUseCase = "ReorderWorkItem"
)

// MoveWorkItem answers POST /items/{itemId}:move.
func (c *RestController) MoveWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, _ openapi.MoveWorkItemParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.MoveWorkItemJSONBody
	present, err := decodeJSONWithPresence(r, &body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String()}
	// Only the fields the client sent. `target_parent_id` in particular: sending it as null asks for the top
	// level, and omitting it asks for the parent to stay - a handler that passed nil for both would move items
	// nobody asked to move, and the value alone cannot tell the two apart.
	//
	// Null reaches the catalogue as the empty string rather than as nil, which is what `WorkItemController`
	// does for the same distinction. `usecase.Input.Present` reports a present-but-nil entry as absent, by
	// design and with a test on it - so a nil here would spell "the caller said nothing" for a caller who
	// said null, and the use case would leave the parent exactly where the request asked it not to be.
	if present["target_parent_id"] {
		in["target_parent_id"] = uuidOrEmpty(body.TargetParentId)
	}
	if present["target_collection_id"] {
		in["target_collection_id"] = uuidOrEmpty(body.TargetCollectionId)
	}
	if present["before_item_id"] {
		in["before_item_id"] = uuidOrEmpty(body.BeforeItemId)
	}
	if present["target_bucket_id"] {
		// The same shape, and the reason the board needs it: null takes the entry off the board and omitting
		// the field leaves the column where the collection allows.
		in["target_bucket_id"] = uuidOrEmpty(body.TargetBucketId)
	}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), moveWorkItemUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	item, result := moveResultResponse(out)
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, r, http.StatusOK, result)
}

// ReorderWorkItem answers POST /items/{itemId}:reorder.
func (c *RestController) ReorderWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, _ openapi.ReorderWorkItemParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String()}

	// The body is optional: no body at all means "move it to the end", which is a position like any other.
	if r.ContentLength > 0 {
		var body openapi.ReorderWorkItemJSONBody
		if err := decodeJSON(r, &body); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
		if body.BeforeItemId != nil {
			in["before_item_id"] = body.BeforeItemId.String()
		}
	}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), reorderWorkItemUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	item := workItemResponse(out)
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, item)
}

// moveResultResponse maps the catalogue's answer onto the contract's MoveResult.
//
// `dropped_references` is an empty array rather than absent or null. It is empty because nothing yet can fail
// to resolve in a new collection (I-W6), and it is an array because a client that iterates it should not have
// to nil-check the field the day it fills.
func moveResultResponse(out usecase.Output) (openapi.WorkItem, openapi.MoveResult) {
	nested, _ := out["item"].(usecase.Output)
	item := workItemResponse(nested)

	return item, openapi.MoveResult{Item: item, DroppedReferences: droppedReferencesResponse(out)}
}

// droppedReferencesResponse maps what an operation could not carry over (I-W6). Shared by the move
// and the copy, which report the same losses in the same shape and would otherwise be two mappings
// of one answer.
//
// An empty array rather than absent or null: a client that iterates the losses should not have to
// nil-check the field on the operations that happen to lose nothing.
func droppedReferencesResponse(out usecase.Output) []openapi.DroppedReference {
	dropped := []openapi.DroppedReference{}

	references, ok := out["dropped_references"].([]usecase.Output)
	if !ok {
		return dropped
	}
	for _, reference := range references {
		entry := openapi.DroppedReference{
			Kind: openapi.DroppedReferenceKind(reference.String("kind")),
			Id:   reference.String("id"),
			Code: reference.String("code"),
		}
		if itemID, err := uuid.Parse(reference.String("item_id")); err == nil {
			// The entry that lost it. Absent rather than zero when the answer did not name one: a
			// null identifier would read as an entry, and no entry has that identifier.
			entry.ItemId = &itemID
		}
		dropped = append(dropped, entry)
	}
	return dropped
}

// ifMatchOf reads the header the generated parameters of these two operations do not carry.
//
// `:move` and `:reorder` are actions, and api-guidelines.md §5 gives actions `Idempotency-Key` rather than
// `If-Match`, so the contract declares no If-Match parameter for them. Reading it when a client sends it
// anyway costs nothing and makes the optimistic lock available where a client wants it; a client that sends
// none is unaffected.
func ifMatchOf(r *http.Request) *string {
	value := r.Header.Get("If-Match")
	if value == "" {
		return nil
	}
	return &value
}

// actorOf is the actor the middleware put on the context. Absent is the zero actor, which every use case
// refuses as unauthenticated - the check belongs there and not here (ADR-0005).
func actorOf(r *http.Request) appshared.ActorContext {
	actor, _ := appshared.ActorFrom(r.Context())
	return actor
}
