// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The saved views. The controller holds no rules, as ever: it reads the request, hands it
// to the catalogue, and maps the result - what a layout means, who sees a shared view and whether
// a stored query parses are all decided inwards of here.


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
