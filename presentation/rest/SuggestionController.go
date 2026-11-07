// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// What AI proposed.
const (
	acceptSuggestionUseCase  = "AcceptSuggestion"
	dismissSuggestionUseCase = "DismissSuggestion"
)

// suggestionPageResponse draws a page of suggestions, which carries its rows as `items`.
func suggestionPageResponse(out usecase.Output) openapi.SuggestionPage {
	page := openapi.SuggestionPage{Items: []openapi.Suggestion{}}
	if items, held := out["items"].([]usecase.Output); held {
		for _, item := range items {
			page.Items = append(page.Items, suggestionResponse(item))
		}
	}
	if cursor := out.String("next_cursor"); cursor != "" {
		page.NextCursor = &cursor
	}
	if more, held := out["has_more"].(bool); held {
		page.HasMore = more
	}
	return page
}

// acceptedSuggestionResponse is the accept answer: the suggestion and what accepting created.
func acceptedSuggestionResponse(out usecase.Output) acceptedResponse {
	accepted := acceptedResponse{Suggestion: suggestionResponse(out)}
	if created := out.String("created_id"); created != "" {
		id := uuidValue(created)
		accepted.CreatedID = &id
	}
	if listed, held := out["created_ids"].([]any); held {
		ids := make([]openapi_types.UUID, 0, len(listed))
		for _, each := range listed {
			if text, isText := each.(string); isText {
				ids = append(ids, uuidValue(text))
			}
		}
		accepted.CreatedIDs = &ids
	}
	return accepted
}

// acceptedResponse is `AcceptedSuggestion`: the suggestion's fields, flat, `created_id`, and the
// entries an ENTRIES acceptance created.
type acceptedResponse struct {
	openapi.Suggestion
	CreatedID  *openapi_types.UUID   `json:"created_id,omitempty"`
	CreatedIDs *[]openapi_types.UUID `json:"created_ids,omitempty"`
}

// suggestionResponse maps the use case's answer.
func suggestionResponse(out usecase.Output) openapi.Suggestion {
	answer := openapi.Suggestion{
		TargetType:    openapi.SuggestionTargetType(out.String("target_type")),
		Kind:          openapi.SuggestionKind(out.String("kind")),
		Status:        openapi.SuggestionStatus(out.String("status")),
		Source:        openapi.SuggestionSource(out.String("source")),
		Model:         out.String("model"),
		PromptId:      out.String("prompt_id"),
		PromptVersion: out.String("prompt_version"),
		Payload:       map[string]any{},
	}
	answer.Id = uuidValue(out.String("id"))
	answer.TargetId = uuidValue(out.String("target_id"))
	if payload, held := out["payload"].(map[string]any); held {
		answer.Payload = payload
	}
	dropped := out.Int("dropped_nodes")
	answer.DroppedNodes = &dropped
	if produced, held := out["produced_at"].(time.Time); held {
		answer.ProducedAt = produced
	}
	if created, held := out["created_at"].(time.Time); held {
		answer.CreatedAt = created
	}
	if decided, held := out["decided_at"].(time.Time); held && !decided.IsZero() {
		answer.DecidedAt = &decided
		by := uuidValue(out.String("decided_by"))
		answer.DecidedBy = &by
	}
	if version, held := out["version"].(int); held {
		answer.Version = version
	}
	return answer
}

// acceptedAsk is what an AI ask answers: the job that will put the question. A client follows it to
// learn that the job met a refusal - a budget spent since, AI switched off - rather than an answer.
func acceptedAsk(out usecase.Output) openapi.JobRef {
	return openapi.JobRef{JobId: uuidValue(out.String("job_id")), Status: openapi.JobStatusQUEUED}
}
