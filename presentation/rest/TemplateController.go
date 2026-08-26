// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The templates (D-06): their own resource rather than a sub-resource of a container, because one
// may be defined for the whole workspace and a path would have to invent a container for it.

const (
	listTemplatesUseCase       = "ListTemplates"
	createTemplateUseCase      = "CreateTemplate"
	getTemplateUseCase         = "GetTemplate"
	updateTemplateUseCase      = "UpdateTemplate"
	deleteTemplateUseCase      = "DeleteTemplate"
	instantiateTemplateUseCase = "InstantiateTemplate"
)

// ListTemplates answers GET /templates.
func (c *RestController) ListTemplates(
	w http.ResponseWriter, r *http.Request, params openapi.ListTemplatesParams,
) {
	in := usecase.Input{
		"cursor": optionalStringField(params.Cursor),
		"size":   optionalIntField(params.Size),
	}
	if params.ContainerId != nil {
		in["container_id"] = params.ContainerId.String()
	}

	out, ok := c.read(w, r, listTemplatesUseCase, in)
	if !ok {
		return
	}

	page := openapi.TemplatePage{Data: []openapi.Template{}, Page: pageResponse(out)}
	for _, row := range rowsOf(out) {
		page.Data = append(page.Data, templateResponse(row))
	}
	writeJSON(w, r, http.StatusOK, page)
}

// CreateTemplate answers POST /templates.
func (c *RestController) CreateTemplate(
	w http.ResponseWriter, r *http.Request, _ openapi.CreateTemplateParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.CreateTemplateJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"scope_type": string(body.ScopeType),
		"name":       body.Name,
		"root_type":  string(body.RootType),
		"nodes":      templateNodesInput(body.Nodes),
	}
	if body.ScopeId != nil {
		in["scope_id"] = body.ScopeId.String()
	}
	if body.Description != nil {
		in["description"] = *body.Description
	}

	out, err := c.UseCases.Invoke(r.Context(), createTemplateUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusCreated, templateResponse(out))
}

// GetTemplate answers GET /templates/{templateId}.
func (c *RestController) GetTemplate(
	w http.ResponseWriter, r *http.Request, templateID openapi.TemplateId,
) {
	out, ok := c.read(w, r, getTemplateUseCase,
		usecase.Input{"template_id": templateID.String()})
	if !ok {
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, templateResponse(out))
}

// UpdateTemplate answers PATCH /templates/{templateId}.
func (c *RestController) UpdateTemplate(
	w http.ResponseWriter, r *http.Request, templateID openapi.TemplateId,
	params openapi.UpdateTemplateParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.UpdateTemplateJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"template_id": templateID.String()}
	if body.Name != nil {
		in["name"] = *body.Name
	}
	if body.Description != nil {
		in["description"] = *body.Description
	}
	// Presence rather than the value: an absent tree means "not touched", and a handler that could
	// not tell it from an empty one would answer one caller's request with the other's meaning.
	if body.Nodes != nil {
		in["nodes"] = templateNodesInput(*body.Nodes)
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), updateTemplateUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, templateResponse(out))
}

// DeleteTemplate answers DELETE /templates/{templateId}. 204: the caller asked for the template to
// be gone, and the trees it stamped out are read as the entries they are.
func (c *RestController) DeleteTemplate(
	w http.ResponseWriter, r *http.Request, templateID openapi.TemplateId,
	params openapi.DeleteTemplateParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"template_id": templateID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	if _, err := c.UseCases.Invoke(r.Context(), deleteTemplateUseCase, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// InstantiateTemplate answers POST /templates/{templateId}:instantiate.
func (c *RestController) InstantiateTemplate(
	w http.ResponseWriter, r *http.Request, templateID openapi.TemplateId,
	_ openapi.InstantiateTemplateParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.InstantiateTemplateJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"template_id":   templateID.String(),
		"collection_id": body.CollectionId.String(),
	}
	if body.ParentId != nil {
		in["parent_id"] = body.ParentId.String()
	}
	if body.AnchorDate != nil {
		in["anchor_date"] = body.AnchorDate.Format("2006-01-02")
	}
	if body.Title != nil {
		in["title"] = *body.Title
	}

	out, err := c.UseCases.Invoke(r.Context(), instantiateTemplateUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	instance := openapi.TemplateInstance{
		TemplateId:        uuidValue(out.String("template_id")),
		RootItemId:        uuidValue(out.String("root_item_id")),
		Created:           out.Int("created"),
		DroppedReferences: []openapi.DroppedReference{},
	}
	for _, row := range outputsOf(out["dropped_references"]) {
		itemID := uuidValue(row.String("item_id"))
		instance.DroppedReferences = append(instance.DroppedReferences, openapi.DroppedReference{
			ItemId: &itemID,
			Kind:   openapi.DroppedReferenceKind(row.String("kind")),
			Id:     row.String("reference"),
			Code:   row.String("code"),
		})
	}
	writeJSON(w, r, http.StatusCreated, instance)
}

// templateNodesInput maps the contract's tree onto the untyped document the catalogue takes. The
// mapping is here because the generated types are the contract's shape rather than the domain's
// (project-structure.md §3).
func templateNodesInput(nodes []openapi.TemplateNode) []any {
	document := make([]any, 0, len(nodes))
	for _, node := range nodes {
		document = append(document, templateNodeInput(node))
	}
	return document
}

func templateNodeInput(node openapi.TemplateNode) map[string]any {
	document := map[string]any{
		"type":  string(node.Type),
		"title": node.Title,
	}
	if node.Notes != nil {
		document["notes"] = *node.Notes
	}
	if node.DueOffset != nil {
		document["due_offset"] = *node.DueOffset
	}
	if node.DueDateOnly != nil {
		document["due_date_only"] = *node.DueDateOnly
	}
	if node.AssigneeId != nil {
		document["assignee_id"] = node.AssigneeId.String()
	}
	if node.Children != nil {
		document["children"] = templateNodesInput(*node.Children)
	}
	return document
}

// templateResponse maps the catalogue's output onto the generated schema.
func templateResponse(out usecase.Output) openapi.Template {
	template := openapi.Template{
		Id:        uuidValue(out.String("id")),
		ScopeType: openapi.TemplateScope(out.String("scope_type")),
		Name:      out.String("name"),
		RootType:  openapi.ItemType(out.String("root_type")),
		Nodes:     []openapi.TemplateNode{},
		CreatedAt: timeValue(out["created_at"]),
		Version:   out.Int("version"),
	}
	if scope := out.String("scope_id"); scope != "" {
		scopeID := uuidValue(scope)
		template.ScopeId = &scopeID
	}
	if description := out.String("description"); description != "" {
		template.Description = &description
	}
	if at, ok := out["updated_at"].(time.Time); ok {
		template.UpdatedAt = &at
	}
	for _, node := range outputsOf(out["nodes"]) {
		template.Nodes = append(template.Nodes, templateNodeResponse(node))
	}
	return template
}

func templateNodeResponse(out usecase.Output) openapi.TemplateNode {
	node := openapi.TemplateNode{
		Type:  openapi.ItemType(out.String("type")),
		Title: out.String("title"),
	}
	if notes := out.String("notes"); notes != "" {
		node.Notes = &notes
	}
	if offset := out.String("due_offset"); offset != "" {
		node.DueOffset = &offset
	}
	if flag, ok := out["due_date_only"].(bool); ok {
		node.DueDateOnly = &flag
	}
	if assignee := out.String("assignee_id"); assignee != "" {
		id := uuidValue(assignee)
		node.AssigneeId = &id
	}

	children := make([]openapi.TemplateNode, 0)
	for _, child := range outputsOf(out["children"]) {
		children = append(children, templateNodeResponse(child))
	}
	node.Children = &children
	return node
}

// outputsOf reads a list of projections out of an answer, empty when the shape is not what it
// should be - the same restraint rowsOf keeps, and for the same reason.
func outputsOf(value any) []usecase.Output {
	rows, _ := value.([]usecase.Output)
	return rows
}
