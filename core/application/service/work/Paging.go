// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// The page size limits of api-guidelines.md §4, and the shape a paged answer takes.
//
// In the application layer rather than in the REST adapter, because all three channels page: an MCP
// tool call and an automation action ask for a page the same way a request does, and a limit enforced
// in one adapter is a limit the other two do not have.
const (
	// DefaultPageSize is what a caller that names no size gets.
	DefaultPageSize = 50
	// MaxPageSize is the ceiling. Bulk export goes through an :export job instead, which is why this
	// is a clamp rather than a refusal: a client asking for 500 rows wants as many as it can have,
	// and 200 of them is a better answer than an error.
	MaxPageSize = 200
)

// PageSize clamps a requested size into the contract's range.
func PageSize(requested int) int {
	switch {
	case requested < 1:
		return DefaultPageSize
	case requested > MaxPageSize:
		return MaxPageSize
	default:
		return requested
	}
}

// pageOutput is the response shape of every paged read: `{ "data": [...], "page": {...} }`
// (api-guidelines.md §4).
//
// The rows are `[]usecase.Output` rather than `[]any`, so that the three channels see one shape - a
// REST body, an MCP tool result and an automation action result all describe the page in the same
// words as the contract's PageInfo schema.
func pageOutput(data []usecase.Output, info repository.PageInfo) usecase.Output {
	page := map[string]any{
		"next_cursor": nil,
		"has_more":    info.HasMore,
	}
	if info.NextCursor != "" {
		page["next_cursor"] = info.NextCursor
	}
	return usecase.Output{"data": data, "page": page}
}
