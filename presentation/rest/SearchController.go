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

const searchItemsUseCase = "SearchItems"

// SearchItems answers POST /search (C-08).
//
// A POST that reads, exactly as the query is: the request is a document, and a URL long enough to
// carry a search phrase and a scope is a URL a proxy truncates. Nothing is written and the same
// request may be repeated (api-guidelines.md §3).
func (c *RestController) SearchItems(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	var body openapi.ItemSearchQuery
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), searchItemsUseCase, actor, searchInput(body))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	page := openapi.WorkItemPage{Data: []openapi.WorkItem{}, Page: pageResponse(out)}
	for _, row := range rowsOf(out) {
		page.Data = append(page.Data, workItemResponse(row))
	}
	writeJSON(w, r, http.StatusOK, page)
}

// searchInput maps the request onto the catalogue's input.
//
// Nothing is defaulted here. The language a caller did not state is the actor's locale, which this
// layer does not resolve, and the page size a caller did not state is the contract's default - both
// are the use case's to decide, so that the MCP and automation channels get the same answer
// (ADR-0005).
func searchInput(body openapi.ItemSearchQuery) usecase.Input {
	in := usecase.Input{
		"q":                body.Q,
		"container_id":     optionalUUIDField(body.ContainerId),
		"language":         optionalStringField(body.Language),
		"include_archived": optionalBoolField(body.IncludeArchived),
		"include_trashed":  optionalBoolField(body.IncludeTrashed),
	}
	if body.Page != nil {
		in["cursor"] = optionalStringField(body.Page.Cursor)
		in["size"] = optionalIntField(body.Page.Size)
	}
	return in
}
