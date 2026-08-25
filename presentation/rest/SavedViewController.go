// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The saved views (D-07). The controller holds no rules, as ever: it reads the request, hands it
// to the catalogue, and maps the result - what a layout means, who sees a shared view and whether
// a stored query parses are all decided inwards of here.

const (
	listSavedViewsUseCase  = "ListSavedViews"
	createSavedViewUseCase = "CreateSavedView"
	getSavedViewUseCase    = "GetSavedView"
	updateSavedViewUseCase = "UpdateSavedView"
	deleteSavedViewUseCase = "DeleteSavedView"
	shareSavedViewUseCase  = "ShareSavedView"
)

// ListSavedViews answers GET /views.
func (c *RestController) ListSavedViews(
	w http.ResponseWriter, r *http.Request, params openapi.ListSavedViewsParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{}
	if params.ContainerId != nil {
		in["container_id"] = params.ContainerId.String()
	}

	out, err := c.UseCases.Invoke(r.Context(), listSavedViewsUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	rows, _ := out["data"].([]usecase.Output)
	views := make([]openapi.SavedView, 0, len(rows))
	for _, row := range rows {
		views = append(views, savedViewResponse(row))
	}
	writeJSON(w, r, http.StatusOK, views)
}

// CreateSavedView answers POST /views.
func (c *RestController) CreateSavedView(
	w http.ResponseWriter, r *http.Request, _ openapi.CreateSavedViewParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.SavedViewCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"scope_type": string(body.ScopeType),
		"name":       body.Name,
		"layout":     body.Layout,
		"query":      map[string]any(body.Query),
	}
	if body.ScopeId != nil {
		in["scope_id"] = body.ScopeId.String()
	}
	if body.Grouping != nil {
		in["grouping"] = map[string]any(*body.Grouping)
	}
	if body.VisibleFields != nil {
		in["visible_fields"] = stringList(*body.VisibleFields)
	}
	if body.Sharing != nil {
		in["sharing"] = string(*body.Sharing)
	}

	out, err := c.UseCases.Invoke(r.Context(), createSavedViewUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	created := savedViewResponse(out)
	w.Header().Set("Location", APIBasePath+"/views/"+created.Id.String())
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusCreated, created)
}

// GetSavedView answers GET /views/{viewId}.
func (c *RestController) GetSavedView(w http.ResponseWriter, r *http.Request, viewID openapi.ViewId) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), getSavedViewUseCase, actorOf(r),
		usecase.Input{"view_id": viewID.String()})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, savedViewResponse(out))
}

// UpdateSavedView answers PATCH /views/{viewId}. A merge patch: a member that is absent is left
// alone, and the query and the hints replace whole - a query is one statement.
func (c *RestController) UpdateSavedView(
	w http.ResponseWriter, r *http.Request, viewID openapi.ViewId, params openapi.UpdateSavedViewParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.SavedViewUpdate
	present, err := decodeJSONWithPresence(r, &body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"view_id": viewID.String()}
	if present["name"] {
		in["name"] = stringOrEmpty(body.Name)
	}
	if present["layout"] {
		in["layout"] = stringOrEmpty(body.Layout)
	}
	if present["query"] && body.Query != nil {
		in["query"] = map[string]any(*body.Query)
	}
	if present["grouping"] && body.Grouping != nil {
		in["grouping"] = map[string]any(*body.Grouping)
	}
	if present["visible_fields"] && body.VisibleFields != nil {
		in["visible_fields"] = stringList(*body.VisibleFields)
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), updateSavedViewUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, savedViewResponse(out))
}

// DeleteSavedView answers DELETE /views/{viewId}.
func (c *RestController) DeleteSavedView(
	w http.ResponseWriter, r *http.Request, viewID openapi.ViewId, params openapi.DeleteSavedViewParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"view_id": viewID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	if _, err := c.UseCases.Invoke(r.Context(), deleteSavedViewUseCase, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ShareSavedView answers POST /views/{viewId}:share.
func (c *RestController) ShareSavedView(
	w http.ResponseWriter, r *http.Request, viewID openapi.ViewId, params openapi.ShareSavedViewParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.SavedViewShare
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"view_id": viewID.String(), "sharing": string(body.Sharing)}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), shareSavedViewUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, savedViewResponse(out))
}

// savedViewResponse maps the catalogue's output onto the generated schema.
func savedViewResponse(out usecase.Output) openapi.SavedView {
	saved := openapi.SavedView{
		Id:        uuidValue(out.String("id")),
		ScopeType: openapi.SavedViewScopeType(out.String("scope_type")),
		OwnerId:   uuidValue(out.String("owner_id")),
		Name:      out.String("name"),
		Layout:    out.String("layout"),
		Sharing:   openapi.SavedViewSharing(out.String("sharing")),
		Version:   out.Int("version"),
	}
	if scope := out.String("scope_id"); scope != "" {
		scopeID := uuidValue(scope)
		saved.ScopeId = &scopeID
	}
	if query, carried := out["query"].(map[string]any); carried {
		saved.Query = query
	}
	if grouping, carried := out["grouping"].(map[string]any); carried {
		saved.Grouping = grouping
	}
	if fields, carried := out["visible_fields"].([]string); carried {
		saved.VisibleFields = fields
	}
	if at := timeValue(out["created_at"]); !at.IsZero() {
		saved.CreatedAt = &at
	}
	return saved
}

// stringList reads the wire's string array into the catalogue's list shape.
func stringList(values []string) []any {
	list := make([]any, 0, len(values))
	for _, value := range values {
		list = append(list, value)
	}
	return list
}
