// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The templates: their own resource rather than a sub-resource of a container, because one
// may be defined for the whole workspace and a path would have to invent a container for it.

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

// templateInstanceResponse is what an instantiation made, and every reference it dropped.
func templateInstanceResponse(out usecase.Output) openapi.TemplateInstance {
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
	return instance
}
