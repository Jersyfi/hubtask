// SPDX-License-Identifier: Apache-2.0
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

const aggregateItemsUseCase = "AggregateItems"

// AggregateItems answers POST /items:aggregate. The filter and the aggregation travel as they
// arrived; the grammar that reads them is the domain's (ADR-0026), as for QueryItems.
func (c *RestController) AggregateItems(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		WriteProblem(w, shared.ErrMalformedRequest.WithDetail("request.body_unreadable").WithCause(err), requestID)
		return
	}
	var body openapi.ItemAggregateQuery
	if err := decodeFrom(bytes.NewReader(raw), &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		WriteProblem(w, shared.ErrMalformedRequest.WithDetail("request.body_malformed").WithCause(err), requestID)
		return
	}

	in := usecase.Input{
		"scope_container_id": optionalUUIDField(body.Scope.ContainerId),
		"scope_item_id":      optionalUUIDField(body.Scope.ItemId),
		"include_archived":   optionalBoolField(body.IncludeArchived),
		"include_trashed":    optionalBoolField(body.IncludeTrashed),
	}
	if body.Scope.IncludeDescendants != nil {
		in["include_descendants"] = *body.Scope.IncludeDescendants
	}
	if body.Scope.Workspace != nil {
		in["scope_workspace"] = *body.Scope.Workspace
	}
	for _, part := range []string{"filter", "aggregate"} {
		if value, present := document[part]; present && value != nil {
			in[part] = value
		}
	}

	out, err := c.UseCases.Invoke(r.Context(), aggregateItemsUseCase, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, out)
}
