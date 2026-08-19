// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const listTrashUseCase = "ListTrash"

// ListTrash answers GET /trash.
//
// The page can be shorter than the size asked for, and that is the contract rather than a bug: the
// trash spans hubs, so it is narrowed to what the caller may see rather than refused, and the cursor
// is a boundary in what was scanned rather than in what came back. A client pages on until has_more
// is false (api-guidelines.md §4).
func (c *RestController) ListTrash(
	w http.ResponseWriter, r *http.Request, params openapi.ListTrashParams,
) {
	in := usecase.Input{
		"cursor": optionalStringField(params.Cursor),
		"size":   optionalIntField(params.Size),
	}

	out, ok := c.read(w, r, listTrashUseCase, in)
	if !ok {
		return
	}

	page := openapi.TrashPage{Data: []openapi.TrashEntry{}, Page: pageResponse(out)}
	for _, row := range rowsOf(out) {
		page.Data = append(page.Data, trashEntryResponse(row))
	}
	writeJSON(w, r, http.StatusOK, page)
}

// trashEntryResponse maps one deletion.
//
// The three optional identifiers are pointers with no omitempty in the generated type, so an entry
// that has none carries an explicit null rather than no field at all - which is what lets a client
// read them unconditionally across a list that mixes containers and entries by design.
func trashEntryResponse(out usecase.Output) openapi.TrashEntry {
	entry := openapi.TrashEntry{
		Kind:         openapi.TrashEntryKind(out.String("kind")),
		Id:           uuidValue(out.String("id")),
		TrashBatchId: uuidValue(out.String("trash_batch_id")),
		DeletedAt:    timeValue(out["deleted_at"]),
		Title:        out.String("title"),
		Subtype:      out.String("subtype"),
		Version:      out.Int("version"),
	}
	entry.HubId = optionalUUIDResponse(out.String("hub_id"))
	entry.CollectionId = optionalUUIDResponse(out.String("collection_id"))
	entry.ParentId = optionalUUIDResponse(out.String("parent_id"))
	return entry
}

// optionalUUIDResponse turns the catalogue's "not set" - an absent value, which reads back as the
// empty string - into the null the contract spells it as.
func optionalUUIDResponse(value string) *openapi_types.UUID {
	if value == "" {
		return nil
	}
	id := uuidValue(value)
	return &id
}
