// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

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
func (c *RestController) MoveWorkItem(w http.ResponseWriter, r *http.Request, itemID openapi.ItemId) {
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
	if present["target_parent_id"] {
		in["target_parent_id"] = optionalUUIDField(body.TargetParentId)
	}
	if present["target_collection_id"] {
		in["target_collection_id"] = optionalUUIDField(body.TargetCollectionId)
	}
	if present["before_item_id"] {
		in["before_item_id"] = optionalUUIDField(body.BeforeItemId)
	}
	if present["target_bucket_id"] {
		// Passed on so the catalogue refuses it by name. Buckets arrive with B-09, and a client that moved a
		// card into one and received a 200 would believe the card is in that bucket.
		in["target_bucket_id"] = optionalUUIDField(body.TargetBucketId)
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

	// An empty array rather than absent or null. It is empty because nothing yet can fail to resolve in a new
	// collection (I-W6), and it is an array because a client that iterates it should not have to nil-check the
	// field the day it fills.
	dropped := []openapi.DroppedReference{}
	if references, ok := out["dropped_references"].([]usecase.Output); ok {
		for _, reference := range references {
			dropped = append(dropped, openapi.DroppedReference{
				Kind: openapi.DroppedReferenceKind(reference.String("kind")),
				Id:   reference.String("id"),
				Code: reference.String("code"),
			})
		}
	}

	return item, openapi.MoveResult{Item: item, DroppedReferences: dropped}
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
