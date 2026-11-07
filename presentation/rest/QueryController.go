// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

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
