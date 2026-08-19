// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The labels an entry carries (B-09). A sub-resource with PUT and DELETE rather than a field on the
// entry, because a set is not a field: two devices adding two different labels at once is the case
// the OR-set exists to serve, and a merge patch carrying the whole array would let the later of the
// two erase the other's (offline-sync.md §4.2).

const (
	addLabelUseCase    = "AddLabel"
	removeLabelUseCase = "RemoveLabel"
)

// AddLabel answers PUT /items/{itemId}/labels/{labelId}.
func (c *RestController) AddLabel(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, labelID openapi.LabelId,
) {
	c.itemLabel(w, r, addLabelUseCase, itemID, labelID)
}

// RemoveLabel answers DELETE /items/{itemId}/labels/{labelId}.
func (c *RestController) RemoveLabel(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, labelID openapi.LabelId,
) {
	c.itemLabel(w, r, removeLabelUseCase, itemID, labelID)
}

// itemLabel is what the two share: both name the same pair in the path, take no body, and answer
// with the set the entry now carries.
//
// No ETag. Neither operation touches the entry's own row, so there is no version of the entry to
// write against - and a header saying otherwise would invite a client to send an If-Match that
// nothing here honours (api-guidelines.md §5).
func (c *RestController) itemLabel(
	w http.ResponseWriter, r *http.Request, name string,
	itemID openapi.ItemId, labelID openapi.LabelId,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	out, err := c.UseCases.Invoke(r.Context(), name, actor, usecase.Input{
		"item_id":  itemID.String(),
		"label_id": labelID.String(),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, itemLabelsResponse(out))
}

// itemLabelsResponse maps the catalogue's answer onto the contract's ItemLabels. The array is empty
// rather than null when the entry carries nothing: a client that iterates it should not have to
// nil-check the field.
func itemLabelsResponse(out usecase.Output) openapi.ItemLabels {
	set := openapi.ItemLabels{
		ItemId:   uuidValue(out.String("item_id")),
		LabelIds: []openapi_types.UUID{},
	}
	ids, _ := out["label_ids"].([]string)
	for _, id := range ids {
		set.LabelIds = append(set.LabelIds, uuidValue(id))
	}
	return set
}
