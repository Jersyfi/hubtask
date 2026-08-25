// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Copying an entry (C-11). Nothing here holds a rule: what a copy carries, what the destination can
// resolve and what it therefore reports back are decided in the application layer, once, whichever
// channel the call came through (ADR-0005, arc42 §4).

const duplicateWorkItemUseCase = "DuplicateWorkItem"

// DuplicateWorkItem answers POST /items/{itemId}:duplicate.
func (c *RestController) DuplicateWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, _ openapi.DuplicateWorkItemParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String()}

	// The body is optional: no body at all means "copy this entry where it is", which is the
	// commonest copy there is.
	if r.ContentLength > 0 {
		var body openapi.DuplicateWorkItemJSONBody
		present, err := decodeJSONWithPresence(r, &body)
		if err != nil {
			WriteProblem(w, err, requestID)
			return
		}

		if body.IncludeSubtree != nil {
			in["include_subtree"] = *body.IncludeSubtree
		}
		// Presence rather than the value, for the reason a move reads it that way: sending
		// `target_parent_id` as null asks for the top level of a collection and omitting it asks
		// for the copy to land beside the original, and the difference is invisible in the value.
		if present["target_parent_id"] {
			in["target_parent_id"] = optionalUUIDField(body.TargetParentId)
		}
		if present["target_collection_id"] {
			in["target_collection_id"] = optionalUUIDField(body.TargetCollectionId)
		}
		if body.Title != nil {
			in["title"] = *body.Title
		}
	}

	out, err := c.UseCases.Invoke(r.Context(), duplicateWorkItemUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	result := duplicateResultResponse(out)
	// Location and ETag are what let a client follow up without guessing: where the copy is, and
	// which version it may write against (api-guidelines.md §5).
	w.Header().Set("Location", APIBasePath+"/items/"+result.Item.Id.String())
	w.Header().Set("ETag", etag(result.Item.Version))
	writeJSON(w, r, http.StatusCreated, result)
}

// duplicateResultResponse maps the catalogue's answer onto the contract's DuplicateResult.
func duplicateResultResponse(out usecase.Output) openapi.DuplicateResult {
	nested, _ := out["item"].(usecase.Output)

	return openapi.DuplicateResult{
		Item:              workItemResponse(nested),
		Copied:            out.Int("copied"),
		DroppedReferences: droppedReferencesResponse(out),
	}
}
