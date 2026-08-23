// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The files an entry carries (C-06). A sub-resource rather than a field of the entry, because it is
// a set: it merges as an OR-set, it lives beside the row, and neither direction spends the entry's
// version. The same split the label and member controllers draw, and for the same reason
// (offline-sync.md §4.2).

const (
	attachMediaUseCase     = "AttachMedia"
	detachMediaUseCase     = "DetachMedia"
	listAttachmentsUseCase = "ListAttachments"
)

// ListAttachments answers GET /items/{itemId}/attachments.
func (c *RestController) ListAttachments(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	params openapi.ListAttachmentsParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String()}
	if params.Cursor != nil {
		in["cursor"] = *params.Cursor
	}
	if params.Size != nil {
		in["size"] = *params.Size
	}

	out, err := c.UseCases.Invoke(r.Context(), listAttachmentsUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	page := openapi.MediaPage{Data: []openapi.MediaObject{}, Page: pageResponse(out)}
	for _, row := range rowsOf(out) {
		page.Data = append(page.Data, mediaResponse(row))
	}
	writeJSON(w, r, http.StatusOK, page)
}

// AttachMedia answers PUT /items/{itemId}/attachments/{mediaId}.
func (c *RestController) AttachMedia(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, mediaID openapi.MediaId,
) {
	c.attachment(w, r, attachMediaUseCase, itemID, mediaID)
}

// DetachMedia answers DELETE /items/{itemId}/attachments/{mediaId}.
func (c *RestController) DetachMedia(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, mediaID openapi.MediaId,
) {
	c.attachment(w, r, detachMediaUseCase, itemID, mediaID)
}

// attachment runs one direction and writes back what the entry now carries.
//
// The set rather than the entry, and no ETag: neither direction touches the entry's own row, so
// there is no version to report and none was spent. That is the same answer the member and label
// sub-resources give (api-guidelines.md §5).
func (c *RestController) attachment(
	w http.ResponseWriter, r *http.Request, name string, itemID openapi.ItemId,
	mediaID openapi.MediaId,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), name, actorOf(r), usecase.Input{
		"item_id": itemID.String(), "media_id": mediaID.String(),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, attachmentsResponse(out))
}

// attachmentsResponse maps the catalogue's projection onto the contract's schema.
func attachmentsResponse(out usecase.Output) openapi.ItemAttachments {
	ids, _ := out["media_ids"].([]string)

	carried := make([]openapi_types.UUID, 0, len(ids))
	for _, id := range ids {
		carried = append(carried, uuidValue(id))
	}
	return openapi.ItemAttachments{ItemId: uuidValue(out.String("item_id")), MediaIds: carried}
}
