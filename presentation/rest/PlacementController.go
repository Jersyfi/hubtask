// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/google/uuid"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Moving and reordering. Neither holds a rule: the cycle check, the re-checked placement, the rank and
// the subtree rewrite all happen in the application and domain layers, once, whichever channel the call came
// through (ADR-0005, arc42 §4).

// moveResultAnswer draws a move's answer: the entry where it now is, and what it could not carry.
func moveResultAnswer(out usecase.Output) openapi.MoveResult {
	_, result := moveResultResponse(out)
	return result
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
