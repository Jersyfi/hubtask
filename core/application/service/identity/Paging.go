// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
)

const (
	// defaultPageSize is what a caller that names no size gets (api-guidelines.md §4).
	defaultPageSize = 50
	// maxPageSize is the ceiling. A clamp rather than a refusal: a client asking for 500 rows
	// wants as many as it can have, and 200 of them is a better answer than an error.
	maxPageSize = 200
)

func pageSize(requested int) int {
	switch {
	case requested < 1:
		return defaultPageSize
	case requested > maxPageSize:
		return maxPageSize
	default:
		return requested
	}
}

// pageOutput is the contract's list shape, as every channel renders it: the rows under `data`,
// the walk's state under `page`, with an explicit null for the cursor of the last page.
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
