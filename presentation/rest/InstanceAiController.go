// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The AI models the installation offers (ADR-0072 §1), over REST. The operator's view: everything
// but the key, which no call answers.

const (
	listInstanceAiProvidersUseCase     = "ListInstanceAiProviders"
	configureInstanceAiProviderUseCase = "ConfigureInstanceAiProvider"
	removeInstanceAiProviderUseCase    = "RemoveInstanceAiProvider"
)

// ListInstanceAiProviders answers GET /admin/ai-providers.
func (c *RestController) ListInstanceAiProviders(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), listInstanceAiProvidersUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	rows, _ := out["data"].([]usecase.Output)
	answer := make([]openapi.OfferedAiModel, 0, len(rows))
	for _, row := range rows {
		answer = append(answer, offeredAiModelResponse(row))
	}
	writeJSON(w, r, http.StatusOK, answer)
}

// CreateInstanceAiProvider answers POST /admin/ai-providers.
func (c *RestController) CreateInstanceAiProvider(w http.ResponseWriter, r *http.Request) {
	c.writeOfferedAiModel(w, r, "", http.StatusCreated)
}

// ConfigureInstanceAiProvider answers PUT /admin/ai-providers/{providerId}.
func (c *RestController) ConfigureInstanceAiProvider(
	w http.ResponseWriter, r *http.Request, providerID openapi.ProviderId,
) {
	c.writeOfferedAiModel(w, r, providerID.String(), http.StatusOK)
}

func (c *RestController) writeOfferedAiModel(w http.ResponseWriter, r *http.Request, providerID string, status int) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.OfferedAiModelConfiguration
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{
		"display_name": body.DisplayName,
		"processor":    body.Processor,
		"kind":         string(body.Kind),
		"base_url":     body.BaseUrl,
		"jurisdiction": string(body.Jurisdiction),
	}
	if providerID != "" {
		in["id"] = providerID
	}
	if body.CompletionModel != nil {
		in["completion_model"] = *body.CompletionModel
	}
	if body.EmbeddingModel != nil {
		in["embedding_model"] = *body.EmbeddingModel
	}
	// Absent keeps the sealed key and empty clears it, so the key travels only when it was sent.
	if body.ApiKey != nil {
		in["api_key"] = *body.ApiKey
	}
	out, err := c.UseCases.Invoke(r.Context(), configureInstanceAiProviderUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, status, offeredAiModelResponse(out))
}

// RemoveInstanceAiProvider answers DELETE /admin/ai-providers/{providerId}.
func (c *RestController) RemoveInstanceAiProvider(
	w http.ResponseWriter, r *http.Request, providerID openapi.ProviderId,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	if _, err := c.UseCases.Invoke(r.Context(), removeInstanceAiProviderUseCase, actorOf(r),
		usecase.Input{"id": providerID.String()}); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func offeredAiModelResponse(out usecase.Output) openapi.OfferedAiModel {
	answer := openapi.OfferedAiModel{
		DisplayName:  out.String("display_name"),
		Processor:    out.String("processor"),
		Kind:         openapi.AiProviderKind(out.String("kind")),
		Jurisdiction: openapi.AiJurisdiction(out.String("jurisdiction")),
	}
	answer.Id = uuidValue(out.String("id"))
	if baseURL := out.String("base_url"); baseURL != "" {
		answer.BaseUrl = &baseURL
	}
	if model := out.String("completion_model"); model != "" {
		answer.CompletionModel = &model
	}
	if model := out.String("embedding_model"); model != "" {
		answer.EmbeddingModel = &model
	}
	if held, present := out["has_api_key"].(bool); present {
		answer.HasApiKey = held
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
