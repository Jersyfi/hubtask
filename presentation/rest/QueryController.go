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

const queryItemsUseCase = "QueryItems"

// QueryItems answers POST /items:query (B-12).
//
// A POST that reads: the query is a document rather than a set of parameters, and a URL long
// enough to carry a filter tree is a URL a proxy truncates. Nothing is written, and the same
// request may be repeated (api-guidelines.md §3).
func (c *RestController) QueryItems(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	var body openapi.ItemQuery
	document, err := decodeQuery(r, &body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	labels, err := expansionsOf(body.Expand)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), queryItemsUseCase, actor, queryInput(body, document, labels))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, queryResponse(out))
}

// decodeQuery reads the body twice from the same bytes: once into the generated type, so that an
// unknown field is refused and the scalars are typed, and once untyped, so that the parts of the
// request that are a grammar rather than a field list reach the use case as the documents they are.
//
// The two passes cannot disagree, because the typed one has already established that this is an
// object whose keys are known.
func decodeQuery(r *http.Request, into *openapi.ItemQuery) (map[string]any, error) {
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

// queryInput maps the request onto the catalogue's input.
//
// The filter, the sort and the grouping travel as they arrived. The grammar that reads them lives
// in the domain (core/domain/model/view), so this layer neither validates nor reshapes them - a
// second reading here would be a second place for the grammar to be wrong, and the MCP and
// automation channels would not go through it (ADR-0005, ADR-0026).
func queryInput(body openapi.ItemQuery, document map[string]any, labels bool) usecase.Input {
	in := usecase.Input{
		"scope_container_id": optionalUUIDField(body.Scope.ContainerId),
		"scope_item_id":      optionalUUIDField(body.Scope.ItemId),
		"include_archived":   optionalBoolField(body.IncludeArchived),
		"include_trashed":    optionalBoolField(body.IncludeTrashed),
		"expand_labels":      labels,
	}
	if body.Scope.IncludeDescendants != nil {
		in["include_descendants"] = *body.Scope.IncludeDescendants
	}
	if body.Page != nil {
		in["cursor"] = optionalStringField(body.Page.Cursor)
		in["size"] = optionalIntField(body.Page.Size)
	}
	if body.Count != nil {
		in["count"] = string(*body.Count)
	}

	for _, part := range []string{"filter", "sort", "group_by"} {
		if value, present := document[part]; present && value != nil {
			in[part] = value
		}
	}
	return in
}

// queryResponse maps the catalogue's answer onto the contract's shape.
//
// Both `data` and `groups` are always arrays, never null, whichever shape the query took: the
// contract says both are present, and a client that had to check for null before iterating would
// be checking on every response for a case that only arises in one of them.
func queryResponse(out usecase.Output) openapi.ItemQueryResult {
	result := openapi.ItemQueryResult{
		Data:   []openapi.WorkItem{},
		Groups: []openapi.ItemQueryGroup{},
		Page:   pageResponse(out),
		Total:  optionalCount(out["total"]),
	}
	for _, row := range rowsOf(out) {
		result.Data = append(result.Data, workItemResponse(row))
	}

	groups, _ := out["groups"].([]usecase.Output)
	for _, group := range groups {
		rendered := openapi.ItemQueryGroup{
			Data:  []openapi.WorkItem{},
			Page:  pageResponse(group),
			Count: optionalCount(group["count"]),
		}
		if key, ok := group["key"].(string); ok {
			rendered.Key = &key
		}
		for _, row := range rowsOf(group) {
			rendered.Data = append(rendered.Data, workItemResponse(row))
		}
		result.Groups = append(result.Groups, rendered)
	}
	return result
}

// optionalCount renders a total that was asked for, and an explicit null for one that was not. The
// field has no omitempty in the generated type, so "not counted" is a null a client can read
// unconditionally rather than a missing key.
func optionalCount(value any) *int {
	switch count := value.(type) {
	case int:
		return &count
	case int64:
		converted := int(count)
		return &converted
	case float64:
		converted := int(count)
		return &converted
	}
	return nil
}
