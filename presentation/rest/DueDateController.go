// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// When an entry is due (D-01). A sub-resource beside the cover's, and additionally the writer the
// merge patch dispatches into: the contract has carried the three due fields on the create and
// update schemas since 0.1.0, so both doors serve them - through the one use case pair, so a due
// date means the same thing whichever way it arrived.

const (
	setDueDateUseCase   = "SetDueDate"
	clearDueDateUseCase = "ClearDueDate"
)

// SetDueDate answers PUT /items/{itemId}/due.
func (c *RestController) SetDueDate(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, params openapi.SetDueDateParams,
) {
	var body openapi.DueDateInput
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}

	in := usecase.Input{"item_id": itemID.String(), "due_at": body.DueAt.Format(time.RFC3339Nano)}
	if body.DueDateOnly != nil {
		in["due_date_only"] = *body.DueDateOnly
	}
	if body.DueTimeZone != nil {
		in["due_time_zone"] = *body.DueTimeZone
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}
	c.dueDate(w, r, setDueDateUseCase, in)
}

// ClearDueDate answers DELETE /items/{itemId}/due. No body: taking the due date off names nothing
// beyond which entry - the instant, the flag and the zone leave together.
func (c *RestController) ClearDueDate(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, params openapi.ClearDueDateParams,
) {
	in := usecase.Input{"item_id": itemID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}
	c.dueDate(w, r, clearDueDateUseCase, in)
}

// dueDate runs one direction and writes the entry back.
//
// The ETag is the version after the change, which is what a client needs in order to follow the
// due date with an edit (api-guidelines.md §5). 200 with the entry rather than 204: the caller
// knows what it asked for, and not what version that produced.
func (c *RestController) dueDate(
	w http.ResponseWriter, r *http.Request, name string, in usecase.Input,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), name, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, workItemResponse(out))
}
