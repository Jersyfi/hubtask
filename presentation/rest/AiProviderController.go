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

// The AI provider's configuration use cases (J-02).
const (
	readAiProviderUseCase      = "ReadAiProvider"
	configureAiProviderUseCase = "ConfigureAiProvider"
	removeAiProviderUseCase    = "RemoveAiProvider"
)

// ReadAiProvider answers GET /ai-provider.
func (c *RestController) ReadAiProvider(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), readAiProviderUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, aiProviderResponse(out))
}

// ConfigureAiProvider answers PUT /ai-provider.
func (c *RestController) ConfigureAiProvider(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.AiProviderConfiguration
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"kind":         string(body.Kind),
		"jurisdiction": string(body.Jurisdiction),
	}
	if body.BaseUrl != nil {
		in["base_url"] = *body.BaseUrl
	}
	if body.CompletionModel != nil {
		in["completion_model"] = *body.CompletionModel
	}
	if body.EmbeddingModel != nil {
		in["embedding_model"] = *body.EmbeddingModel
	}
	if body.ProcessingAllowed != nil {
		in["processing_allowed"] = *body.ProcessingAllowed
	}
	// Absent keeps the stored key and present-but-empty clears it, which is the one field here
	// where the difference is the whole meaning - so the pointer is read rather than dereferenced
	// into a value that would lose it.
	if body.ApiKey != nil {
		in["api_key"] = *body.ApiKey
	}

	out, err := c.UseCases.Invoke(r.Context(), configureAiProviderUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, aiProviderResponse(out))
}

// RemoveAiProvider answers DELETE /ai-provider.
func (c *RestController) RemoveAiProvider(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	if _, err := c.UseCases.Invoke(
		r.Context(), removeAiProviderUseCase, actorOf(r), usecase.Input{},
	); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// aiProviderResponse maps the use case's answer. The API key is not among the fields, because it
// is not among the use case's either - there is no call that answers it.
func aiProviderResponse(out usecase.Output) openapi.AiProvider {
	answer := openapi.AiProvider{
		Kind:         openapi.AiProviderKind(out.String("kind")),
		Jurisdiction: openapi.AiJurisdiction(out.String("jurisdiction")),
	}
	if baseURL := out.String("base_url"); baseURL != "" {
		answer.BaseUrl = &baseURL
	}
	if model := out.String("completion_model"); model != "" {
		answer.CompletionModel = &model
	}
	if model := out.String("embedding_model"); model != "" {
		answer.EmbeddingModel = &model
	}
	if allowed, held := out["processing_allowed"].(bool); held {
		answer.ProcessingAllowed = allowed
	}
	if created, held := out["created_at"].(time.Time); held {
		answer.CreatedAt = created
	}
	if updated, held := out["updated_at"].(time.Time); held && !updated.IsZero() {
		answer.UpdatedAt = &updated
	}
	if version, held := out["version"].(int); held {
		answer.Version = version
	}
	return answer
}
