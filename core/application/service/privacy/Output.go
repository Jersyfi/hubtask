// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The projection every channel answers with. In the application layer rather than in the REST
// adapter, because MCP and an automation rule read the same case (arc42 §4).

// RequestOutput is one case as it leaves the boundary.
//
// The subject travels as an identifier and an address, both of which are personal data and both of
// which the caller supplied - a case about somebody that could not say who would be a case nobody
// could act on. What is *not* here is anything of the person's beyond that: a case is a record of
// an obligation rather than a copy of what is held about them.
func RequestOutput(request domain.Request) usecase.Output {
	out := usecase.Output{
		"id":          request.ID.String(),
		"kind":        string(request.Kind),
		"status":      string(request.Status),
		"scope":       string(request.Scope),
		"received_at": request.ReceivedAt.UTC(),
		"due_at":      request.DueAt.UTC(),
	}
	for field, value := range map[string]shared.ID{
		"subject_account_id": request.SubjectAccountID,
		"handled_by":         request.HandledBy,
		"result_target_id":   request.TargetID,
	} {
		if !value.IsZero() {
			out[field] = value.String()
		}
	}
	for field, value := range map[string]string{
		"subject_email":    request.SubjectEmail,
		"erasure_mode":     string(request.ErasureMode),
		"rejection_reason": request.RejectionReason,
		"result_archive":   request.ResultArchive,
		"notes":            request.Notes,
	} {
		if value != "" {
			out[field] = value
		}
	}
	if !request.CompletedAt.IsZero() {
		out["completed_at"] = request.CompletedAt.UTC()
	}
	return out
}

// pageOutput is the shape of a paged answer: `{ "data": [...], "page": {...} }`
// (api-guidelines.md §4).
func pageOutput(data []usecase.Output, info repository.PageInfo) usecase.Output {
	page := map[string]any{"next_cursor": nil, "has_more": info.HasMore}
	if info.NextCursor != "" {
		page["next_cursor"] = info.NextCursor
	}
	return usecase.Output{"data": data, "page": page}
}

// parseInstant reads the one spelling the contract declares, RFC 3339.
func parseInstant(raw, field string) (time.Time, error) {
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		code := "privacy." + field + "_malformed"
		return time.Time{}, shared.ErrValidation.
			WithDetail(code).
			WithParams(map[string]string{"value": raw}).
			WithFields(shared.FieldError{Path: "/" + field, Code: code})
	}
	return at, nil
}
