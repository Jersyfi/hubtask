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

// The two directions of completion (B-07). Both hold no rules: the guards, the roll-up, the events and the
// audit entries all happen in the application layer, once, whichever channel the call arrived through
// (ADR-0005, arc42 §4).

const (
	completeWorkItemUseCase = "CompleteWorkItem"
	reopenWorkItemUseCase   = "ReopenWorkItem"
)

// CompleteWorkItem answers POST /items/{itemId}:complete.
//
// The body is optional in the contract and carries one field. It is decoded when it is there and its
// absence is not an error - which is why this does not go through decodeJSON, whose job is to refuse a
// body that is malformed rather than to tolerate one that is missing.
func (c *RestController) CompleteWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, _ openapi.CompleteWorkItemParams,
) {
	in := usecase.Input{"item_id": itemID.String()}

	if r.ContentLength > 0 {
		var body openapi.CompleteWorkItemJSONBody
		if err := decodeJSON(r, &body); err != nil {
			WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
			return
		}
		// Passed on only when the client sent it, so that the catalogue refuses the value it cannot serve
		// and accepts the default the contract documents.
		if body.CascadeChildren != nil {
			in["cascade_children"] = *body.CascadeChildren
		}
	}

	c.completion(w, r, completeWorkItemUseCase, in)
}

// ReopenWorkItem answers POST /items/{itemId}:reopen. No body: there is nothing to say beyond which item.
func (c *RestController) ReopenWorkItem(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, _ openapi.ReopenWorkItemParams,
) {
	c.completion(w, r, reopenWorkItemUseCase, usecase.Input{"item_id": itemID.String()})
}

// completion runs one direction and writes the item back.
//
// The ETag is the version after the change, which is what a client needs in order to follow the completion
// with an edit (api-guidelines.md §5). 200 rather than 204 even though the caller knows what it asked for:
// a roll-up may have moved the version by more than one, and the body is where the client learns the state
// it now has.
func (c *RestController) completion(
	w http.ResponseWriter, r *http.Request, name string, in usecase.Input,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	out, err := c.UseCases.Invoke(r.Context(), name, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, workItemResponse(out))
}
