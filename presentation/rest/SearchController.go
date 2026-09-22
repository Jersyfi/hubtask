// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	searchItemsUseCase   = "SearchItems"
	reindexSearchUseCase = "ReindexSearch"
)

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

	// Read twice, as the query is: once into the generated type for everything the contract names
	// by field, and once as a document for the two parts whose grammar is the domain's — the filter
	// and the sort travel as they arrived (ADR-0064, and `decodeQuery`'s own reasoning).
	var body openapi.ItemSearchQuery
	document, err := decodeSearch(r, &body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), searchItemsUseCase, actor, searchInput(body, document))
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
func searchInput(body openapi.ItemSearchQuery, document map[string]any) usecase.Input {
	in := usecase.Input{
		// Absent is the empty request the domain decides about, not a nil the descriptor would
		// refuse: `q` is optional beside a filter, and which of the two is missing is one question
		// asked in one place (ADR-0064).
		"q":                stringOrEmpty(body.Q),
		"container_id":     optionalUUIDField(body.ContainerId),
		"language":         optionalStringField(body.Language),
		"mode":             searchModeField(body.Mode),
		"include_archived": optionalBoolField(body.IncludeArchived),
		"include_trashed":  optionalBoolField(body.IncludeTrashed),
	}
	if body.Page != nil {
		in["cursor"] = optionalStringField(body.Page.Cursor)
		in["size"] = optionalIntField(body.Page.Size)
	}

	// As they arrived, for the reason the query passes its own that way: the grammar that reads
	// them is in the domain, so this layer neither validates nor reshapes them.
	for _, part := range []string{"filter", "sort"} {
		if value, present := document[part]; present && value != nil {
			in[part] = value
		}
	}
	return in
}

// decodeSearch reads the request twice, the way `decodeQuery` does and for the same reason.
func decodeSearch(r *http.Request, into *openapi.ItemSearchQuery) (map[string]any, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, shared.ErrMalformedRequest.WithDetail("request.body_unreadable").WithCause(err)
	}
	if err := decodeFrom(bytes.NewReader(raw), into); err != nil {
		return nil, err
	}

	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, shared.ErrMalformedRequest.WithDetail("request.body_malformed").WithCause(err)
	}
	return document, nil
}

// searchModeField passes the mode through as text, unvalidated.
//
// The enum is checked in the use case rather than here, for the reason nothing else is defaulted
// here: a value this layer refused would be a value MCP and automation still accepted, and the
// three channels have to answer the same question the same way (ADR-0005).
func searchModeField(mode *openapi.SearchMode) any {
	if mode == nil {
		return nil
	}
	return string(*mode)
}

// ReindexSearch answers POST /search:reindex (M-09): the job to watch, and how many rows it
// will rewrite.
func (c *RestController) ReindexSearch(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), reindexSearchUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusAccepted, out)
}
