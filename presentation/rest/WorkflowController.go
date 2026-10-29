// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The workflow operations are answered by the generated mapping (Doors.gen.go); taking a step is
// written here because its answer is the entry, drawn by workItemResponse as every entry is.

const takeStepUseCase = "TakeStep"

// TakeStep answers POST /items/{itemId}:step (UC-FLW-03 check 2).
func (c *RestController) TakeStep(w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, _ openapi.TakeStepParams) {
	var body openapi.TakeStepJSONBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{"item_id": itemID.String()}
	if body.Step != nil {
		in["step"] = *body.Step
	}
	if body.ToStatus != nil {
		in["to_status"] = *body.ToStatus
	}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}
	out, ok := c.read(w, r, takeStepUseCase, in)
	if !ok {
		return
	}
	item := workItemResponse(out)
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, r, http.StatusOK, item)
}
