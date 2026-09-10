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

// What AI proposed (J-05).
const (
	listSuggestionsUseCase      = "ListSuggestions"
	acceptSuggestionUseCase     = "AcceptSuggestion"
	dismissSuggestionUseCase    = "DismissSuggestion"
	suggestDecompositionUseCase = "SuggestDecomposition"
	aiSuggestFieldsUseCase      = "AiSuggestFields"
	aiSummarizeUseCase          = "AiSummarize"
	aiClassifyUseCase           = "AiClassify"
	suggestDuplicatesUseCase    = "SuggestDuplicates"
	aiSummarizeThreadUseCase    = "AiSummarizeThread"
	aiSummarizeContainerUseCase = "AiSummarizeContainer"
)

// The three actions automation.md §1.3 documents, served over REST as well because an automation
// action is a use case and a use case is reachable through all three channels (J-08).
func (c *RestController) AiSuggestFields(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
) {
	c.askAi(w, r, aiSuggestFieldsUseCase, itemID)
}

func (c *RestController) AiSummarize(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
) {
	c.askAi(w, r, aiSummarizeUseCase, itemID)
}

func (c *RestController) AiClassify(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
) {
	c.askAi(w, r, aiClassifyUseCase, itemID)
}

// AiSummarizeThread answers POST /items/{itemId}:summarize-thread (K-05).
func (c *RestController) AiSummarizeThread(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
) {
	c.askAi(w, r, aiSummarizeThreadUseCase, itemID)
}

// AiSummarizeContainer answers POST /containers/{containerId}:summarize (K-05).
//
// Its own handler rather than `askAi`'s, because what it names is a container: the input key
// differs, and a body asking for the answer to be applied would be asking for something nothing
// does.
func (c *RestController) AiSummarizeContainer(
	w http.ResponseWriter, r *http.Request, containerID openapi.ContainerId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	if _, err := c.UseCases.Invoke(
		r.Context(), aiSummarizeContainerUseCase, actorOf(r),
		usecase.Input{"container_id": containerID.String()},
	); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// askAi is the four of them, which differ in the use case they name and in nothing else.
func (c *RestController) askAi(
	w http.ResponseWriter, r *http.Request, name string, itemID openapi.ItemId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String()}
	// The body is optional: a caller who wants a proposal sends none, which is the default, and
	// an empty one is not an error. Anything that is there is read, and a malformed document still
	// is (StartOidcSignIn's reasoning).
	if r.ContentLength > 0 {
		var body openapi.AiAsk
		if err := decodeJSON(r, &body); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
		if body.Apply != nil {
			in["apply"] = *body.Apply
		}
	}

	if _, err := c.UseCases.Invoke(r.Context(), name, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// SuggestDecomposition answers POST /items/{itemId}:decompose.
//
// 202 and no body: the provider has not been asked yet, so there is nothing to answer with.
func (c *RestController) SuggestDecomposition(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	_ openapi.SuggestDecompositionParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	if _, err := c.UseCases.Invoke(
		r.Context(), suggestDecompositionUseCase, actorOf(r),
		usecase.Input{"item_id": itemID.String()},
	); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// SuggestDuplicates answers POST /items/{itemId}:duplicates.
//
// 200 with the suggestion, or 204 when there is nothing to propose - which is also the answer with
// no pgvector, with no provider that can embed, and for an entry the embedding pass has not
// reached. The adapter cannot tell those apart and does not try: they are one answer to the caller,
// and which one it is is `/meta/capabilities`' business (K-04).
func (c *RestController) SuggestDuplicates(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(
		r.Context(), suggestDuplicatesUseCase, actorOf(r),
		usecase.Input{"item_id": itemID.String()},
	)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if len(out) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, r, http.StatusOK, suggestionResponse(out))
}

// ListSuggestions answers GET /suggestions.
func (c *RestController) ListSuggestions(
	w http.ResponseWriter, r *http.Request, params openapi.ListSuggestionsParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{
		"target_type": string(params.TargetType),
		"target_id":   params.TargetId.String(),
	}
	if params.Status != nil {
		in["status"] = string(*params.Status)
	}
	if params.Cursor != nil {
		in["cursor"] = *params.Cursor
	}
	if params.Size != nil {
		in["page_size"] = *params.Size
	}

	out, err := c.UseCases.Invoke(r.Context(), listSuggestionsUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

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
	writeJSON(w, r, http.StatusOK, page)
}

// AcceptSuggestion answers POST /suggestions/{id}:accept.
func (c *RestController) AcceptSuggestion(
	w http.ResponseWriter, r *http.Request, suggestionID openapi.SuggestionId,
) {
	c.decideSuggestion(w, r, acceptSuggestionUseCase, suggestionID)
}

// DismissSuggestion answers POST /suggestions/{id}:dismiss.
func (c *RestController) DismissSuggestion(
	w http.ResponseWriter, r *http.Request, suggestionID openapi.SuggestionId,
) {
	c.decideSuggestion(w, r, dismissSuggestionUseCase, suggestionID)
}

// decideSuggestion is both answers: they differ in the use case they name and in nothing else,
// which is the same shape the application layer has for the same reason.
//
// The body is read, and it was not until J-16 - which made a whole class of suggestion impossible
// to accept over REST. `SuggestionAcceptance` carries the overrides a person changed or added
// before accepting, and a proposal about a jumble entry *needs* one: accepting it converts the
// entry, and `ConvertJumbleEntry` requires a destination collection a model cannot name. Dropping
// the body meant every such acceptance answered `usecase.field_required` for a field the caller
// had in fact sent.
//
// Optional, because a dismissal carries none and an acceptance of a work item's fields needs none:
// an absent body is an empty input rather than a refusal.
func (c *RestController) decideSuggestion(
	w http.ResponseWriter, r *http.Request, name string, suggestionID openapi.SuggestionId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"suggestion_id": suggestionID.String()}
	if r.ContentLength > 0 {
		var body openapi.SuggestionAcceptance
		if err := decodeJSON(r, &body); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
		if body.Overrides != nil {
			in["overrides"] = *body.Overrides
		}
	}

	out, err := c.UseCases.Invoke(r.Context(), name, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, suggestionResponse(out))
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
