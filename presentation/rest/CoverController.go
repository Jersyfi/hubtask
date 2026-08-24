// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// How a card presents itself (C-06). A sub-resource rather than a field of the merge patch, for
// the reason the assignee is an action route: it is one decision about one field, and writing it
// through the patch would spend a rename's version on choosing a colour.

const (
	setCoverUseCase   = "SetCover"
	clearCoverUseCase = "ClearCover"
)

// SetCover answers PUT /items/{itemId}/cover.
func (c *RestController) SetCover(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, params openapi.SetCoverParams,
) {
	var body openapi.CoverInput
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}

	in := usecase.Input{"item_id": itemID.String(), "kind": string(body.Kind)}
	if body.ColorToken != nil {
		in["color_token"] = *body.ColorToken
	}
	if body.MediaId != nil {
		in["media_id"] = body.MediaId.String()
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}
	c.cover(w, r, setCoverUseCase, in)
}

// ClearCover answers DELETE /items/{itemId}/cover. No body: taking the cover off names nothing
// beyond which entry.
func (c *RestController) ClearCover(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, params openapi.ClearCoverParams,
) {
	in := usecase.Input{"item_id": itemID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}
	c.cover(w, r, clearCoverUseCase, in)
}

// cover runs one direction and writes the entry back.
//
// The ETag is the version after the change, which is what a client needs in order to follow the
// cover with an edit (api-guidelines.md §5). 200 with the entry rather than 204: the caller knows
// what it chose, and not what version that produced.
func (c *RestController) cover(
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
